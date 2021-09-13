package controllers

import (
	"context"
	"fmt"
	"reflect"
	"strings"

	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	"github.com/pomerium/ingress-controller/model"
)

const (
	// IngressClassDefaultAnnotationKey see https://kubernetes.io/docs/concepts/services-networking/ingress/#default-ingress-class
	IngressClassDefaultAnnotationKey = "ingressclass.kubernetes.io/is-default-class"
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

	// object Kinds are frequently used, do not change and are cached
	ingressKind      string
	ingressClassKind string
	secretKind       string
	serviceKind      string
}

type option func(ic *ingressController)

func WithControllerName(name string) option {
	return func(ic *ingressController) {
		ic.controllerName = name
	}
}

func WithAnnotationPrefix(prefix string) option {
	return func(ic *ingressController) {
		ic.annotationPrefix = prefix
	}
}

func WithNamespaces(ns []string) option {
	return func(ic *ingressController) {
		ic.namespaces = arrayToMap(ns)
	}
}

// PomeriumReconciler updates pomerium configuration based on provided network resources
// it is not expected to be thread safe
type PomeriumReconciler interface {
	// Upsert should update or create the pomerium routes corresponding to this ingress
	Upsert(ctx context.Context, ic *model.IngressConfig) (changes bool, err error)
	// Delete should delete pomerium routes corresponding to this ingress name
	Delete(ctx context.Context, namespacedName types.NamespacedName) error
	// DeleteAll wipes the configuration entirely
	DeleteAll(ctx context.Context) error
}

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *ingressController) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	logger.Info("Reconcile")

	ingress := new(networkingv1.Ingress)
	if err := r.Client.Get(ctx, req.NamespacedName, ingress); err != nil {
		if !apierrors.IsNotFound(err) {
			return ctrl.Result{Requeue: true}, fmt.Errorf("get ingress: %w", err)
		}
		logger.Info("gone")
		return r.deleteIngress(ctx, req.NamespacedName)
	}

	managing, err := r.isManaging(ctx, ingress)
	if err != nil {
		return ctrl.Result{Requeue: true}, fmt.Errorf("get ingressClass info: %w", err)
	}

	logger.Info("got ingress", "managing", managing, "version", ingress.GetResourceVersion())
	if !managing {
		return r.deleteIngress(ctx, req.NamespacedName)
	}

	ic, err := r.fetchIngress(ctx, ingress)
	if err != nil {
		logger.Error(err, "obtaining ingress related resources", "deps",
			r.Registry.Deps(model.Key{Kind: r.ingressKind, NamespacedName: req.NamespacedName}))
		return ctrl.Result{Requeue: true}, fmt.Errorf("fetch ingress related resources: %w", err)
	}

	return r.upsertIngress(ctx, ic)
}

func (r *ingressController) deleteIngress(ctx context.Context, name types.NamespacedName) (ctrl.Result, error) {
	if err := r.PomeriumReconciler.Delete(ctx, name); err != nil {
		return ctrl.Result{Requeue: true}, fmt.Errorf("deleting ingress: %w", err)
	}
	log.FromContext(ctx).Info("ingress deleted")
	r.Registry.DeleteCascade(model.Key{Kind: r.ingressKind, NamespacedName: name})
	return ctrl.Result{}, nil
}

func (r *ingressController) upsertIngress(ctx context.Context, ic *model.IngressConfig) (ctrl.Result, error) {
	changed, err := r.PomeriumReconciler.Upsert(ctx, ic)
	if err != nil {
		r.EventRecorder.Event(ic.Ingress, corev1.EventTypeWarning, "UpdatePomeriumConfig", err.Error())
		return ctrl.Result{Requeue: true}, fmt.Errorf("upsert: %w", err)
	}

	r.updateDependencies(ic)
	if changed {
		r.EventRecorder.Event(ic.Ingress, corev1.EventTypeNormal, "PomeriumConfigUpdated", "updated pomerium configuration")
	}
	log.FromContext(ctx).Info("upsertIngress", "deps", r.Deps(r.objectKey(ic.Ingress)), "spec", ic.Ingress.Spec, "changed", changed)

	return ctrl.Result{}, nil
}

// ObjectKey returns a registry key for a given kubernetes object
// the object must be properly initialized (GVK, name, namespace)
func (r *ingressController) objectKey(obj client.Object) model.Key {
	name := types.NamespacedName{Name: obj.GetName(), Namespace: obj.GetNamespace()}
	gvk, err := apiutil.GVKForObject(obj, r.Scheme)
	if err != nil {
		panic(err)
	}
	kind := gvk.Kind
	if kind == "" {
		panic("no kind available for object")
	}
	return model.Key{Kind: kind, NamespacedName: name}
}

func (r *ingressController) updateDependencies(ic *model.IngressConfig) {
	ingKey := r.objectKey(ic.Ingress)
	r.DeleteCascade(ingKey)

	for _, s := range ic.Secrets {
		r.Add(ingKey, r.objectKey(s))
	}
	for _, s := range ic.Services {
		r.Add(ingKey, r.objectKey(s))
	}
}

func (r *ingressController) isManaging(ctx context.Context, ing *networkingv1.Ingress) (bool, error) {
	icl := new(networkingv1.IngressClassList)
	if err := r.Client.List(ctx, icl); err != nil {
		return false, err
	}

	if len(r.namespaces) > 0 && !r.namespaces[ing.Namespace] {
		return false, nil
	}

	var className string
	if ing.Spec.IngressClassName != nil {
		className = *ing.Spec.IngressClassName
	}

	for _, ic := range icl.Items {
		if ic.Spec.Controller != r.controllerName {
			continue
		}
		if className == ic.Name {
			return true, nil
		}
		if strings.ToLower(ic.Annotations[IngressClassDefaultAnnotationKey]) == "true" {
			return true, nil
		}
	}
	return false, nil
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

// getDependantIngressFn returns for a given object kind (i.e. a secret) a function
// that would return ingress objects keys that depend from this object
func (r *ingressController) getDependantIngressFn(kind string) func(a client.Object) []reconcile.Request {
	logger := log.FromContext(context.Background()).WithValues("kind", kind)

	return func(a client.Object) []reconcile.Request {
		if len(r.namespaces) > 0 && !r.namespaces[a.GetNamespace()] {
			return nil
		}
		name := types.NamespacedName{Name: a.GetName(), Namespace: a.GetNamespace()}
		deps := r.DepsOfKind(model.Key{Kind: kind, NamespacedName: name}, r.ingressKind)
		reqs := make([]reconcile.Request, 0, len(deps))
		for _, k := range deps {
			reqs = append(reqs, reconcile.Request{NamespacedName: k.NamespacedName})
		}
		logger.Info("watch", "name", fmt.Sprintf("%s/%s", a.GetNamespace(), a.GetName()), "deps", reqs)
		return reqs
	}
}

func (r *ingressController) watchIngressClass(string) func(a client.Object) []reconcile.Request {
	logger := log.FromContext(context.Background())

	return func(a client.Object) []reconcile.Request {
		ic, ok := a.(*networkingv1.IngressClass)
		if !ok {
			logger.Error(fmt.Errorf("got %s", reflect.TypeOf(a)), "expected IngressClass")
			return nil
		}
		if ic.Spec.Controller != r.controllerName {
			return nil
		}
		il := new(networkingv1.IngressList)
		err := r.Client.List(context.Background(), il)
		if err != nil {
			logger.Error(err, "list")
			return nil
		}
		deps := make([]reconcile.Request, 0, len(il.Items))
		for i := range il.Items {
			deps = append(deps, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      il.Items[i].Name,
					Namespace: il.Items[i].Namespace,
				},
			})
		}
		logger.Info("watch", "deps", deps, "ingressClass", a.GetName())
		return deps
	}
}
