package gateway

import (
	context "context"
	"fmt"
	golog "log"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	gateway_v1 "sigs.k8s.io/gateway-api/apis/v1"
	gateway_v1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"

	"github.com/pomerium/ingress-controller/pomerium"
)

type gatewayController struct {
	// Client is k8s api server client
	client.Client
	// PomeriumReconciler updates Pomerium service configuration
	pomerium.GatewayReconciler
	controllerName string
	// Registry is used to keep track of dependencies between objects
	//model.Registry
	// XXX: I think we'll need something similar, but aware of the Gateway-specific objects
	// MultiPomeriumStatusReporter is used to report when settings are updated
	//reporter.MultiPomeriumStatusReporter
}

const DefaultControllerName = "pomerium.io/gateway-controller"

// NewGatewayController creates and registers a new controller for Gateway objects.
func NewGatewayController(
	ctx context.Context,
	mgr ctrl.Manager,
	pgr pomerium.GatewayReconciler,
	controllerName string,
) error {
	gtc := &gatewayController{
		Client:            mgr.GetClient(),
		GatewayReconciler: pgr,
		controllerName:    controllerName,
	}

	mgr.GetFieldIndexer().IndexField(ctx, &corev1.Secret{}, "type",
		func(o client.Object) []string { return []string{string(o.(*corev1.Secret).Type)} })

	// All updates will trigger the same reconcile request.
	enqueueRequest := handler.EnqueueRequestsFromMapFunc(
		func(_ context.Context, _ client.Object) []reconcile.Request {
			return []reconcile.Request{{
				NamespacedName: types.NamespacedName{
					Name: controllerName,
				},
			}}
		})

	err := ctrl.NewControllerManagedBy(mgr).
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
		// XXX: make this more efficient (ignore non-referenced secrets?)
		Watches(&corev1.Secret{}, enqueueRequest).
		Watches(&corev1.Namespace{}, enqueueRequest). // can we make this more efficient?
		Watches(&corev1.Service{}, enqueueRequest).
		Watches(&gateway_v1beta1.ReferenceGrant{}, enqueueRequest).
		// XXX: need to watch reference grants too
		Complete(gtc)
	if err != nil {
		return fmt.Errorf("build controller: %w", err)
	}

	return nil
}

func (c *gatewayController) Reconcile(ctx context.Context, _ ctrl.Request) (ctrl.Result, error) {
	// XXX: where does this log output go?
	logger := log.FromContext(ctx).V(1)
	logger.Info("[Gateway] reconciling... ")

	golog.Println(" *** [Gateway] Reconcile *** ") // XXX

	o, err := c.fetchObjects(ctx)
	if err != nil {
		// XXX: requeue if there was some fetch error?
		return ctrl.Result{}, err
	}

	config := c.processGateways(ctx, o)

	c.GatewaySetConfig(ctx, config)

	return ctrl.Result{}, nil
}
