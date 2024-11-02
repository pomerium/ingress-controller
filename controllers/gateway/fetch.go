package gateway

import (
	context "context"

	"github.com/hashicorp/go-set/v3"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gateway_v1 "sigs.k8s.io/gateway-api/apis/v1"
	gateway_v1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"
)

// objects holds all relevant Gateway objects and their dependencies.
type objects struct {
	Gateways            map[refKey]*gateway_v1.Gateway
	HTTPRoutesByGateway map[refKey][]httpRouteAndParentRef
	ReferenceGrants     map[refKey]*gateway_v1beta1.ReferenceGrant
	TLSSecrets          map[refKey]*corev1.Secret
}

// fetchObjects fetches all relevant Gateway objects.
func (c *gatewayController) fetchObjects(ctx context.Context) (*objects, error) {
	o := objects{
		Gateways:            make(map[refKey]*gateway_v1.Gateway),
		HTTPRoutesByGateway: make(map[refKey][]httpRouteAndParentRef),
		TLSSecrets:          make(map[refKey]*corev1.Secret),
	}

	// Fetch all GatewayClasses and filter by controller name.
	var gcl gateway_v1.GatewayClassList
	if err := c.List(ctx, &gcl); err != nil {
		return nil, err
	}
	gcNames := set.New[string](0)
	for i := range gcl.Items {
		gc := &gcl.Items[i]
		if gc.Spec.ControllerName == gateway_v1.GatewayController(c.controllerName) {
			gcNames.Insert(gc.Name)
		}
	}

	// Fetch all Gateways and filter by GatewayClass name.
	var gl gateway_v1.GatewayList
	if err := c.List(ctx, &gl); err != nil {
		return nil, err
	}
	for i := range gl.Items {
		g := &gl.Items[i]
		// XXX: do we need to compare namespace as well here?
		if gcNames.Contains(string(g.Spec.GatewayClassName)) {
			o.Gateways[refKeyForObject(g)] = g
		}
	}

	// Fetch all HTTPRoutes and filter by Gateway parentRef.
	var hrl gateway_v1.HTTPRouteList
	if err := c.List(ctx, &hrl); err != nil {
		return nil, err
	}
	for i := range hrl.Items {
		hr := &hrl.Items[i]
		for j := range hr.Spec.ParentRefs {
			pr := &hr.Spec.ParentRefs[j]
			key := refKeyForParentRef(hr, pr)
			if _, ok := o.Gateways[key]; ok {
				o.HTTPRoutesByGateway[key] = append(o.HTTPRoutesByGateway[key],
					httpRouteAndParentRef{hr, pr})
			}
		}
	}

	// Fetch all ReferenceGrants.
	var rgl gateway_v1beta1.ReferenceGrantList
	if err := c.List(ctx, &rgl); err != nil {
		return nil, err
	}
	for i := range rgl.Items {
		rg := &rgl.Items[i]
		o.ReferenceGrants[refKeyForObject(rg)] = rg
	}

	// Fetch all TLS secrets.
	var sl corev1.SecretList
	if err := c.List(ctx, &sl, client.MatchingFields{"type": string(corev1.SecretTypeTLS)}); err != nil {
		return nil, err
	}
	for i := range sl.Items {
		s := &sl.Items[i]
		o.TLSSecrets[refKeyForObject(s)] = s
	}

	return &o, nil
}
