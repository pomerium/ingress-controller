package ingress

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/pomerium/ingress-controller/model"
)

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

	ics, active, err := r.reconcileAll(ctx)
	if err == nil {
		// Prune before reporting each active Ingress. On large clusters those
		// status updates can consume most of the initial reconciliation timeout,
		// leaving no usable context for stale-entry cleanup.
		r.PruneIngressStatuses(ctx, active)
	}
	for i := range ics {
		ingress := ics[i].Ingress
		if err != nil {
			r.IngressNotReconciled(ctx, ingress, err)
		} else if _, statusErr := r.updateIngressStatus(ctx, ingress); statusErr != nil {
			r.IngressNotReconciled(ctx, ingress, fmt.Errorf("update /status: %w", statusErr))
		} else {
			r.IngressReconciled(ctx, ingress)
		}
	}
	return err
}

// reconcileAll reads the current managed Ingress state and persists it with a
// single full-state Set call. It deliberately does not update statuses; queued
// reconcile requests do that after the batch has been applied.
func (r *ingressController) reconcileAll(ctx context.Context) ([]*model.IngressConfig, map[string]struct{}, error) {
	logger := log.FromContext(ctx)
	ingressList := new(networkingv1.IngressList)
	if err := r.Client.List(ctx, ingressList); err != nil {
		return nil, nil, fmt.Errorf("list ingresses: %w", err)
	}

	var ics []*model.IngressConfig
	active := make(map[string]struct{})
	for i := range ingressList.Items {
		ingress := &ingressList.Items[i]
		res, err := r.isManaging(ctx, ingress)
		if err != nil {
			return nil, nil, fmt.Errorf("get ingressClass info: %w", err)
		}
		if !res.managed {
			logger.V(1).Info("skipping ingress", "ingress", ingress.Name, "reason", res.reasonIfNot)
			continue
		}
		name := types.NamespacedName{Namespace: ingress.Namespace, Name: ingress.Name}
		r.managedIngresses.Store(name, struct{}{})
		active[name.String()] = struct{}{}
		ic, err := r.fetchIngress(ctx, ingress)
		if err != nil {
			r.IngressNotReconciled(ctx, ingress, err)
			// Do not persist a partial full-state configuration. A dependency may
			// be temporarily unavailable while informer caches converge; omitting
			// its Ingress here would remove a previously valid route. Returning the
			// error preserves the last applied configuration and lets
			// controller-runtime retry the batch.
			return ics, active, fmt.Errorf("fetch ingress %s dependencies: %w", name, err)
		}
		logger.V(1).Info("fetch", "ingress", ingress.Name, "secrets", len(ic.Secrets), "services", len(ic.Services))
		ics = append(ics, ic)
	}

	_, err := r.IngressReconciler.Set(ctx, ics)
	return ics, active, err
}

const reasonIngressDeleted = "Ingress resource was deleted"

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *ingressController) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	if err := r.initComplete.yield(ctx); err != nil {
		return ctrl.Result{Requeue: true}, fmt.Errorf("initial reconciliation: %w", err)
	}
	if r.batchCoordinator != nil {
		return r.reconcileBatched(ctx, req)
	}

	ingress := new(networkingv1.Ingress)
	if err := r.Client.Get(ctx, req.NamespacedName, ingress); err != nil {
		if !apierrors.IsNotFound(err) {
			return ctrl.Result{Requeue: true}, fmt.Errorf("get ingress: %w", err)
		}
		return r.deleteIngress(ctx, req.NamespacedName, reasonIngressDeleted)
	} else if ingress.DeletionTimestamp != nil {
		return r.deleteIngress(ctx, req.NamespacedName, reasonIngressDeleted)
	}

	managing, err := r.isManaging(ctx, ingress)
	if err != nil {
		return ctrl.Result{Requeue: true}, fmt.Errorf("get ingressClass info: %w", err)
	}

	if !managing.managed {
		return r.deleteIngress(ctx, req.NamespacedName, managing.reasonIfNot)
	}
	r.managedIngresses.Store(req.NamespacedName, struct{}{})

	ic, err := r.fetchIngress(ctx, ingress)
	if err != nil {
		r.IngressNotReconciled(ctx, ingress, err)
		return ctrl.Result{Requeue: true}, fmt.Errorf("fetch ingress related resources: %w", err)
	}

	res, err := r.upsertIngress(ctx, ic)
	if err != nil {
		return res, fmt.Errorf("upsert ingress: %w", err)
	}
	return res, nil
}

func (r *ingressController) deleteIngress(ctx context.Context, name types.NamespacedName, reason string) (ctrl.Result, error) {
	_, knownManaged := r.managedIngresses.Load(name)
	changed, err := r.IngressReconciler.Delete(ctx, name)
	if err != nil {
		return ctrl.Result{Requeue: true}, fmt.Errorf("deleting ingress: %w", err)
	}
	if changed || knownManaged {
		r.IngressDeleted(ctx, name, reason)
	}
	r.managedIngresses.Delete(name)
	r.DeleteCascade(model.Key{Kind: r.ingressKind, NamespacedName: name})
	return ctrl.Result{}, nil
}

func (r *ingressController) upsertIngress(ctx context.Context, ic *model.IngressConfig) (ctrl.Result, error) {
	_, err := r.IngressReconciler.Upsert(ctx, ic)
	if err != nil {
		r.IngressNotReconciled(ctx, ic.Ingress, err)
		return ctrl.Result{Requeue: true}, fmt.Errorf("upsert: %w", err)
	}

	r.IngressReconciled(ctx, ic.Ingress)

	if _, err = r.updateIngressStatus(ctx, ic.Ingress); err != nil {
		return ctrl.Result{Requeue: true}, fmt.Errorf("update ingress status: %w", err)
	}

	return ctrl.Result{}, nil
}

func (r *ingressController) updateIngressStatus(ctx context.Context, ingress *networkingv1.Ingress) (bool, error) {
	if r.updateStatusFromService == nil {
		return false, nil
	}

	svc := new(corev1.Service)
	if err := r.Client.Get(ctx, *r.updateStatusFromService, svc); err != nil {
		return false, fmt.Errorf("get pomerium-proxy service %s: %w", r.updateStatusFromService.String(), err)
	}

	desired := networkingv1.IngressLoadBalancerStatus{
		Ingress: svcStatusToIngress(svc),
	}
	if apiequality.Semantic.DeepEqual(ingress.Status.LoadBalancer, desired) {
		return false, nil
	}
	ingress.Status.LoadBalancer = desired

	if err := r.Client.Status().Update(ctx, ingress); err != nil {
		return false, err
	}
	return true, nil
}

func svcStatusToIngress(svc *corev1.Service) []networkingv1.IngressLoadBalancerIngress {
	switch svc.Spec.Type {
	case corev1.ServiceTypeLoadBalancer:
		src := svc.Status.LoadBalancer.Ingress
		dst := make([]networkingv1.IngressLoadBalancerIngress, len(src))
		for i := range src {
			dst[i] = networkingv1.IngressLoadBalancerIngress{
				Hostname: src[i].Hostname,
				IP:       src[i].IP,
				Ports:    svcPortToIngress(src[i].Ports),
			}
		}
		return dst
	case corev1.ServiceTypeNodePort:
		return []networkingv1.IngressLoadBalancerIngress{{
			IP: svc.Spec.ClusterIP,
		}}
	default:
		return nil
	}
}

func svcPortToIngress(src []corev1.PortStatus) []networkingv1.IngressPortStatus {
	dst := make([]networkingv1.IngressPortStatus, len(src))
	for i := range src {
		dst[i] = networkingv1.IngressPortStatus{
			Protocol: src[i].Protocol,
			Port:     src[i].Port,
			Error:    src[i].Error,
		}
	}
	return dst
}
