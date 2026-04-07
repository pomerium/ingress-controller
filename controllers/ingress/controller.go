package ingress

import (
	"time"

	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	icsv1 "github.com/pomerium/ingress-controller/apis/ingress/v1"
	"github.com/pomerium/ingress-controller/controllers/reporter"
	"github.com/pomerium/ingress-controller/model"
	"github.com/pomerium/ingress-controller/pomerium"
	"github.com/pomerium/ingress-controller/util/generic"
)

const (
	initialReconciliationTimeout = time.Minute * 5
	controllerName               = "pomerium-ingress"
)

// ingressController watches ingress and related resources for updates and reconciles with pomerium
type ingressController struct {
	// controllerName to watch in the IngressClass.spec.controller
	controllerName string
	// annotationPrefix is a prefix (without /) for Ingress annotations
	annotationPrefix string

	// Scheme keeps track between objects and their group/version/kinds
	*runtime.Scheme
	// Client is k8s apiserver client with object caching
	client.Client

	// PomeriumReconciler updates Pomerium service configuration
	pomerium.IngressReconciler
	// Registry keeps track of dependencies between k8s objects
	model.Registry

	// Namespaces to listen to, nil/empty to listen to all
	namespaces map[string]bool

	// ingressStatusReporter is used to report ingress status changes
	reporter.MultiIngressStatusReporter

	// updateStatusFromService defines a pomerium-proxy service name that should be watched for changes in the status field
	// and all dependent ingresses should be updated accordingly
	updateStatusFromService *types.NamespacedName

	// globalSettings defines which global settings object to watch
	globalSettings *types.NamespacedName

	// object Kinds are frequently used, do not change and are cached
	endpointsKind    string
	ingressKind      string
	ingressClassKind string
	secretKind       string
	serviceKind      string
	settingsKind     string

	initComplete *once
}

// Option customizes ingress controller
type Option func(ic *ingressController)

// WithGlobalSettings makes ingress controller set up and report
func WithGlobalSettings(name types.NamespacedName) Option {
	return func(ic *ingressController) {
		ic.globalSettings = &name
	}
}

// WithIngressStatusReporter adds ingress status reporting option, multiple may be added
func WithIngressStatusReporter(reporters ...reporter.IngressStatusReporter) Option {
	return func(ic *ingressController) {
		ic.MultiIngressStatusReporter = append(ic.MultiIngressStatusReporter, reporters...)
	}
}

// WithControllerName changes default ingress controller name
func WithControllerName(name string) Option {
	return func(ic *ingressController) {
		ic.controllerName = name
	}
}

// WithAnnotationPrefix makes ingress controller watch annotation with custom prefix
func WithAnnotationPrefix(prefix string) Option {
	return func(ic *ingressController) {
		ic.annotationPrefix = prefix
	}
}

// WithNamespaces requires ingress controller to only monitor specific namespaces
func WithNamespaces(ns []string) Option {
	return func(ic *ingressController) {
		ic.namespaces = arrayToMap(ns)
	}
}

// WithUpdateIngressStatusFromService configures ingress controller to watch a designated service (pomerium proxy)
// for its load balancer status, and update all managed ingresses accordingly
func WithUpdateIngressStatusFromService(name types.NamespacedName) Option {
	return func(ic *ingressController) {
		ic.updateStatusFromService = &name
	}
}

// WithWatchSettings specifies which global settings to watch
func WithWatchSettings(name types.NamespacedName) Option {
	return func(ic *ingressController) {
		ic.globalSettings = &name
	}
}

// SetupWithManager sets up the controller with the Manager
func (r *ingressController) SetupWithManager(mgr ctrl.Manager) error {
	r.Client = mgr.GetClient()
	r.Scheme = mgr.GetScheme()

	// cache frequently used object kinds
	r.secretKind = generic.GVKForType[*corev1.Secret](r.Scheme).Kind
	r.ingressKind = generic.GVKForType[*networkingv1.Ingress](r.Scheme).Kind
	r.serviceKind = generic.GVKForType[*corev1.Service](r.Scheme).Kind
	r.settingsKind = generic.GVKForType[*icsv1.Pomerium](r.Scheme).Kind
	r.endpointsKind = generic.GVKForType[*corev1.Endpoints](r.Scheme).Kind
	r.ingressClassKind = generic.GVKForType[*networkingv1.IngressClass](r.Scheme).Kind

	err := ctrl.NewControllerManagedBy(mgr).
		Named(controllerName).
		For(&networkingv1.Ingress{}).
		Watches(
			&networkingv1.IngressClass{},
			handler.EnqueueRequestsFromMapFunc(r.watchIngressClass()),
			builder.WithPredicates(generic.NewPredicateFuncs(func(ic *networkingv1.IngressClass) bool {
				return ic.Spec.Controller == r.controllerName
			})),
		).
		Watches(&corev1.Secret{}, handler.EnqueueRequestsFromMapFunc(r.getDependantIngressFn(r.secretKind))).
		Watches(&corev1.Service{}, handler.EnqueueRequestsFromMapFunc(r.getDependantIngressFn(r.serviceKind))).
		Watches(&corev1.Endpoints{}, handler.EnqueueRequestsFromMapFunc(r.getDependantIngressFn(r.endpointsKind))).
		WithEventFilter(predicate.ResourceVersionChangedPredicate{}).
		Complete(r)
	if err != nil {
		return err
	}
	return nil
}

func (r *ingressController) isWatching(obj client.Object) bool {
	if len(r.namespaces) == 0 {
		return true
	}

	if (r.updateStatusFromService != nil) &&
		(*r.updateStatusFromService == types.NamespacedName{Name: obj.GetName(), Namespace: obj.GetNamespace()}) {
		return true
	}

	return r.namespaces[obj.GetNamespace()]
}
