// Package reporter contains various methods to report status updates
package reporter

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	"google.golang.org/protobuf/proto"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
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
	obj := icsv1.Pomerium{
		ObjectMeta: metav1.ObjectMeta{
			Name: r.Name,
		},
		Status: icsv1.PomeriumStatus{
			Routes: map[string]icsv1.ResourceStatus{
				types.NamespacedName{Namespace: ingress.Namespace, Name: ingress.Name}.String(): {
					ObservedGeneration: ingress.Generation,
					ObservedAt:         metav1.Time{Time: time.Now()},
					Reconciled:         true,
					Error:              nil,
				}},
		},
	}
	return r.Status().Patch(ctx, &obj, client.MergeFrom(&icsv1.Pomerium{ObjectMeta: obj.ObjectMeta}))
}

// IngressNotReconciled an updated ingress resource was received,
// however it could not be reconciled with Pomerium due to errors
func (r *IngressSettingsReporter) IngressNotReconciled(ctx context.Context, ingress *networkingv1.Ingress, reason error) error {
	obj := icsv1.Pomerium{
		ObjectMeta: metav1.ObjectMeta{
			Name: r.Name,
		},
		Status: icsv1.PomeriumStatus{
			Routes: map[string]icsv1.ResourceStatus{
				types.NamespacedName{Namespace: ingress.Namespace, Name: ingress.Name}.String(): {
					ObservedGeneration: ingress.Generation,
					ObservedAt:         metav1.Time{Time: time.Now()},
					Reconciled:         false,
					Error:              proto.String(reason.Error()),
				}},
		},
	}
	return r.Status().Patch(ctx, &obj, client.MergeFrom(&icsv1.Pomerium{ObjectMeta: obj.ObjectMeta}))
}

// IngressDeleted an ingress resource was deleted and Pomerium no longer serves it
func (r *IngressSettingsReporter) IngressDeleted(ctx context.Context, name types.NamespacedName, reason string) error {
	patch, err := json.Marshal([]struct {
		Op   string `json:"op"`
		Path string `json:"path"`
	}{{
		Op: "remove",
		// https://datatracker.ietf.org/doc/html/rfc6901#section-3
		// "/"(forward slash) is encoded as "~1"
		// ¯\_(ツ)_/¯
		Path: fmt.Sprintf("/status/ingress/%s~1%s", name.Namespace, name.Name),
	}})
	if err != nil {
		return err
	}

	return r.Status().Patch(ctx, &icsv1.Pomerium{
		ObjectMeta: metav1.ObjectMeta{
			Name: r.Name,
		},
		Status: icsv1.PomeriumStatus{
			Routes: map[string]icsv1.ResourceStatus{},
		},
	}, client.RawPatch(types.JSONPatchType, patch))
}

// IngressEventReporter reflects updates as events posted to the ingress
type IngressEventReporter struct {
	record.EventRecorder
}

const (
	reasonPomeriumConfigUpdated     = "Updated"
	reasonPomeriumConfigUpdateError = "UpdateError"
	msgPomeriumConfigUpdated        = "config updated"
	msgPomeriumConfigRejected       = "config rejected"
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
	var obj icsv1.Pomerium
	if err := r.Client.Get(ctx, r.NamespacedName, &obj); err != nil {
		return fmt.Errorf("get %s: %w", r.NamespacedName, err)
	}

	r.EventRecorder.AnnotatedEventf(&obj,
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
	return r.postEvent(ctx, ingress, corev1.EventTypeNormal, reasonPomeriumConfigUpdated, fmt.Sprintf("deleted: %s", reason))
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
