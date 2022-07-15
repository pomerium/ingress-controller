package reporter

import (
	"context"
	"time"

	"google.golang.org/protobuf/proto"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	icsv1 "github.com/pomerium/ingress-controller/apis/ingress/v1"
)

// PomeriumReporter is used to report pomerium deployment status updates
type PomeriumReporter interface {
	// SettingsUpdated indicates the settings were successfully applied.
	SettingsUpdated(context.Context, *icsv1.Pomerium) error
	// SettingsRejected indicates settings were rejected.
	SettingsRejected(context.Context, *icsv1.Pomerium, error) error
}

// SettingsReporter is a common struct for CRD status updates
type SettingsReporter struct {
	types.NamespacedName
	client.Client
}

// SettingsStatusReporter is a PomeriumReporter that updates /status of the Settings CRD
type SettingsStatusReporter struct {
	SettingsReporter
}

// SettingsUpdated marks that settings was reconciled with pomerium
func (s *SettingsStatusReporter) SettingsUpdated(ctx context.Context, obj *icsv1.Pomerium) error {
	return s.Status().Patch(ctx, &icsv1.Pomerium{
		ObjectMeta: obj.ObjectMeta,
		Status: icsv1.PomeriumStatus{
			SettingsStatus: &icsv1.ResourceStatus{
				ObservedGeneration: obj.Generation,
				ObservedAt:         metav1.Time{Time: time.Now()},
				Reconciled:         true,
				Error:              nil,
			},
		},
	}, client.MergeFrom(&icsv1.Pomerium{ObjectMeta: obj.ObjectMeta}))
}

// SettingsRejected settings was not reconciled with pomerium and provides a reason
func (s *SettingsStatusReporter) SettingsRejected(ctx context.Context, obj *icsv1.Pomerium, err error) error {
	return s.Status().Patch(ctx, &icsv1.Pomerium{
		ObjectMeta: obj.ObjectMeta,
		Status: icsv1.PomeriumStatus{
			SettingsStatus: &icsv1.ResourceStatus{
				ObservedGeneration: obj.Generation,
				ObservedAt:         metav1.Time{Time: time.Now()},
				Reconciled:         false,
				Error:              proto.String(err.Error()),
			},
		},
	}, client.MergeFrom(&icsv1.Pomerium{ObjectMeta: obj.ObjectMeta}))
}

// SettingsEventReporter posts events to the Settings CRD
type SettingsEventReporter struct {
	SettingsReporter
	record.EventRecorder
}

// SettingsUpdated marks configuration was reconciled with pomerium
func (s *SettingsEventReporter) SettingsUpdated(ctx context.Context, obj *icsv1.Pomerium) error {
	s.Event(obj, corev1.EventTypeNormal, reasonPomeriumConfigUpdated, msgPomeriumConfigUpdated)
	return nil
}

// SettingsRejected marks configuration was rejected
func (s *SettingsEventReporter) SettingsRejected(ctx context.Context, obj *icsv1.Pomerium, err error) error {
	s.Event(obj, corev1.EventTypeNormal, reasonPomeriumConfigUpdateError, err.Error())
	return nil
}

// SettingsLogReporter posts events to the log
type SettingsLogReporter struct{}

// SettingsUpdated marks configuration was synced with pomerium
func (s *SettingsLogReporter) SettingsUpdated(ctx context.Context, _ *icsv1.Pomerium) error {
	log.FromContext(ctx).Info(msgPomeriumConfigUpdated)
	return nil
}

// SettingsRejected settings were not synced with Pomerium and provides a reason
func (s *SettingsLogReporter) SettingsRejected(ctx context.Context, _ *icsv1.Pomerium, err error) error {
	log.FromContext(ctx).Error(err, msgPomeriumConfigRejected)
	return nil
}
