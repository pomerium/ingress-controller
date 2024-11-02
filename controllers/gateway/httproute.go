package gateway

import (
	"strings"

	"github.com/hashicorp/go-set/v3"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	gateway_v1 "sigs.k8s.io/gateway-api/apis/v1"
)

type httpRouteAndParentRef struct {
	route  *gateway_v1.HTTPRoute
	parent *gateway_v1.ParentReference
}

type routeAttachment struct {
	// Resolved hostnames, with "all" represented as "*".
	hostnames []gateway_v1.Hostname

	// "Accepted" condition status reason.
	reason gateway_v1.RouteConditionReason
}

func processHTTPRoute(
	g *gateway_v1.Gateway,
	listeners map[string]listenerAndStatus,
	r httpRouteAndParentRef,
) routeAttachment {
	// An HTTPRoute may specify a listener name directly.
	if r.parent.SectionName != nil {
		l, ok := listeners[string(*r.parent.SectionName)]
		if !ok {
			return routeAttachment{reason: gateway_v1.RouteReasonNoMatchingParent}
		}
		return processHTTPRouteForListener(g, l, r)
	}

	if len(listeners) == 0 {
		return routeAttachment{reason: gateway_v1.RouteReasonNoMatchingParent}
	}

	// Otherwise check for route attachement with all listeners.
	hostnames := set.New[gateway_v1.Hostname](0)
	var reason gateway_v1.RouteConditionReason
	for _, l := range listeners {
		ra := processHTTPRouteForListener(g, l, r)
		hostnames.InsertSlice(ra.hostnames)
		// If the route attaches to any listener, we consider the route accepted.
		// Otherwise we'll return the reason associated with the first listener.
		if reason == "" || ra.reason == gateway_v1.RouteReasonAccepted {
			reason = ra.reason
		}
	}

	return routeAttachment{hostnames.Slice(), reason}
}

func processHTTPRouteForListener(
	g *gateway_v1.Gateway,
	l listenerAndStatus,
	r httpRouteAndParentRef,
) routeAttachment {
	if !isHTTPRouteAllowed(l.listener.AllowedRoutes, r.route, g.Namespace) {
		return routeAttachment{reason: gateway_v1.RouteReasonNotAllowedByListeners}
	}

	hostnames := routeHostnames(l.listener.Hostname, r.route.Spec.Hostnames)
	if len(hostnames) == 0 {
		return routeAttachment{reason: gateway_v1.RouteReasonNoMatchingListenerHostname}
	}

	l.status.AttachedRoutes++

	return routeAttachment{
		hostnames: hostnames,
		reason:    gateway_v1.RouteReasonAccepted,
	}
}

func isHTTPRouteAllowed(
	allowed *gateway_v1.AllowedRoutes,
	r *gateway_v1.HTTPRoute,
	gatewayNamespace string,
) bool {
	// This assumes we've already checked SupportedKinds in processGateway(), so we need only
	// check that the route namespace is allowed.
	from := gateway_v1.NamespacesFromSame
	if allowed.Namespaces != nil && allowed.Namespaces.From != nil {
		from = *allowed.Namespaces.From
	}
	switch from {
	case gateway_v1.NamespacesFromAll:
		return true
	case gateway_v1.NamespacesFromSame:
		return r.Namespace == gatewayNamespace
	case gateway_v1.NamespacesFromSelector:
		selector, err := metav1.LabelSelectorAsSelector(allowed.Namespaces.Selector)
		if err != nil {
			// XXX: update route status with this error?
			return false
		}
		return selector.Matches(labels.Set(r.Labels))
	default:
		return false
	}
}

func routeHostnames(
	listenerHostname *gateway_v1.Hostname,
	routeHostnames []gateway_v1.Hostname,
) []gateway_v1.Hostname {
	// If the listener does not specify a hostname, it accepts any hostname.
	if listenerHostname == nil {
		// If the route also does not specify a hostname, it matches all hostnames.
		if len(routeHostnames) == 0 {
			return []gateway_v1.Hostname{"*"}
		}
		return routeHostnames
	}

	// If the listener specifies a hostname and the route does not, only the listener hostname matches.
	if len(routeHostnames) == 0 {
		return []gateway_v1.Hostname{*listenerHostname}
	}

	// If both the listener and route specify hostnames, compute the intersection.
	var matching []gateway_v1.Hostname
	for _, rh := range routeHostnames {
		if h := hostnameIntersection(string(*listenerHostname), string(rh)); h != "" {
			matching = append(matching, gateway_v1.Hostname(h))
		}
	}
	return matching
}

func hostnameIntersection(a, b string) string {
	// Simplest case: exact match.
	if a == b {
		return a
	}

	// From the spec:
	//
	// "Hostnames that are prefixed with a wildcard label (`*.`) are interpreted as
	// a suffix match. That means that a match for `*.example.com` would match both
	//`test.example.com`, and `foo.test.example.com`, but not `example.com`."
	if strings.HasPrefix(a, "*.") && strings.HasSuffix(b, a[1:]) {
		return b
	}
	if strings.HasPrefix(b, "*.") && strings.HasSuffix(a, b[1:]) {
		return a
	}

	return "" // no intersection
}

func setRouteStatus(
	r httpRouteAndParentRef,
	acceptedReason gateway_v1.RouteConditionReason,
	controllerName string,
) (modified bool) {
	rps, modified := ensureRouteParentStatusExists(r, controllerName)
	acceptedStatus := metav1.ConditionTrue
	if acceptedReason != gateway_v1.RouteReasonAccepted {
		acceptedStatus = metav1.ConditionFalse
	}
	if upsertCondition(&rps.Conditions, r.route.Generation, metav1.Condition{
		Type:   string(gateway_v1.RouteConditionAccepted),
		Status: acceptedStatus,
		Reason: string(acceptedReason),
	}) {
		modified = true
	}
	return modified
}

func ensureRouteParentStatusExists(
	r httpRouteAndParentRef,
	controllerName string,
) (rps *gateway_v1.RouteParentStatus, created bool) {
	refKey := refKeyForParentRef(r.route, r.parent)
	for i := range r.route.Status.Parents {
		rps = &r.route.Status.Parents[i]
		if refKeyForParentRef(r.route, &rps.ParentRef) == refKey {
			return rps, false
		}
	}
	end := len(r.route.Status.Parents)
	r.route.Status.Parents = append(r.route.Status.Parents, gateway_v1.RouteParentStatus{
		ParentRef:      *r.parent,
		ControllerName: gateway_v1.GatewayController(controllerName),
	})
	return &r.route.Status.Parents[end], true
}
