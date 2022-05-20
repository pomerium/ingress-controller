package controllers

import (
	"context"

	"github.com/hashicorp/go-multierror"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// IngressStatusReporter updates status of ingress objects
type IngressStatusReporter interface {
	// IngressReconciled an ingress was successfully reconciled with Pomerium
	IngressReconciled(ctx context.Context, ingress *networkingv1.Ingress) error
	// IngressNotReconciled an updated ingress resource was received,
	// however it could not be reconciled with Pomerium due to errors
	IngressNotReconciled(ctx context.Context, ingress *networkingv1.Ingress, reason error) error
	// IngressDeleted an ingress resource was deleted and Pomerium no longer serves it
	IngressDeleted(ctx context.Context, name types.NamespacedName, reason string) error
}

// MultiIngressStatusReporter dispatches updates over multiple reporters
type MultiIngressStatusReporter []IngressStatusReporter

func (r MultiIngressStatusReporter) reportErrorIfAny(ctx context.Context, err error, name types.NamespacedName) {
	if err == nil {
		return
	}
	log.FromContext(ctx).Error(err, "updating ingress status", "ingress", name)
}

// IngressReconciled an ingress was successfully reconciled with Pomerium
func (r MultiIngressStatusReporter) IngressReconciled(ctx context.Context, ingress *networkingv1.Ingress) {
	var errs *multierror.Error
	for _, u := range r {
		if err := u.IngressReconciled(ctx, ingress); err != nil {
			errs = multierror.Append(errs, err)
		}
	}
	r.reportErrorIfAny(ctx, errs.ErrorOrNil(), types.NamespacedName{Namespace: ingress.Namespace, Name: ingress.Name})
}

// IngressNotReconciled an updated ingress resource was received,
// however it could not be reconciled with Pomerium due to errors
func (r MultiIngressStatusReporter) IngressNotReconciled(ctx context.Context, ingress *networkingv1.Ingress, reason error) {
	var errs *multierror.Error
	for _, u := range r {
		if err := u.IngressNotReconciled(ctx, ingress, reason); err != nil {
			errs = multierror.Append(errs, err)
		}
	}
	r.reportErrorIfAny(ctx, errs.ErrorOrNil(), types.NamespacedName{Namespace: ingress.Namespace, Name: ingress.Name})
}

// IngressDeleted an ingress resource was deleted and Pomerium no longer serves it
func (r MultiIngressStatusReporter) IngressDeleted(ctx context.Context, name types.NamespacedName, reason string) {
	var errs *multierror.Error
	for _, u := range r {
		if err := u.IngressDeleted(ctx, name, reason); err != nil {
			errs = multierror.Append(errs, err)
		}
	}
	r.reportErrorIfAny(ctx, errs, name)
}
