package ingress

import (
	"context"
	"fmt"

	networkingv1 "k8s.io/api/networking/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/pomerium/ingress-controller/model"
)

func (r *ingressController) flushReconcileBatch(ctx context.Context) error {
	if err := r.initComplete.yield(ctx); err != nil {
		return fmt.Errorf("initial reconciliation: %w", err)
	}
	_, _, err := r.reconcileAll(ctx)
	return err
}

func (r *ingressController) reconcileBatched(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	ingress := new(networkingv1.Ingress)
	if err := r.Client.Get(ctx, req.NamespacedName, ingress); err != nil {
		if !apierrors.IsNotFound(err) {
			return ctrl.Result{Requeue: true}, fmt.Errorf("get ingress: %w", err)
		}
		return r.reconcileBatchedDelete(ctx, req.NamespacedName, reasonIngressDeleted)
	} else if ingress.DeletionTimestamp != nil {
		return r.reconcileBatchedDelete(ctx, req.NamespacedName, reasonIngressDeleted)
	}

	managing, err := r.isManaging(ctx, ingress)
	if err != nil {
		return ctrl.Result{Requeue: true}, fmt.Errorf("get ingressClass info: %w", err)
	}
	if !managing.managed {
		return r.reconcileBatchedDelete(ctx, req.NamespacedName, managing.reasonIfNot)
	}
	r.managedIngresses.Store(req.NamespacedName, struct{}{})

	ic, err := r.fetchIngress(ctx, ingress)
	if err != nil {
		r.IngressNotReconciled(ctx, ingress, err)
		return ctrl.Result{Requeue: true}, fmt.Errorf("fetch ingress related resources: %w", err)
	}

	needsUpdate, err := r.ingressChangeDetector.NeedsIngressUpdate(ic)
	if err != nil {
		return ctrl.Result{Requeue: true}, fmt.Errorf("detect ingress change: %w", err)
	}
	if needsUpdate {
		if err := r.batchCoordinator.Submit(ctx); err != nil {
			r.IngressNotReconciled(ctx, ingress, err)
			return ctrl.Result{Requeue: true}, fmt.Errorf("apply ingress batch: %w", err)
		}
	}

	statusChanged, err := r.updateIngressStatus(ctx, ingress)
	if err != nil {
		return ctrl.Result{Requeue: true}, fmt.Errorf("update ingress status: %w", err)
	}
	if needsUpdate || statusChanged {
		r.IngressReconciled(ctx, ingress)
	}
	return ctrl.Result{}, nil
}

func (r *ingressController) reconcileBatchedDelete(
	ctx context.Context,
	name types.NamespacedName,
	reason string,
) (ctrl.Result, error) {
	tracked := r.ingressChangeDetector.TracksIngress(name)
	_, knownManaged := r.managedIngresses.Load(name)
	if tracked {
		if err := r.batchCoordinator.Submit(ctx); err != nil {
			return ctrl.Result{Requeue: true}, fmt.Errorf("apply ingress deletion batch: %w", err)
		}
	}
	if tracked || knownManaged {
		r.IngressDeleted(ctx, name, reason)
	}
	r.managedIngresses.Delete(name)
	r.DeleteCascade(model.Key{Kind: r.ingressKind, NamespacedName: name})
	return ctrl.Result{}, nil
}
