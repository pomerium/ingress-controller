package gateway

import (
	context "context"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gateway_v1 "sigs.k8s.io/gateway-api/apis/v1"
)

type gatewayClassController struct {
	client.Client
	controllerName string
}

// NewGatewayClassController creates and registers a new controller for GatewayClass objects.
// This controller does just one thing: it sets the "Accepted" status condition.
func NewGatewayClassController(
	mgr ctrl.Manager,
	controllerName string,
) error {
	gtcc := &gatewayClassController{
		Client:         mgr.GetClient(),
		controllerName: controllerName,
	}

	return ctrl.NewControllerManagedBy(mgr).
		Named("gateway-class").
		For(&gateway_v1.GatewayClass{}).
		Complete(gtcc)
}

func (c *gatewayClassController) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	var gc gateway_v1.GatewayClass
	if err := c.Get(ctx, req.NamespacedName, &gc); err != nil {
		return ctrl.Result{}, err
	}

	if gc.Spec.ControllerName != gateway_v1.GatewayController(c.controllerName) {
		return ctrl.Result{}, nil
	}

	if setGatewayClassAccepted(&gc) {
		// Condition changed, need to update status.
		if err := c.Status().Update(ctx, &gc); err != nil {
			return ctrl.Result{}, err
		}
	}

	return ctrl.Result{}, nil
}

func setGatewayClassAccepted(gc *gateway_v1.GatewayClass) (modified bool) {
	return upsertCondition(&gc.Status.Conditions, gc.Generation, metav1.Condition{
		Type:   string(gateway_v1.GatewayClassConditionStatusAccepted),
		Status: metav1.ConditionTrue,
		Reason: string(gateway_v1.GatewayClassReasonAccepted),
	})
}
