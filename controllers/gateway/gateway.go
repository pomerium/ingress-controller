package gateway

import (
	context "context"
	golog "log"

	"github.com/pomerium/ingress-controller/model"
	corev1 "k8s.io/api/core/v1"
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
	config.IsHTTPOnly = true // XXX

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
		result := processHTTPRoute(o, gateway, listenersByName, r)
		if len(result.Hostnames) > 0 {
			config.Routes = append(config.Routes, model.GatewayHTTPRouteConfig{
				HTTPRoute:        r.route,
				Hostnames:        result.Hostnames,
				ValidBackendRefs: result.ValidBackendRefs,
			})
		}
	}

	updateGatewayAddresses(o, gateway)

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

var (
	gatewayAddressTypeIPAddress = gateway_v1.AddressType("IPAddress")
	gatewayAddressTypeHostname  = gateway_v1.AddressType("Hostname")
)

func updateGatewayAddresses(o *objects, gateway *gateway_v1.Gateway) {
	// Copy the external addresses from the "pomerium-proxy" service.
	proxy := o.Services[refKey{Group: corev1.GroupName, Kind: "Service", Namespace: "pomerium", Name: "pomerium-proxy"}]
	if proxy == nil {
		// XXX: what to do if no "pomerium-proxy" service exists?
		return
	}
	gateway.Status.Addresses = make([]gateway_v1.GatewayStatusAddress, 0, len(proxy.Status.LoadBalancer.Ingress))
	for _, ingress := range proxy.Status.LoadBalancer.Ingress {
		if ingress.IP != "" {
			gateway.Status.Addresses = append(gateway.Status.Addresses, gateway_v1.GatewayStatusAddress{
				Type:  &gatewayAddressTypeIPAddress,
				Value: ingress.IP,
			})
		} else if ingress.Hostname != "" {
			gateway.Status.Addresses = append(gateway.Status.Addresses, gateway_v1.GatewayStatusAddress{
				Type:  &gatewayAddressTypeHostname,
				Value: ingress.Hostname,
			})
		}
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
