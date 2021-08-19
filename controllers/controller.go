package controllers

import (
	"context"
	"errors"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	"github.com/pomerium/ingress-controller/model"
)

// Controller watches ingress and related resources for updates and reconciles with pomerium
type Controller struct {
	client.Client
	PomeriumReconciler
	model.Registry
	ingressKind string
	secretKind  string
	serviceKind string
}

// PomeriumReconciler updates pomerium configuration based on provided network resources
// it is not expected to be thread safe
type PomeriumReconciler interface {
	// Upsert should update or create the pomerium routes corresponding to this ingress
	Upsert(ctx context.Context, ic *model.IngressConfig) error
	// Delete should delete pomerium routes corresponding to this ingress name
	Delete(ctx context.Context, namespacedName types.NamespacedName) error
}

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *Controller) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	ic, err := r.fetchIngress(ctx, req.NamespacedName)
	if err != nil {
		if !r.isIngressNotFound(err) {
			logger.Error(err, "obtaining ingress related resources", "deps",
				r.Registry.Deps(model.Key{Kind: r.ingressKind, NamespacedName: req.NamespacedName}))
			return ctrl.Result{Requeue: true}, fmt.Errorf("fetch ingress related resources: %w", err)
		}
		logger.Info("not found")
		if err := r.deleteIngress(ctx, req.NamespacedName); err != nil {
			return ctrl.Result{Requeue: true}, fmt.Errorf("deleting: %w", err)
		}
		return ctrl.Result{Requeue: false}, nil
	}

	if err := r.upsertIngress(ctx, ic); err != nil {
		return ctrl.Result{Requeue: true}, fmt.Errorf("upsert: %w", err)
	}
	logger.Info("updated", "deps", r.Registry.Deps(model.ObjectKey(ic.Ingress)))
	return ctrl.Result{Requeue: false}, nil
}

func (r *Controller) deleteIngress(ctx context.Context, name types.NamespacedName) error {
	if err := r.PomeriumReconciler.Delete(ctx, name); err != nil {
		return err
	}
	r.Registry.DeleteCascade(model.Key{Kind: r.ingressKind, NamespacedName: name})
	return nil
}

func (r *Controller) upsertIngress(ctx context.Context, ic *model.IngressConfig) error {
	if err := r.PomeriumReconciler.Upsert(ctx, ic); err != nil {
		return fmt.Errorf("upsert: %w", err)
	}

	ic.UpdateDependencies(r.Registry)
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

	scheme := mgr.GetScheme()
	for _, o := range []struct {
		client.Object
		kind  *string
		watch bool
	}{
		{&networkingv1.Ingress{}, &r.ingressKind, false},
		{&corev1.Secret{}, &r.secretKind, true},
		{&corev1.Service{}, &r.serviceKind, true},
	} {
		gvk, err := apiutil.GVKForObject(o.Object, scheme)
		if err != nil {
			return fmt.Errorf("cannot get kind: %w", err)
		}
		*o.kind = gvk.Kind

		if !o.watch {
			continue
		}

		if err := c.Watch(
			&source.Kind{Type: o.Object},
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
		deps := r.DepsOfKind(model.Key{Kind: kind, NamespacedName: name}, r.ingressKind)
		reqs := make([]reconcile.Request, 0, len(deps))
		for _, k := range deps {
			reqs = append(reqs, reconcile.Request{NamespacedName: k.NamespacedName})
		}
		return reqs
	}
}

func (r Controller) isIngressNotFound(err error) bool {
	if status := apierrors.APIStatus(nil); errors.As(err, &status) {
		s := status.Status()
		return s.Reason == metav1.StatusReasonNotFound &&
			s.Details != nil &&
			s.Details.Kind == r.ingressKind
	}
	return false
}
