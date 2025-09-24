// Package health encapsulates extensions to pomerium core's pkg/health
package health

import (
	"github.com/pomerium/pomerium/pkg/health"
)

const (
	// SettingsBootstrapReconciler checks that the bootstrap reconciler has run once successfully
	SettingsBootstrapReconciler = health.Check("controller.settings.reconciler.bootstrap")
	// SettingsReconciler checks that the leased settings reconciler has run
	SettingsReconciler = health.Check("controller.settings.reconciler")
)
