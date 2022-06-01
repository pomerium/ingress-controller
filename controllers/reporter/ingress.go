// Package reporter contains various methods to report status updates
package reporter

import (
	"context"
	"time"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/log"

	icsv1 "github.com/pomerium/ingress-controller/apis/ingress/v1"
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

// IngressSettingsReporter reflects ingress updates in a Pomerium Settings CRD /status section
type IngressSettingsReporter struct {
	SettingsReporter
}

// IngressReconciled an ingress was successfully reconciled with Pomerium
func (r *IngressSettingsReporter) IngressReconciled(ctx context.Context, ingress *networkingv1.Ingress) error {
	settings, err := r.getSettings(ctx)
	if err != nil {
		return err
	}

	settings.Status.Routes[types.NamespacedName{Namespace: ingress.Namespace, Name: ingress.Name}.String()] =
		icsv1.RouteStatus{
			LastReconciled: &metav1.Time{Time: time.Now()},
			Reconciled:     true,
		}

	return r.Client.Status().Update(ctx, settings)
}

// IngressNotReconciled an updated ingress resource was received,
// however it could not be reconciled with Pomerium due to errors
func (r *IngressSettingsReporter) IngressNotReconciled(ctx context.Context, ingress *networkingv1.Ingress, reason error) error {
	settings, err := r.getSettings(ctx)
	if err != nil {
		return err
	}

	key := types.NamespacedName{Namespace: ingress.Namespace, Name: ingress.Name}.String()
	route := settings.Status.Routes[key]
	route.Reconciled = false
	route.Error = reason.Error()
	settings.Status.Routes[key] = route

	return r.Client.Status().Update(ctx, settings)
}

// IngressDeleted an ingress resource was deleted and Pomerium no longer serves it
func (r *IngressSettingsReporter) IngressDeleted(ctx context.Context, name types.NamespacedName, reason string) error {
	settings, err := r.getSettings(ctx)
	if err != nil {
		return err
	}

	delete(settings.Status.Routes, name.String())

	return r.Client.Status().Update(ctx, settings)
}

// IngressEventReporter reflects updates as events posted to the ingress
type IngressEventReporter struct {
	record.EventRecorder
}

const (
	reasonPomeriumConfigUpdated     = "Updated"
	reasonPomeriumConfigUpdateError = "UpdateError"
	msgPomeriumConfigUpdated        = "Pomerium configuration updated"
	msgIngressDeleted               = "deleted from Pomerium"
)

// IngressReconciled an ingress was successfully reconciled with Pomerium
func (r *IngressEventReporter) IngressReconciled(ctx context.Context, ingress *networkingv1.Ingress) error {
	r.EventRecorder.Event(ingress, corev1.EventTypeNormal, reasonPomeriumConfigUpdated, msgPomeriumConfigUpdated)
	return nil
}

// IngressNotReconciled an updated ingress resource was received,
// however it could not be reconciled with Pomerium due to errors
func (r *IngressEventReporter) IngressNotReconciled(ctx context.Context, ingress *networkingv1.Ingress, reason error) error {
	r.EventRecorder.Event(ingress, corev1.EventTypeWarning, reasonPomeriumConfigUpdateError, reason.Error())
	return nil
}

// IngressDeleted an ingress resource was deleted and Pomerium no longer serves it
func (r *IngressEventReporter) IngressDeleted(ctx context.Context, name types.NamespacedName, reason string) error {
	return nil
}

// IngressSettingsEventReporter posts ingress updates as events to Settings CRD
type IngressSettingsEventReporter struct {
	SettingsReporter
	record.EventRecorder
}

func (r *IngressSettingsEventReporter) postEvent(ctx context.Context, ingress types.NamespacedName, eventType, reason, msg string) error {
	settings, err := r.getSettings(ctx)
	if err != nil {
		return err
	}
	r.EventRecorder.AnnotatedEventf(settings,
		map[string]string{"ingress": ingress.String()},
		eventType, reason, "%s: %s", ingress.String(), msg)
	return nil
}

// IngressReconciled an ingress was successfully reconciled with Pomerium
func (r *IngressSettingsEventReporter) IngressReconciled(ctx context.Context, ingress *networkingv1.Ingress) error {
	return r.postEvent(ctx, types.NamespacedName{Name: ingress.Name, Namespace: ingress.Namespace},
		corev1.EventTypeNormal, reasonPomeriumConfigUpdated, msgPomeriumConfigUpdated)
}

// IngressNotReconciled an updated ingress resource was received,
// however it could not be reconciled with Pomerium due to errors
func (r *IngressSettingsEventReporter) IngressNotReconciled(ctx context.Context, ingress *networkingv1.Ingress, reason error) error {
	return r.postEvent(ctx, types.NamespacedName{Name: ingress.Name, Namespace: ingress.Namespace},
		corev1.EventTypeWarning, reasonPomeriumConfigUpdateError, reason.Error())
}

// IngressDeleted an ingress resource was deleted and Pomerium no longer serves it
func (r *IngressSettingsEventReporter) IngressDeleted(ctx context.Context, ingress types.NamespacedName, reason string) error {
	return r.postEvent(ctx, ingress, corev1.EventTypeNormal, reasonPomeriumConfigUpdated, msgIngressDeleted)
}

// IngressLogReporter reflects updates as log messages
type IngressLogReporter struct {
	// V is target log level verbosity
	V int
	// Name is the name of the logger
	Name string
}

func (r *IngressLogReporter) logger(ctx context.Context, namespace, name string) logr.Logger {
	return log.FromContext(ctx).
		WithName(r.Name).
		WithValues("ingress", types.NamespacedName{Namespace: namespace, Name: name}.String()).
		V(r.V)
}

// IngressReconciled an ingress was successfully reconciled with Pomerium
func (r *IngressLogReporter) IngressReconciled(ctx context.Context, ingress *networkingv1.Ingress) error {
	r.logger(ctx, ingress.Namespace, ingress.Name).Info("ok")
	return nil
}

// IngressNotReconciled an updated ingress resource was received,
// however it could not be reconciled with Pomerium due to errors
func (r *IngressLogReporter) IngressNotReconciled(ctx context.Context, ingress *networkingv1.Ingress, reason error) error {
	r.logger(ctx, ingress.Namespace, ingress.Name).Error(reason, "not reconciled")
	return nil
}

// IngressDeleted an ingress resource was deleted and Pomerium no longer serves it
func (r *IngressLogReporter) IngressDeleted(ctx context.Context, name types.NamespacedName, reason string) error {
	r.logger(ctx, name.Namespace, name.Name).Info("deleted")
	return nil
}
