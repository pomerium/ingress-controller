package controllers

import (
	"context"
	"fmt"
	"time"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
)

const (
	retryDuration = time.Second * 30
)

// ResourceWatcher watches a given resource to update
type ResourceWatcher struct {
	kind string
	ResourceReconciler
}

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *ResourceWatcher) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	if err := r.ResourceReconciler.Reconcile(ctx, r.kind, req.NamespacedName); err != nil {
		return ctrl.Result{RequeueAfter: retryDuration}, err
	}
	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager
func (r *ResourceWatcher) SetupWithManager(mgr ctrl.Manager, obj client.Object) error {
	gkv, err := apiutil.GVKForObject(obj, mgr.GetScheme())
	if err != nil {
		return fmt.Errorf("could not determine gkv from object: %w", err)
	}
	r.kind = gkv.Kind

	return ctrl.NewControllerManagedBy(mgr).
		For(obj).
		Complete(r)
}
