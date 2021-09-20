// Package controllers implements ingress controller functions
package controllers

import (
	"context"
	"fmt"

	"github.com/pomerium/ingress-controller/model"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
)

const (
	DefaultAnnotationPrefix    = "ingress.pomerium.io"
	DefaultClassControllerName = "pomerium.io/ingress-controller"
)

// NewIngressController creates new controller runtime
func NewIngressController(ctx context.Context, cfg *rest.Config, crOpts ctrl.Options, pcr PomeriumReconciler, opts ...option) (ctrl.Manager, error) {
	mgr, err := ctrl.NewManager(cfg, crOpts)
	if err != nil {
		return nil, fmt.Errorf("unable to start manager: %w", err)
	}

	ctx, cancel := context.WithCancel(ctx)
	registry := model.NewRegistry()
	ic := &ingressController{
		annotationPrefix:   DefaultAnnotationPrefix,
		controllerName:     DefaultClassControllerName,
		PomeriumReconciler: pcr,
		Client:             mgr.GetClient(),
		Registry:           registry,
		EventRecorder:      mgr.GetEventRecorderFor("Ingress"),
	}
	ic.initComplete = newOnce(ic.reconcileInitial, cancel)
	for _, opt := range opts {
		opt(ic)
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
