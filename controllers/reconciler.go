package controllers

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// PomeriumReconciler updates pomerium configuration based on provided network resources
// it is not expected to be thread safe
type PomeriumReconciler interface {
	// Upsert should update or create the pomerium routes corresponding to this ingress
	Upsert(ctx context.Context, ing *networkingv1.Ingress, tlsSecrets []*TLSSecret, services map[types.NamespacedName]*corev1.Service) error
	// Delete should delete pomerium routes corresponding to this ingress name
	Delete(ctx context.Context, namespacedName types.NamespacedName) error
}

type ResourceReconciler interface {
	// Reconcile attempts to perform configuration reconciliation
	// and may return an error in case information is currently incomplete
	Reconcile(ctx context.Context, kind string, name types.NamespacedName) error
}

type reconciler struct {
	client.Client
	PomeriumReconciler
	Registry
}

func NewReconciler(c client.Client, cr PomeriumReconciler, r Registry) ResourceReconciler {
	return &reconciler{
		Client:             c,
		PomeriumReconciler: cr,
		Registry:           r,
	}
}

func (r reconciler) Reconcile(ctx context.Context, kind string, name types.NamespacedName) error {
	logger := log.FromContext(ctx)
	logger.Info("REQUEST", "kind", kind)
	if kind != "Ingress" { // TODO: change to GVK
		return nil
	}

	return r.reconcileIngress(ctx, name)
}

func (r reconciler) reconcileIngress(ctx context.Context, name types.NamespacedName) error {
	logger := log.FromContext(ctx)
	ing, tlsSecrets, services, err := fetchIngress(ctx, r.Client, name)
	if err != nil {
		if !apierrors.IsNotFound(err) {
			return fmt.Errorf("fetch ingress and related resources: %w", err)
		}
		logger.Info("not found", "name", name)
		if err := r.PomeriumReconciler.Delete(ctx, name); err != nil {
			return fmt.Errorf("deleting: %w", err)
		}
		return nil
	}

	if err := r.PomeriumReconciler.Upsert(ctx, ing, tlsSecrets, services); err != nil {
		return fmt.Errorf("upsert: %w", err)
	}
	logger.Info("updated", "uid", ing.UID, "version", ing.ResourceVersion)
	return nil
}
