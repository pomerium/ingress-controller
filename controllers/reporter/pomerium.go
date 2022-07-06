package reporter

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	icsv1 "github.com/pomerium/ingress-controller/apis/ingress/v1"
)

// PomeriumReporter is used to report pomerium deployment status updates
type PomeriumReporter interface {
	// SettingsUpdated indicates the settings were successfully applied
	SettingsUpdated(context.Context) error
}

// SettingsReporter is a common struct for CRD status updates
type SettingsReporter struct {
	types.NamespacedName
	client.Client
}

func (s *SettingsReporter) getSettings(ctx context.Context) (*icsv1.Pomerium, error) {
	var obj icsv1.Pomerium
	if err := s.Get(ctx, s.NamespacedName, &obj); err != nil {
		return nil, fmt.Errorf("get %s: %w", s.NamespacedName, err)
	}

	if obj.Status.Routes == nil {
		obj.Status.Routes = make(map[string]icsv1.RouteStatus)
	}

	return &obj, nil
}

// SettingsStatusReporter is a PomeriumReporter that updates /status of the Settings CRD
type SettingsStatusReporter struct {
	SettingsReporter
}

// SettingsUpdated marks that settings was reconciled with pomerium
func (s *SettingsStatusReporter) SettingsUpdated(ctx context.Context) error {
	return nil
}

// SettingsEventReporter posts events to the Settings CRD
type SettingsEventReporter struct {
	SettingsReporter
	record.EventRecorder
}

// SettingsUpdated marks configuration was reconciled with pomerium
func (s *SettingsEventReporter) SettingsUpdated(ctx context.Context) error {
	settings, err := s.getSettings(ctx)
	if err != nil {
		return err
	}
	s.Event(settings, corev1.EventTypeNormal, reasonPomeriumConfigUpdated, msgPomeriumConfigUpdated)
	return nil
}

// SettingsLogReporter posts events to the log
type SettingsLogReporter struct{}

// SettingsUpdated marks configuration was synced with pomerium
func (s *SettingsLogReporter) SettingsUpdated(ctx context.Context) error {
	log.FromContext(ctx).Info("pomerium settings updated")
	return nil
}
