package health

import (
	"github.com/pomerium/pomerium/pkg/health"
)

const (
	IngressCtrlEmbeddedPomerium            = health.Check("controller.embedded.pomerium")
	IngressCtrlBootstrapConfig             = health.Check("controller.bootstrap.config")
	IngressCtrlSettingsBootstrapReconciler = health.Check("controller.settings.reconciler.bootstrap")
	IngressCtrlSettingsReconciler          = health.Check("controller.settings.reconciler")
	IngressCtrlIngressReconciler           = health.Check("controller.ingress.reconciler")
	IngressCtrlGatewayReconciler           = health.Check("controller.gateway.reconciler")
	IngressCtrlConfigReconciler            = health.Check("controller.config.reconciler")
)
