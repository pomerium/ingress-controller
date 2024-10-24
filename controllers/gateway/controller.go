package gateway

import (
	context "context"
	"fmt"

	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	gateway_v1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/pomerium/ingress-controller/pomerium"
)

type gatewayController struct {
	// Client is k8s api server client
	client.Client
	// PomeriumReconciler updates Pomerium service configuration
	pomerium.ConfigReconciler
	// Registry is used to keep track of dependencies between objects
	//model.Registry
	// XXX: I think we'll need something similar, but aware of the Gateway-specific objects
	// MultiPomeriumStatusReporter is used to report when settings are updated
	//reporter.MultiPomeriumStatusReporter
}

// NewGatewayController creates and registers a new controller for Gateway objects.
func NewGatewayController(
	mgr ctrl.Manager,
	pcr pomerium.ConfigReconciler,
	controllerName string,
) error {
	gtc := &gatewayController{
		Client:           mgr.GetClient(),
		ConfigReconciler: pcr,
	}

	// All updates will trigger the same reconcile request.
	enqueueRequest := handler.EnqueueRequestsFromMapFunc(
		func(_ context.Context, _ client.Object) []reconcile.Request {
			return []reconcile.Request{{
				NamespacedName: types.NamespacedName{
					Name: controllerName,
				},
			}}
		})

	// For GatewayClass we can easily filter out non-matching objects by ControllerName.
	// (I don't see an "easy" way to do something similar for Gateway or route objects.)
	filterGatewayClass := func(object client.Object) bool {
		gwc, ok := object.(*gateway_v1.GatewayClass)
		if !ok {
			return false
		}
		return gwc.Spec.ControllerName == gateway_v1.GatewayController(controllerName)
	}

	err := ctrl.NewControllerManagedBy(mgr).
		Named("gateway").
		Watches(
			&gateway_v1.GatewayClass{},
			enqueueRequest,
			builder.WithPredicates(predicate.NewPredicateFuncs(filterGatewayClass)),
		).
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
		Complete(gtc)
	if err != nil {
		return fmt.Errorf("build controller: %w", err)
	}

	return nil
}

// Reconcile syncs Settings CRD with pomerium databroker
func (c *gatewayController) Reconcile(ctx context.Context, _ ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx).V(1)
	logger.Info("[Gateway] reconciling... ")

	return ctrl.Result{}, nil
}
