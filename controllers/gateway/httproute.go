package gateway

import (
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	gateway_v1 "sigs.k8s.io/gateway-api/apis/v1"
)

type httpRouteAndParentRef struct {
	route  *gateway_v1.HTTPRoute
	parent *gateway_v1.ParentReference
}

type routeAttachment struct {
	hostnames []gateway_v1.Hostname
	reason    gateway_v1.RouteConditionReason
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
	var hostnames []gateway_v1.Hostname
	var reason gateway_v1.RouteConditionReason
	for _, l := range listeners {
		ra := processHTTPRouteForListener(g, l, r)
		hostnames = append(hostnames, ra.hostnames...)
		// If the route attaches to any listener, we consider the route accepted.
		// Otherwise we'll return the reason associated with the first listener.
		if reason == "" || ra.reason == gateway_v1.RouteReasonAccepted {
			reason = ra.reason
		}
	}

	return routeAttachment{hostnames, reason}
}

func processHTTPRouteForListener(
	g *gateway_v1.Gateway,
	l listenerAndStatus,
	r httpRouteAndParentRef,
) routeAttachment {
	if !isHTTPRouteAllowed(l.listener.AllowedRoutes, r.route, g.Namespace) {
		return routeAttachment{reason: gateway_v1.RouteReasonNotAllowedByListeners}
	}

	hostnames := routeHostnames((*string)(l.listener.Hostname), r.route.Spec.Hostnames)
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

func routeHostnames(listenerHostname *string, routeHostnames []gateway_v1.Hostname) []gateway_v1.Hostname {
	// If the listener does not specify a hostname, it accepts any hostname.
	if listenerHostname == nil {
		return routeHostnames
	}

	// If the listener does specify a hostname, the route must include a matching hostname.
	var matching []gateway_v1.Hostname
	for _, h := range routeHostnames {
		if hostnameMatches(*listenerHostname, string(h)) {
			matching = append(matching, h)
		}
	}
	return matching
}

func hostnameMatches(a, b string) bool {
	// Simplest case: exact match.
	if a == b {
		return true
	}

	// From the spec:
	//
	// "Hostnames that are prefixed with a wildcard label (`*.`) are interpreted as
	// a suffix match. That means that a match for `*.example.com` would match both
	//`test.example.com`, and `foo.test.example.com`, but not `example.com`."
	return (strings.HasPrefix(a, "*.") && strings.HasSuffix(b, a[1:])) ||
		(strings.HasPrefix(b, "*.") && strings.HasSuffix(a, b[1:]))
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
