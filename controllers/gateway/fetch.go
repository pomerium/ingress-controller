package gateway

import (
	context "context"

	"github.com/hashicorp/go-set/v3"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gateway_v1 "sigs.k8s.io/gateway-api/apis/v1"
	gateway_v1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"
)

// objects holds all relevant Gateway objects and their dependencies.
type objects struct {
	Gateways                map[refKey]*gateway_v1.Gateway
	HTTPRoutesByGateway     map[refKey][]httpRouteInfo
	OriginalHTTPRouteStatus []httpRouteAndOriginalStatus
	Namespaces              map[string]*corev1.Namespace
	ReferenceGrants         referenceGrantMap
	TLSSecrets              map[refKey]*corev1.Secret
	Services                map[refKey]*corev1.Service
}

type httpRouteAndOriginalStatus struct {
	route          *gateway_v1.HTTPRoute
	originalStatus *gateway_v1.HTTPRouteStatus
}

// fetchObjects fetches all relevant Gateway objects.
func (c *gatewayController) fetchObjects(ctx context.Context) (*objects, error) {
	var o objects

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
	o.Gateways = make(map[refKey]*gateway_v1.Gateway)
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
	o.HTTPRoutesByGateway = make(map[refKey][]httpRouteInfo)
	for i := range hrl.Items {
		hr := &hrl.Items[i]
		o.OriginalHTTPRouteStatus = append(o.OriginalHTTPRouteStatus,
			httpRouteAndOriginalStatus{route: hr, originalStatus: hr.Status.DeepCopy()})
		for j := range hr.Spec.ParentRefs {
			pr := &hr.Spec.ParentRefs[j]
			key := refKeyForParentRef(hr, pr)
			if _, ok := o.Gateways[key]; ok {
				o.HTTPRoutesByGateway[key] = append(o.HTTPRoutesByGateway[key],
					newHTTPRouteInfo(hr, pr, c.controllerName))
			}
		}
	}

	// Fetch all Namespaces (the labels may be needed for the allowedRoutes restrictions).
	var nl corev1.NamespaceList
	if err := c.List(ctx, &nl); err != nil {
		return nil, err
	}
	o.Namespaces = make(map[string]*corev1.Namespace)
	for i := range nl.Items {
		n := &nl.Items[i]
		o.Namespaces[n.Name] = n
	}

	// Fetch all ReferenceGrants.
	var rgl gateway_v1beta1.ReferenceGrantList
	if err := c.List(ctx, &rgl); err != nil {
		return nil, err
	}
	o.ReferenceGrants = buildReferenceGrantMap(rgl.Items)

	// Fetch all TLS secrets.
	var sl corev1.SecretList
	if err := c.List(ctx, &sl, client.MatchingFields{"type": string(corev1.SecretTypeTLS)}); err != nil {
		return nil, err
	}
	o.TLSSecrets = make(map[refKey]*corev1.Secret)
	for i := range sl.Items {
		s := &sl.Items[i]
		o.TLSSecrets[refKeyForObject(s)] = s
	}

	// Fetch all Services.
	var servicesList corev1.ServiceList
	if err := c.List(ctx, &servicesList); err != nil {
		return nil, err
	}
	o.Services = make(map[refKey]*corev1.Service)
	for i := range servicesList.Items {
		s := &servicesList.Items[i]
		o.Services[refKeyForObject(s)] = s
	}

	return &o, nil
}

type httpRouteInfo struct {
	route  *gateway_v1.HTTPRoute
	parent *gateway_v1.ParentReference
	status *gateway_v1.RouteParentStatus
}

// newHTTPRouteInfo populates an httpRouteInfo struct, allocating a RouteParentStatus if needed.
func newHTTPRouteInfo(
	route *gateway_v1.HTTPRoute,
	parent *gateway_v1.ParentReference,
	controllerName string,
) httpRouteInfo {
	r := httpRouteInfo{route: route, parent: parent}
	for i := range route.Status.Parents {
		r.status = &route.Status.Parents[i]
		if equality.Semantic.DeepEqual(&r.status.ParentRef, parent) {
			return r
		}
	}

	end := len(r.route.Status.Parents)
	r.route.Status.Parents = append(r.route.Status.Parents, gateway_v1.RouteParentStatus{
		ParentRef:      *parent,
		ControllerName: gateway_v1.GatewayController(controllerName),
	})
	r.status = &r.route.Status.Parents[end]
	return r
}
