package gateway

import (
	golog "log"
	"strings"

	"github.com/hashicorp/go-set/v3"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	gateway_v1 "sigs.k8s.io/gateway-api/apis/v1"
)

type httpRouteResult struct {
	Hostnames        []gateway_v1.Hostname
	ValidBackendRefs set.Collection[*gateway_v1.BackendRef]
}

// processHTTPRoute checks the validity of an HTTPRoute, updates its status accordingly, and
// computes its matching hostnames (with "all" represented as "*").
func processHTTPRoute(
	o *objects,
	g *gateway_v1.Gateway,
	listeners map[string]listenerAndStatus,
	r httpRouteInfo,
) httpRouteResult {
	golog.Printf(" *** processHTTPRoute: %s ***", r.route.Name) // XXX

	var result httpRouteResult

	// Reject this route early if it includes any per-backendRef filters as we don't support these.
	if anyBackendRefHasFilters(r.route.Spec.Rules) {
		upsertCondition(&r.status.Conditions, r.route.Generation, metav1.Condition{
			Type:    string(gateway_v1.RouteConditionAccepted),
			Status:  metav1.ConditionFalse,
			Reason:  string(gateway_v1.RouteReasonUnsupportedValue),
			Message: "backendRef filters are not supported",
		})
		return result
	}

	result.ValidBackendRefs = validateHTTPRouteBackendRefsResolved(o, r)

	// An HTTPRoute may specify a listener name directly. In this case we should check for route
	// attachment with just the one listener.
	if r.parent.SectionName != nil {
		l, ok := listeners[string(*r.parent.SectionName)]
		if !ok {
			setRouteStatusAccepted(r, gateway_v1.RouteReasonNoMatchingParent)
			return result
		}
		ra := processHTTPRouteForListener(o, g, l, r.route)
		setRouteStatusAccepted(r, ra.reason)
		result.Hostnames = ra.hostnames
		return result
	}

	// Otherwise check for route attachement with all listeners.
	hostnamesSet := set.New[gateway_v1.Hostname](0)
	var reason gateway_v1.RouteConditionReason
	for _, l := range listeners {
		ra := processHTTPRouteForListener(o, g, l, r.route)
		hostnamesSet.InsertSlice(ra.hostnames)
		// If the route attaches to any listener, we consider the route accepted.
		// Otherwise we'll return the reason associated with the first listener.
		if reason == "" || ra.reason == gateway_v1.RouteReasonAccepted {
			reason = ra.reason
		}
	}
	if reason == "" {
		reason = gateway_v1.RouteReasonNoMatchingParent // no listeners at all
	}
	setRouteStatusAccepted(r, reason)
	result.Hostnames = hostnamesSet.Slice()
	return result
}

func anyBackendRefHasFilters(rules []gateway_v1.HTTPRouteRule) bool {
	for i := range rules {
		rule := &rules[i]
		for i := range rule.BackendRefs {
			if len(rule.BackendRefs[i].Filters) > 0 {
				return true
			}
		}
	}
	return false
}

func validateHTTPRouteBackendRefsResolved(
	o *objects,
	r httpRouteInfo,
) (validRefs set.Collection[*gateway_v1.BackendRef]) {
	// Check that all backendRefs can be resolved.
	validRefs = set.New[*gateway_v1.BackendRef](0)
	invalidRefs := make(map[gateway_v1.RouteConditionReason][]string)
	invalid := func(reason gateway_v1.RouteConditionReason, name string) {
		invalidRefs[reason] = append(invalidRefs[reason], name)
	}
	for i := range r.route.Spec.Rules {
		rule := &r.route.Spec.Rules[i]
		for i := range rule.BackendRefs {
			refKey := refKeyForBackendRef(r.route, &rule.BackendRefs[i].BackendObjectReference)
			if refKey.Group != corev1.GroupName || refKey.Kind != "Service" {
				invalid(gateway_v1.RouteReasonInvalidKind, refKey.Name)
				continue
			}
			if !o.ReferenceGrants.allowed(r.route, refKey) {
				invalid(gateway_v1.RouteReasonRefNotPermitted, refKey.Name)
				continue
			}
			if o.Services[refKey] == nil {
				invalid(gateway_v1.RouteReasonBackendNotFound, refKey.Name)
			}
			validRefs.Insert(&rule.BackendRefs[i].BackendRef)
		}
	}

	resolvedRefs := metav1.Condition{
		Type:   string(gateway_v1.RouteConditionResolvedRefs),
		Status: metav1.ConditionTrue,
		Reason: string(gateway_v1.RouteReasonResolvedRefs),
	}

	var messages []string
	for reason, refNames := range invalidRefs {
		resolvedRefs.Status = metav1.ConditionFalse
		resolvedRefs.Reason = string(reason) // if multiple reasons apply this will set one arbitrarily
		messages = append(messages, "invalid refs ("+string(reason)+"): "+strings.Join(refNames, ", "))
	}
	resolvedRefs.Message = strings.Join(messages, "; ")

	upsertCondition(&r.status.Conditions, r.route.Generation, resolvedRefs)

	return validRefs
}

type routeAttachment struct {
	// Resolved hostnames, with "all" represented as "*".
	hostnames []gateway_v1.Hostname

	// "Accepted" condition status reason.
	reason gateway_v1.RouteConditionReason
}

func processHTTPRouteForListener(
	o *objects,
	g *gateway_v1.Gateway,
	l listenerAndStatus,
	r *gateway_v1.HTTPRoute,
) routeAttachment {
	if !isHTTPRouteAllowed(o, l.listener.AllowedRoutes, r, g.Namespace) {
		return routeAttachment{reason: gateway_v1.RouteReasonNotAllowedByListeners}
	}

	hostnames := routeHostnames(l.listener.Hostname, r.Spec.Hostnames)
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
	o *objects,
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
		routeNamespace := o.Namespaces[r.Namespace]
		return selector.Matches(labels.Set(routeNamespace.Labels))
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

func setRouteStatusAccepted(
	r httpRouteInfo,
	acceptedReason gateway_v1.RouteConditionReason,
) (modified bool) {
	acceptedStatus := metav1.ConditionTrue
	if acceptedReason != gateway_v1.RouteReasonAccepted {
		acceptedStatus = metav1.ConditionFalse
	}
	return upsertCondition(&r.status.Conditions, r.route.Generation, metav1.Condition{
		Type:   string(gateway_v1.RouteConditionAccepted),
		Status: acceptedStatus,
		Reason: string(acceptedReason),
	})
}

// XXX: not sure how to accomplish this:
// When a HTTPBackendRef is invalid, 500 status codes MUST be returned for requests that would
// have otherwise been routed to an invalid backend. If multiple backends are specified, and some
// are invalid, the proportion of requests that would otherwise have been routed to an invalid
// backend MUST receive a 500 status code.
