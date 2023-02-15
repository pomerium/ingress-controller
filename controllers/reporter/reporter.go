package reporter

import (
	"context"

	"github.com/hashicorp/go-multierror"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/log"

	icsv1 "github.com/pomerium/ingress-controller/apis/ingress/v1"
)

// MultiIngressStatusReporter dispatches updates over multiple reporters
type MultiIngressStatusReporter []IngressStatusReporter

// MultiPomeriumStatusReporter dispatches updates over multiple reporters
type MultiPomeriumStatusReporter []PomeriumReporter

func logErrorIfAny(ctx context.Context, err error, kvs ...any) {
	if err == nil {
		return
	}
	log.FromContext(ctx).Error(err, "posting status updates", kvs...)
}

// IngressReconciled an ingress was successfully reconciled with Pomerium
func (r MultiIngressStatusReporter) IngressReconciled(ctx context.Context, ingress *networkingv1.Ingress) {
	var errs *multierror.Error
	for _, u := range r {
		if err := u.IngressReconciled(ctx, ingress); err != nil {
			errs = multierror.Append(errs, err)
		}
	}
	logErrorIfAny(ctx, errs.ErrorOrNil(), "ingress", types.NamespacedName{Namespace: ingress.Namespace, Name: ingress.Name})
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
	logErrorIfAny(ctx, errs.ErrorOrNil(), "ingress", types.NamespacedName{Namespace: ingress.Namespace, Name: ingress.Name})
}

// IngressDeleted an ingress resource was deleted and Pomerium no longer serves it
func (r MultiIngressStatusReporter) IngressDeleted(ctx context.Context, name types.NamespacedName, reason string) {
	var errs *multierror.Error
	for _, u := range r {
		if err := u.IngressDeleted(ctx, name, reason); err != nil {
			errs = multierror.Append(errs, err)
		}
	}
	logErrorIfAny(ctx, errs.ErrorOrNil(), "ingress", name, "original reason", reason)
}

// SettingsUpdated marks that configuration was reconciled
func (r MultiPomeriumStatusReporter) SettingsUpdated(ctx context.Context, obj *icsv1.Pomerium) {
	var errs *multierror.Error
	for _, u := range r {
		if err := u.SettingsUpdated(ctx, obj); err != nil {
			errs = multierror.Append(errs, err)
		}
	}
	logErrorIfAny(ctx, errs.ErrorOrNil())
}

// SettingsRejected marks that configuration was reconciled
func (r MultiPomeriumStatusReporter) SettingsRejected(ctx context.Context, obj *icsv1.Pomerium, err error) {
	var errs *multierror.Error
	for _, u := range r {
		if err := u.SettingsRejected(ctx, obj, err); err != nil {
			errs = multierror.Append(errs, err)
		}
	}
	logErrorIfAny(ctx, errs.ErrorOrNil())
}
