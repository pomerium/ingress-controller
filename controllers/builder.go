// Package controllers implements ingress controller functions
package controllers

import (
	"fmt"

	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/pomerium/ingress-controller/controllers/reporter"
	"github.com/pomerium/ingress-controller/model"
)

const (
	// DefaultAnnotationPrefix defines prefix that would be watched for Ingress annotations
	DefaultAnnotationPrefix = "ingress.pomerium.io"
	// DefaultClassControllerName is controller name
	DefaultClassControllerName = "pomerium.io/ingress-controller"
)

// NewIngressController creates new controller runtime
func NewIngressController(
	cfg *rest.Config,
	crOpts ctrl.Options,
	pcr PomeriumReconciler,
	opts ...Option,
) (ctrl.Manager, error) {
	mgr, err := ctrl.NewManager(cfg, crOpts)
	if err != nil {
		return nil, fmt.Errorf("unable to start manager: %w", err)
	}

	registry := model.NewRegistry()
	ic := &ingressController{
		annotationPrefix:   DefaultAnnotationPrefix,
		controllerName:     DefaultClassControllerName,
		PomeriumReconciler: pcr,
		Client:             mgr.GetClient(),
		Registry:           registry,
		MultiIngressStatusReporter: []IngressStatusReporter{
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
	if err = ic.SetupWithManager(mgr); err != nil {
		return nil, fmt.Errorf("unable to create controller: %w", err)
	}

	return mgr, nil
}

func arrayToMap(in []string) map[string]bool {
	out := make(map[string]bool, len(in))
	for _, k := range in {
		out[k] = true
	}
	return out
}
