package controllers

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
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

	rcn := NewReconciler(mgr.GetClient(), pcr, NewRegistry())

	for _, obj := range []client.Object{
		&networkingv1.Ingress{},
		&corev1.Secret{},
		&corev1.Service{},
	} {
		if err = (&ResourceWatcher{
			ResourceReconciler: rcn,
		}).SetupWithManager(mgr, obj); err != nil {
			return nil, fmt.Errorf("unable to create controller: %w", err)
		}
	}

	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		return nil, fmt.Errorf("unable to set up health check: %w", err)
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		return nil, fmt.Errorf("unable to set up ready check: %w", err)
	}

	return mgr, nil
}
