// Package gateway contains controllers for Gateway API objects.
package gateway

import (
	context "context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	gateway_v1 "sigs.k8s.io/gateway-api/apis/v1"
	gateway_v1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"

	"github.com/pomerium/ingress-controller/pomerium"
)

// DefaultClassControllerName is the default GatewayClass ControllerName.
const DefaultClassControllerName = "pomerium.io/gateway-controller"

// ControllerConfig contains configuration options for the Gateway controller.
type ControllerConfig struct {
	// ControllerName associates this controller with a GatewayClass.
	ControllerName string
	// Gateway addresses are determined from this service.
	ServiceName types.NamespacedName
}

type gatewayController struct {
	client.Client
	pomerium.GatewayReconciler
	ControllerConfig
}

// NewGatewayController creates and registers a new controller for Gateway objects.
func NewGatewayController(
	ctx context.Context,
	mgr ctrl.Manager,
	pgr pomerium.GatewayReconciler,
	config ControllerConfig,
) error {
	gtc := &gatewayController{
		Client:            mgr.GetClient(),
		GatewayReconciler: pgr,
		ControllerConfig:  config,
	}

	err := mgr.GetFieldIndexer().IndexField(ctx, &corev1.Secret{}, "type",
		func(o client.Object) []string { return []string{string(o.(*corev1.Secret).Type)} })
	if err != nil {
		return fmt.Errorf("couldn't create index on Secret type: %w", err)
	}

	// All updates will trigger the same reconcile request.
	enqueueRequest := handler.EnqueueRequestsFromMapFunc(
		func(_ context.Context, _ client.Object) []reconcile.Request {
			return []reconcile.Request{{
				NamespacedName: types.NamespacedName{
					Name: config.ControllerName,
				},
			}}
		})

	err = ctrl.NewControllerManagedBy(mgr).
		Named("gateway").
		Watches(
			&gateway_v1.Gateway{},
			enqueueRequest,
			builder.WithPredicates(predicate.GenerationChangedPredicate{}),
		).
		Watches(
			&gateway_v1.HTTPRoute{},
			enqueueRequest,
			builder.WithPredicates(predicate.GenerationChangedPredicate{}),
		).
		Watches(&corev1.Secret{}, enqueueRequest).
		Watches(&corev1.Namespace{}, enqueueRequest).
		Watches(&corev1.Service{}, enqueueRequest).
		Watches(&gateway_v1beta1.ReferenceGrant{}, enqueueRequest).
		Complete(gtc)
	if err != nil {
		return fmt.Errorf("build controller: %w", err)
	}

	return nil
}

func (c *gatewayController) Reconcile(ctx context.Context, _ ctrl.Request) (ctrl.Result, error) {
	o, err := c.fetchObjects(ctx)
	if err != nil {
		return ctrl.Result{}, err
	}

	config := c.processGateways(ctx, o)

	_, err = c.SetGatewayConfig(ctx, config)
	if err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}
