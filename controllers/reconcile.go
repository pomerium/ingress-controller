package controllers

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/pomerium/ingress-controller/model"
)

// PomeriumReconciler updates pomerium configuration based on provided network resources
// it is not expected to be thread safe
type PomeriumReconciler interface {
	// Upsert should update or create the pomerium routes corresponding to this ingress
	Upsert(ctx context.Context, ic *model.IngressConfig) (changes bool, err error)
	// Set configuration to match provided ingresses
	Set(ctx context.Context, ics []*model.IngressConfig) (changes bool, err error)
	// Delete should delete pomerium routes corresponding to this ingress name
	Delete(ctx context.Context, namespacedName types.NamespacedName) error
}

// reconcileInitial walks over all ingresses and updates configuration at once
// this is currently done for performance reasons
func (r *ingressController) reconcileInitial(ctx context.Context) (err error) {
	logger := log.FromContext(ctx).WithName("initial-sync")
	logger.Info("starting...")
	defer func() {
		if err != nil {
			logger.Error(err, "completed with error")
		} else {
			logger.Info("complete")
		}
	}()

	ingressList := new(networkingv1.IngressList)
	if err := r.Client.List(ctx, ingressList); err != nil {
		return fmt.Errorf("list ingresses: %w", err)
	}

	var ics []*model.IngressConfig
	for i := range ingressList.Items {
		ingress := &ingressList.Items[i]
		managing, err := r.isManaging(ctx, ingress)
		if err != nil {
			return fmt.Errorf("get ingressClass info: %w", err)
		}
		if !managing {
			continue
		}
		ic, err := r.fetchIngress(ctx, ingress)
		if err != nil {
			return fmt.Errorf("fetch ingress %s/%s: %w", ingress.Namespace, ingress.Name, err)
		}
		logger.V(1).Info("fetch", "ingress", ingress.Name, "secrets", len(ic.Secrets), "services", len(ic.Services))
		ics = append(ics, ic)
	}

	changed, err := r.PomeriumReconciler.Set(ctx, ics)
	for i := range ingressList.Items {
		ingress := &ingressList.Items[i]
		if err != nil {
			r.EventRecorder.Event(ingress, corev1.EventTypeWarning, reasonPomeriumConfigUpdateError, err.Error())
		} else if changed {
			r.EventRecorder.Event(ingress, corev1.EventTypeNormal, reasonPomeriumConfigUpdated, msgPomeriumConfigUpdated)
		}
	}

	return err
}

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *ingressController) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	if err := r.initComplete.yield(ctx); err != nil {
		return ctrl.Result{Requeue: true}, fmt.Errorf("initial reconciliation: %w", err)
	}

	logger := log.FromContext(ctx)
	ingress := new(networkingv1.Ingress)
	if err := r.Client.Get(ctx, req.NamespacedName, ingress); err != nil {
		if !apierrors.IsNotFound(err) {
			return ctrl.Result{Requeue: true}, fmt.Errorf("get ingress: %w", err)
		}
		return r.deleteIngress(ctx, req.NamespacedName, "Ingress resource was deleted")
	}

	managing, err := r.isManaging(ctx, ingress)
	if err != nil {
		return ctrl.Result{Requeue: true}, fmt.Errorf("get ingressClass info: %w", err)
	}

	if !managing {
		return r.deleteIngress(ctx, req.NamespacedName, "not marked to be managed by this controller")
	}

	ic, err := r.fetchIngress(ctx, ingress)
	if err != nil {
		logger.Error(err, "obtaining ingress related resources", "deps",
			r.Registry.Deps(model.Key{Kind: r.ingressKind, NamespacedName: req.NamespacedName}))
		return ctrl.Result{Requeue: true}, fmt.Errorf("fetch ingress related resources: %w", err)
	}

	return r.upsertIngress(ctx, ic)
}

func (r *ingressController) deleteIngress(ctx context.Context, name types.NamespacedName, reason string) (ctrl.Result, error) {
	if err := r.PomeriumReconciler.Delete(ctx, name); err != nil {
		return ctrl.Result{Requeue: true}, fmt.Errorf("deleting ingress: %w", err)
	}
	log.FromContext(ctx).Info("deleted from pomerium", "reason", reason)
	r.Registry.DeleteCascade(model.Key{Kind: r.ingressKind, NamespacedName: name})
	return ctrl.Result{}, nil
}

func (r *ingressController) upsertIngress(ctx context.Context, ic *model.IngressConfig) (ctrl.Result, error) {
	changed, err := r.PomeriumReconciler.Upsert(ctx, ic)
	if err != nil {
		r.EventRecorder.Event(ic.Ingress, corev1.EventTypeWarning, reasonPomeriumConfigUpdateError, err.Error())
		return ctrl.Result{Requeue: true}, fmt.Errorf("upsert: %w", err)
	}

	r.updateDependencies(ic)
	if changed {
		log.FromContext(ctx).V(1).Info("ingress updated", "deps", r.Deps(r.objectKey(ic.Ingress)), "spec", ic.Ingress.Spec, "changed", changed)
		r.EventRecorder.Event(ic.Ingress, corev1.EventTypeNormal, reasonPomeriumConfigUpdated, msgPomeriumConfigUpdated)
	}

	if err = r.updateIngressStatus(ctx, ic.Ingress); err != nil {
		return ctrl.Result{Requeue: true}, fmt.Errorf("update ingress status: %w", err)
	}

	return ctrl.Result{}, nil
}

func (r *ingressController) updateIngressStatus(ctx context.Context, ingress *networkingv1.Ingress) error {
	if r.updateStatusFromService == nil {
		return nil
	}

	svc := new(corev1.Service)
	if err := r.Client.Get(ctx, *r.updateStatusFromService, svc); err != nil {
		return fmt.Errorf("get pomerium-proxy service %s: %w", r.updateStatusFromService.String(), err)
	}

	ingress.Status.LoadBalancer = svc.Status.LoadBalancer
	return r.Client.Status().Update(ctx, ingress)
}
