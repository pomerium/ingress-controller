// Package controllers implements ingress controller functions
package controllers

import (
	"fmt"

	"github.com/pomerium/ingress-controller/model"
	ctrl "sigs.k8s.io/controller-runtime"
)

// NewIngressController creates new controller runtime
func NewIngressController(opts ctrl.Options, pcr PomeriumReconciler, ns []string) (ctrl.Manager, error) {
	cfg, err := ctrl.GetConfig()
	if err != nil {
		return nil, fmt.Errorf("get k8s api config: %w", err)
	}
	mgr, err := ctrl.NewManager(cfg, opts)
	if err != nil {
		return nil, fmt.Errorf("unable to start manager: %w", err)
	}

	registry := model.NewRegistry()

	if err = (&ingressController{
		PomeriumReconciler: pcr,
		Client:             mgr.GetClient(),
		Registry:           registry,
		EventRecorder:      mgr.GetEventRecorderFor("Ingress"),
		namespaces:         arrayToMap(ns),
	}).SetupWithManager(mgr); err != nil {
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
