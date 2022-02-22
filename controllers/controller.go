package controllers

import (
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	"github.com/pomerium/ingress-controller/model"
)

//go:generate go run github.com/golang/mock/mockgen -package controllers -destination client_mock.go sigs.k8s.io/controller-runtime/pkg/client Client

const (
	// IngressClassAnnotationKey although deprecated, still may be used by the HTTP solvers even for v1 Ingress resources
	// see https://kubernetes.io/blog/2020/04/02/improvements-to-the-ingress-api-in-kubernetes-1.18/#deprecating-the-ingress-class-annotation
	IngressClassAnnotationKey = "kubernetes.io/ingress.class"

	initialReconciliationTimeout = time.Minute * 5

	reasonPomeriumConfigUpdated     = "Updated"
	reasonPomeriumConfigUpdateError = "UpdateError"
	msgPomeriumConfigUpdated        = "updated pomerium configuration"
)

// ingressController watches ingress and related resources for updates and reconciles with pomerium
type ingressController struct {
	// controllerName to watch in the IngressClass.spec.controller
	controllerName string
	// annotationPrefix is a prefix (without /) for Ingress annotations
	annotationPrefix string

	// Scheme keeps track between objects and their group/version/kinds
	*runtime.Scheme
	// Client is k8s apiserver client proxied thru controller-runtime,
	// that also embeds object cache
	client.Client

	// PomeriumReconciler updates Pomerium service configuration
	PomeriumReconciler
	// Registry keeps track of dependencies between k8s objects
	model.Registry
	// EventRecorder provides means to add events to Ingress objects, that are visible via kubectl describe
	record.EventRecorder

	// Namespaces to listen to, nil/empty to listen to all
	namespaces map[string]bool

	// updateStatusFromService defines a pomerium-proxy service name that should be watched for changes in the status field
	// and all dependent ingresses should be updated accordingly
	updateStatusFromService *types.NamespacedName

	// object Kinds are frequently used, do not change and are cached
	endpointsKind    string
	ingressKind      string
	ingressClassKind string
	secretKind       string
	serviceKind      string

	initComplete *once
}

// Option customizes ingress controller
type Option func(ic *ingressController)

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

// SetupWithManager sets up the controller with the Manager
func (r *ingressController) SetupWithManager(mgr ctrl.Manager) error {
	c, err := ctrl.NewControllerManagedBy(mgr).
		For(&networkingv1.Ingress{}).
		Build(r)
	if err != nil {
		return err
	}

	r.Scheme = mgr.GetScheme()
	for _, o := range []struct {
		client.Object
		kind  *string
		mapFn func(string) func(client.Object) []reconcile.Request
	}{
		{&networkingv1.Ingress{}, &r.ingressKind, nil},
		{&networkingv1.IngressClass{}, &r.ingressClassKind, r.watchIngressClass},
		{&corev1.Secret{}, &r.secretKind, r.getDependantIngressFn},
		{&corev1.Service{}, &r.serviceKind, r.getDependantIngressFn},
		{&corev1.Endpoints{}, &r.endpointsKind, r.getDependantIngressFn},
	} {
		gvk, err := apiutil.GVKForObject(o.Object, r.Scheme)
		if err != nil {
			return fmt.Errorf("cannot get kind: %w", err)
		}
		*o.kind = gvk.Kind

		if nil == o.mapFn {
			continue
		}

		if err := c.Watch(
			&source.Kind{Type: o.Object},
			handler.EnqueueRequestsFromMapFunc(o.mapFn(gvk.Kind))); err != nil {
			return fmt.Errorf("watching %s: %w", gvk.String(), err)
		}
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
