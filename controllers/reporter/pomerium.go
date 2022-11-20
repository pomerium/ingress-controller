package reporter

import (
	"context"
	"fmt"
	"time"

	"google.golang.org/protobuf/proto"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	pom_cfg "github.com/pomerium/pomerium/config"

	icsv1 "github.com/pomerium/ingress-controller/apis/ingress/v1"
	"github.com/pomerium/ingress-controller/util"
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
				Warnings:           getConfigWarnings(ctx),
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
				Warnings:           getConfigWarnings(ctx),
			},
		},
	}, client.MergeFrom(&icsv1.Pomerium{ObjectMeta: obj.ObjectMeta}))
}

func getConfigWarnings(ctx context.Context) []string {
	var out []string
	for _, msg := range util.Get[pom_cfg.FieldMsg](ctx) {
		out = append(out, fmt.Sprintf("%s: %s, please see %s", msg.Key, msg.FieldCheckMsg, msg.DocsURL))
	}
	return out
}

func getConfigWarningsKV(ctx context.Context) [][]any {
	var out [][]any
	for _, msg := range util.Get[pom_cfg.FieldMsg](ctx) {
		var kv []any
		if msg.Key != "" {
			kv = append(kv, "key", msg.Key)
		}
		if msg.DocsURL != "" {
			kv = append(kv, "docs", msg.DocsURL)
		}
		if msg.FieldCheckMsg != "" {
			kv = append(kv, "msg", string(msg.FieldCheckMsg))
		}
		out = append(out, kv)
	}
	return out
}

// SettingsEventReporter posts events to the Settings CRD
type SettingsEventReporter struct {
	SettingsReporter
	record.EventRecorder
}

// SettingsUpdated marks configuration was reconciled with pomerium
func (s *SettingsEventReporter) SettingsUpdated(ctx context.Context, obj *icsv1.Pomerium) error {
	for _, msg := range getConfigWarnings(ctx) {
		s.Event(obj, corev1.EventTypeWarning, reasonPomeriumConfigValidation, msg)
	}
	s.Event(obj, corev1.EventTypeNormal, reasonPomeriumConfigUpdated, msgPomeriumConfigUpdated)
	return nil
}

// SettingsRejected marks configuration was rejected
func (s *SettingsEventReporter) SettingsRejected(ctx context.Context, obj *icsv1.Pomerium, err error) error {
	for _, msg := range getConfigWarnings(ctx) {
		s.Event(obj, corev1.EventTypeWarning, reasonPomeriumConfigValidation, msg)
	}
	s.Event(obj, corev1.EventTypeNormal, reasonPomeriumConfigUpdateError, err.Error())
	return nil
}

// SettingsLogReporter posts events to the log
type SettingsLogReporter struct{}

// SettingsUpdated marks configuration was synced with pomerium
func (s *SettingsLogReporter) SettingsUpdated(ctx context.Context, _ *icsv1.Pomerium) error {
	logger := log.FromContext(ctx)

	for _, kv := range getConfigWarningsKV(ctx) {
		logger.Info("deprecated config", kv...)
	}

	logger.Info(msgPomeriumConfigUpdated)
	return nil
}

// SettingsRejected settings were not synced with Pomerium and provides a reason
func (s *SettingsLogReporter) SettingsRejected(ctx context.Context, _ *icsv1.Pomerium, err error) error {
	logger := log.FromContext(ctx)

	for _, kv := range getConfigWarningsKV(ctx) {
		logger.Info("deprecated config", kv...)
	}

	logger.Error(err, msgPomeriumConfigRejected)
	return nil
}
