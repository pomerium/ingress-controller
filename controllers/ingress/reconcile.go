package ingress

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
		res, err := r.isManaging(ctx, ingress)
		if err != nil {
			return fmt.Errorf("get ingressClass info: %w", err)
		}
		if !res.managed {
			logger.V(1).Info("skipping ingress", "ingress", ingress.Name, "reason", res.reasonIfNot)
			continue
		}
		ic, err := r.fetchIngress(ctx, ingress)
		if err != nil {
			return fmt.Errorf("fetch ingress %s/%s: %w", ingress.Namespace, ingress.Name, err)
		}
		logger.V(1).Info("fetch", "ingress", ingress.Name, "secrets", len(ic.Secrets), "services", len(ic.Services))
		ics = append(ics, ic)
	}

	_, err = r.IngressReconciler.Set(ctx, ics)
	for i := range ics {
		ingress := ics[i].Ingress
		if err != nil {
			r.IngressNotReconciled(ctx, ingress, err)
		} else if err := r.updateIngressStatus(ctx, ingress); err != nil {
			r.IngressNotReconciled(ctx, ingress, fmt.Errorf("update /status: %w", err))
		} else {
			r.IngressReconciled(ctx, ingress)
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

	if !managing.managed {
		return r.deleteIngress(ctx, req.NamespacedName, managing.reasonIfNot)
	}

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
	changed, err := r.IngressReconciler.Delete(ctx, name)
	if err != nil {
		return ctrl.Result{Requeue: true}, fmt.Errorf("deleting ingress: %w", err)
	}
	if changed {
		r.IngressDeleted(ctx, name, reason)
	}
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

	ingress.Status.LoadBalancer = networkingv1.IngressLoadBalancerStatus{}

	if len(svc.Status.LoadBalancer.Ingress) > 0 {
		ingress.Status.LoadBalancer.Ingress = svcLoadBalancerStatusToIngress(svc.Status.LoadBalancer.Ingress)
	} else if svc.Spec.Type == corev1.ServiceTypeNodePort {
		// Assign ClusterIP for NodePort service
		if svc.Spec.ClusterIP != "" {
			ingress.Status.LoadBalancer.Ingress = append(ingress.Status.LoadBalancer.Ingress, networkingv1.IngressLoadBalancerIngress{
				IP: svc.Spec.ClusterIP,
			})
		}
	}

	return r.Client.Status().Update(ctx, ingress)
}

func svcLoadBalancerStatusToIngress(src []corev1.LoadBalancerIngress) []networkingv1.IngressLoadBalancerIngress {
	dst := make([]networkingv1.IngressLoadBalancerIngress, len(src))
	for i := range src {
		dst[i] = networkingv1.IngressLoadBalancerIngress{
			Hostname: src[i].Hostname,
			IP:       src[i].IP,
			Ports:    svcPortToIngress(src[i].Ports),
		}
	}
	return dst
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
