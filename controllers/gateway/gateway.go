package gateway

import (
	context "context"
	golog "log"

	"github.com/pomerium/ingress-controller/model"
	"k8s.io/apimachinery/pkg/api/equality"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gateway_v1 "sigs.k8s.io/gateway-api/apis/v1"
)

// processGateways updates the status of all Gateways and associated routes, and returns a
// GatewayConfig object with all valid configuration.
func (c *gatewayController) processGateways(
	ctx context.Context,
	o *objects,
) *model.GatewayConfig {
	var config model.GatewayConfig

	for key := range o.Gateways {
		c.processGateway(ctx, &config, o, key)
	}

	c.updateModifiedHTTPRouteStatus(ctx, o.OriginalHTTPRouteStatus)

	return &config
}

// processGateway updates the status of this gateway and appends any valid configuration to the
// GatewayConfig object.
func (c *gatewayController) processGateway(
	ctx context.Context,
	config *model.GatewayConfig,
	o *objects,
	gatewayKey refKey,
) {
	golog.Printf(" *** processGateway: %s ***", gatewayKey.Name) // XXX

	gateway := o.Gateways[gatewayKey]

	// Snapshot the existing status, then compare after updates to determine if it has changed.
	previousStatus := gateway.Status.DeepCopy()

	// We need to preserve any existing ListenerStatus conditions, to avoid modifying the
	// LastTransitionTime incorrectly.
	ensureListenerStatusExists(gateway)

	listenersByName := make(map[string]listenerAndStatus)

	for i := range gateway.Spec.Listeners {
		listener := &gateway.Spec.Listeners[i]
		status := &gateway.Status.Listeners[i]
		l := listenerAndStatus{listener, status, gateway.Generation}

		processListener(config, o, gatewayKey, l)

		// Filter out any listeners that do not support HTTPRoutes.
		if len(status.SupportedKinds) > 0 {
			listenersByName[string(listener.Name)] = l
		}

		// Reset AttachedRoutes because processHTTPRoute() will increment these counts.
		status.AttachedRoutes = 0
	}

	for _, r := range o.HTTPRoutesByGateway[gatewayKey] {
		hostnames := processHTTPRoute(o, gateway, listenersByName, r)
		if len(hostnames) > 0 {
			config.Routes = append(config.Routes, model.GatewayHTTPRouteConfig{
				HTTPRoute: r.route,
				Hostnames: hostnames,
				// XXX: other references?
			})
		}
	}

	// TODO: update other Gateway status (Addresses)

	upsertGatewayConditions(gateway,
		metav1.Condition{
			Type:    string(gateway_v1.GatewayConditionAccepted),
			Status:  metav1.ConditionTrue,
			Reason:  string(gateway_v1.GatewayReasonAccepted),
			Message: "", // XXX
		},
		// XXX: how to determine if anything is "Programmed"?
		metav1.Condition{
			Type:    string(gateway_v1.GatewayConditionProgrammed),
			Status:  metav1.ConditionTrue,
			Reason:  string(gateway_v1.GatewayReasonProgrammed),
			Message: "", // XXX
		},
	)

	if !equality.Semantic.DeepEqual(gateway.Status, previousStatus) {
		if err := c.Status().Update(ctx, gateway); err != nil {
			golog.Printf("couldn't update status for %q: %v", gateway.Name, err) // XXX
		}
	}
}

// ensureListenerStatusExists ensures that the elements of g.Status.Listeners correspond to the
// elements of g.Spec.Listeners.
func ensureListenerStatusExists(g *gateway_v1.Gateway) {
	// Check to see if the listeners status already matches.
	if len(g.Status.Listeners) == len(g.Spec.Listeners) {
		ok := true
		for i := range len(g.Spec.Listeners) {
			if g.Status.Listeners[i].Name != g.Spec.Listeners[i].Name {
				ok = false
				break
			}
		}
		if ok {
			return
		}
	}

	// Allocate new listeners status and copy over any existing status.
	listenerStatusMap := make(map[string]gateway_v1.ListenerStatus)
	for _, ls := range g.Status.Listeners {
		listenerStatusMap[string(ls.Name)] = ls
	}
	g.Status.Listeners = make([]gateway_v1.ListenerStatus, len(g.Spec.Listeners))
	for i := range len(g.Spec.Listeners) {
		g.Status.Listeners[i] = listenerStatusMap[string(g.Spec.Listeners[i].Name)]
	}
}

func upsertGatewayConditions(g *gateway_v1.Gateway, conditions ...metav1.Condition) (modified bool) {
	return upsertConditions(&g.Status.Conditions, g.Generation, conditions...)
}

func (c *gatewayController) updateModifiedHTTPRouteStatus(
	ctx context.Context, s []httpRouteAndOriginalStatus,
) {
	for _, r := range s {
		if !equality.Semantic.DeepEqual(r.route.Status, r.originalStatus) {
			if err := c.Status().Update(ctx, r.route); err != nil {
				golog.Printf("couldn't update status for %q: %v", r.route.Name, err)
			}
		}
	}
}
