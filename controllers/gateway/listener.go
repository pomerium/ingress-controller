package gateway

import (
	"fmt"
	"strings"

	"github.com/hashicorp/go-set/v3"
	"k8s.io/apimachinery/pkg/api/equality"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	gateway_v1 "sigs.k8s.io/gateway-api/apis/v1"
)

func setListenersStatus(g *gateway_v1.Gateway, httpRoutes []*gateway_v1.HTTPRoute) (modified bool) {
	prev := g.Status.DeepCopy()

	var status []gateway_v1.ListenerStatus
	for i := range g.Spec.Listeners {
		l := &g.Spec.Listeners[i]

		// Copy over any existing listener status conditions. A newly-created Gateway will not have
		// any listener status and so these will need to created from scratch.
		var conditions []metav1.Condition
		if len(prev.Listeners) > i && prev.Listeners[i].Name == l.Name {
			conditions = prev.Listeners[i].Conditions
		}
		supportedKinds, resolvedRefsCondition := listenerSupportedKinds(l)
		// XXX: reject any unsupported or conflicted listeners
		upsertConditions(&conditions, g.Generation,
			metav1.Condition{
				Type:   string(gateway_v1.ListenerConditionAccepted),
				Status: metav1.ConditionTrue,
				Reason: string(gateway_v1.ListenerReasonAccepted),
			},
			metav1.Condition{
				Type:   string(gateway_v1.ListenerConditionProgrammed),
				Status: metav1.ConditionTrue,
				Reason: string(gateway_v1.ListenerReasonProgrammed),
			},
			resolvedRefsCondition)

		status = append(status, gateway_v1.ListenerStatus{
			Name:           l.Name,
			SupportedKinds: supportedKinds,
			// XXX: adjust when we can accept/reject individual routes
			AttachedRoutes: int32(len(httpRoutes)),
			Conditions:     conditions,
		})
	}
	g.Status.Listeners = status

	return !equality.Semantic.DeepEqual(g.Status, prev)
}

// listenerSupportedKinds returns a list of the supported kinds from among the allowedRoutes, together
// with a corresponding ResolvedRefs condition. (Currently the only supported kind is HTTPRoute.)
func listenerSupportedKinds(l *gateway_v1.Listener) ([]gateway_v1.RouteGroupKind, metav1.Condition) {
	supported := make([]gateway_v1.RouteGroupKind, 0)
	unsupportedKinds := set.New[string](0)
	for _, k := range l.AllowedRoutes.Kinds {
		if groupKindFromRouteGroupKind(&k) == (schema.GroupKind{
			Group: gateway_v1.GroupName,
			Kind:  "HTTPRoute",
		}) {
			supported = []gateway_v1.RouteGroupKind{k}
		} else {
			unsupportedKinds.Insert(string(k.Kind))
		}
	}
	condition := metav1.Condition{Type: string(gateway_v1.ListenerConditionResolvedRefs)}
	if unsupportedKinds.Empty() {
		condition.Status = metav1.ConditionTrue
		condition.Reason = string(gateway_v1.ListenerConditionResolvedRefs)
	} else {
		condition.Status = metav1.ConditionFalse
		condition.Reason = string(gateway_v1.ListenerReasonInvalidRouteKinds)
		condition.Message = fmt.Sprintf("unsupported route kinds: %s (only HTTPRoute is supported)",
			strings.Join(unsupportedKinds.Slice(), ", "))
	}
	return supported, condition
}

func groupKindFromRouteGroupKind(rgk *gateway_v1.RouteGroupKind) schema.GroupKind {
	group := gateway_v1.GroupName
	if rgk.Group != nil {
		group = string(*rgk.Group)
	}
	return schema.GroupKind{
		Group: group,
		Kind:  string(rgk.Kind),
	}
}
