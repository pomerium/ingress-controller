/*
Copyright 2021.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controllers

import (
	"context"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

const (
	retryDuration = time.Second * 30
)

type ConfigReconciler interface {
	// Upsert should update or create the pomerium routes corresponding to this ingress
	Upsert(ctx context.Context, ing *networkingv1.Ingress, tlsSecrets []*TLSSecret, services map[types.NamespacedName]*corev1.Service) error
	// Delete should delete pomerium routes corresponding to this ingress name
	Delete(ctx context.Context, namespacedName types.NamespacedName) error
}

// IngressReconciler reconciles a Ingress object
type IngressReconciler struct {
	client.Client
	Scheme *runtime.Scheme
	ConfigReconciler
}

//+kubebuilder:rbac:groups=networking.k8s.io,resources=ingresses,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=networking.k8s.io,resources=ingresses/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=networking.k8s.io,resources=ingresses/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the Ingress object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.8.3/pkg/reconcile
func (r *IngressReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	ing, tlsSecrets, services, err := fetchIngress(ctx, r.Client, req.NamespacedName)
	if err != nil {
		if !apierrors.IsNotFound(err) {
			return ctrl.Result{
				Requeue:      true,
				RequeueAfter: retryDuration,
			}, fmt.Errorf("fetch ingress and related resources: %w", err)
		}
		logger.Info("not found", "name", req.NamespacedName)
		if err := r.ConfigReconciler.Delete(ctx, req.NamespacedName); err != nil {
			return ctrl.Result{
				Requeue:      true,
				RequeueAfter: retryDuration,
			}, fmt.Errorf("deleting: %w", err)
		}
		return ctrl.Result{
			Requeue: false,
		}, nil

	}

	if err := r.ConfigReconciler.Upsert(ctx, ing, tlsSecrets, services); err != nil {
		return ctrl.Result{
			Requeue:      true,
			RequeueAfter: retryDuration,
		}, fmt.Errorf("upsert: %w", err)
	}
	logger.Info("updated", "uid", ing.UID, "version", ing.ResourceVersion)
	return ctrl.Result{
		Requeue:      false,
		RequeueAfter: 0,
	}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *IngressReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&networkingv1.Ingress{}).
		Complete(r)
}
