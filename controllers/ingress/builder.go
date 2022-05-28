// Package ingress implements Ingress controller functions
package ingress

import (
	"fmt"

	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/pomerium/ingress-controller/controllers/reporter"
	"github.com/pomerium/ingress-controller/model"
	"github.com/pomerium/ingress-controller/pomerium"
)

const (
	// DefaultAnnotationPrefix defines prefix that would be watched for Ingress annotations
	DefaultAnnotationPrefix = "ingress.pomerium.io"
	// DefaultClassControllerName is controller name
	DefaultClassControllerName = "pomerium.io/ingress-controller"
)

// NewIngressController creates new controller runtime
func NewIngressController(
	mgr ctrl.Manager,
	pcr pomerium.Reconciler,
	opts ...Option,
) error {
	registry := model.NewRegistry()
	ic := &ingressController{
		annotationPrefix: DefaultAnnotationPrefix,
		controllerName:   DefaultClassControllerName,
		Reconciler:       pcr,
		Client:           mgr.GetClient(),
		Registry:         registry,
		MultiIngressStatusReporter: []reporter.IngressStatusReporter{
			&reporter.IngressEventReporter{EventRecorder: mgr.GetEventRecorderFor("pomerium-ingress")},
			&reporter.IngressLogReporter{V: 1, Name: "reconcile"},
		},
	}
	ic.initComplete = newOnce(ic.reconcileInitial)
	for _, opt := range opts {
		opt(ic)
	}

	if ic.globalSettings != nil {
		ic.MultiIngressStatusReporter = append(ic.MultiIngressStatusReporter, &reporter.IngressSettingsReporter{
			Name:   *ic.globalSettings,
			Client: ic.Client,
		})
	}
	if err := ic.SetupWithManager(mgr); err != nil {
		return fmt.Errorf("unable to create controller: %w", err)
	}

	return nil
}

func arrayToMap(in []string) map[string]bool {
	out := make(map[string]bool, len(in))
	for _, k := range in {
		out[k] = true
	}
	return out
}
