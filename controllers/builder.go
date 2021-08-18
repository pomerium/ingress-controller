package controllers

import (
	"fmt"

	"github.com/pomerium/ingress-controller/model"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
)

func NewIngressController(opts ctrl.Options, pcr PomeriumReconciler) (ctrl.Manager, error) {
	cfg, err := ctrl.GetConfig()
	if err != nil {
		return nil, fmt.Errorf("get k8s api config: %w", err)
	}
	mgr, err := ctrl.NewManager(cfg, opts)
	if err != nil {
		return nil, fmt.Errorf("unable to start manager: %w", err)
	}

	if err = (&Controller{
		PomeriumReconciler: pcr,
		Client:             mgr.GetClient(),
		Registry:           model.NewRegistry(),
	}).SetupWithManager(mgr); err != nil {
		return nil, fmt.Errorf("unable to create controller: %w", err)
	}

	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		return nil, fmt.Errorf("unable to set up health check: %w", err)
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		return nil, fmt.Errorf("unable to set up ready check: %w", err)
	}

	return mgr, nil
}
