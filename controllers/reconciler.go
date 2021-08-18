package controllers

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

// ResourceWatcher watches a given resource to update
type Controller struct {
	client.Client
	PomeriumReconciler
	Registry
	ingressKind string
}

// PomeriumReconciler updates pomerium configuration based on provided network resources
// it is not expected to be thread safe
type PomeriumReconciler interface {
	// Upsert should update or create the pomerium routes corresponding to this ingress
	Upsert(ctx context.Context, ing *networkingv1.Ingress, tlsSecrets []*corev1.Secret, services map[types.NamespacedName]*corev1.Service) error
	// Delete should delete pomerium routes corresponding to this ingress name
	Delete(ctx context.Context, namespacedName types.NamespacedName) error
}

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *Controller) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	ing, tlsSecrets, services, err := fetchIngress(ctx, r.Client, req.NamespacedName)
	if err != nil {
		if !apierrors.IsNotFound(err) {
			return ctrl.Result{Requeue: true}, fmt.Errorf("fetch ingress and related resources: %w", err)
		}
		logger.Info("not found")
		if err := r.deleteIngress(ctx, req.NamespacedName); err != nil {
			return ctrl.Result{Requeue: true}, fmt.Errorf("deleting: %w", err)
		}
		return ctrl.Result{Requeue: false}, nil
	}

	if err := r.upsertIngress(ctx, ing, tlsSecrets, services); err != nil {
		return ctrl.Result{Requeue: true}, fmt.Errorf("upsert: %w", err)
	}
	logger.Info("updated", "uid", ing.UID, "version", ing.ResourceVersion, "deps", r.Registry.Deps(ObjectKey(ing)))
	return ctrl.Result{Requeue: false}, nil
}

func (r *Controller) deleteIngress(ctx context.Context, name types.NamespacedName) error {
	if err := r.PomeriumReconciler.Delete(ctx, name); err != nil {
		return err
	}
	r.Registry.DeleteCascade(Key{r.ingressKind, name})
	return nil
}

func (r *Controller) upsertIngress(ctx context.Context, ing *networkingv1.Ingress, secrets []*corev1.Secret, services map[types.NamespacedName]*corev1.Service) error {
	if err := r.PomeriumReconciler.Upsert(ctx, ing, secrets, services); err != nil {
		return fmt.Errorf("upsert: %w", err)
	}

	ingKey := ObjectKey(ing)
	for _, s := range secrets {
		r.Registry.Add(ingKey, ObjectKey(s))
	}
	for _, s := range services {
		r.Registry.Add(ingKey, ObjectKey(s))
	}

	return nil
}

// SetupWithManager sets up the controller with the Manager
func (r *Controller) SetupWithManager(mgr ctrl.Manager) error {
	c, err := ctrl.NewControllerManagedBy(mgr).
		For(&networkingv1.Ingress{}).
		Build(r)
	if err != nil {
		return err
	}

	gvk, err := apiutil.GVKForObject(&networkingv1.Ingress{}, mgr.GetScheme())
	if err != nil {
		return fmt.Errorf("cannot get ingress kind: %w", err)
	}
	r.ingressKind = gvk.Kind

	for _, obj := range []client.Object{
		&corev1.Service{},
		&corev1.Secret{},
	} {
		gvk, err = apiutil.GVKForObject(obj, mgr.GetScheme())
		if err != nil {
			return fmt.Errorf("cannot get object kind: %w", err)
		}
		if err := c.Watch(
			&source.Kind{Type: obj},
			handler.EnqueueRequestsFromMapFunc(r.getDependantIngressFn(gvk.Kind))); err != nil {
			return err
		}
	}

	return nil
}

// getDependantIngressFn returns for a given object kind (i.e. a secret) a function
// that would return ingress objects keys that depend from this object
func (r Controller) getDependantIngressFn(kind string) func(a client.Object) []reconcile.Request {
	return func(a client.Object) []reconcile.Request {
		name := types.NamespacedName{Name: a.GetName(), Namespace: a.GetNamespace()}
		deps := r.DepsOfKind(Key{Kind: kind, NamespacedName: name}, r.ingressKind)
		reqs := make([]reconcile.Request, 0, len(deps))
		for _, k := range deps {
			reqs = append(reqs, reconcile.Request{NamespacedName: k.NamespacedName})
		}
		return reqs
	}
}
