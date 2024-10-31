package gateway

import (
	"crypto/tls"
	"fmt"
	"strings"

	"github.com/hashicorp/go-set/v3"
	"github.com/pomerium/pomerium/pkg/slices"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	gateway_v1 "sigs.k8s.io/gateway-api/apis/v1"
)

func setListenersStatus(
	g *gateway_v1.Gateway,
	httpRoutes []*gateway_v1.HTTPRoute,
	availableCertificates map[refKey]*corev1.Secret,
) (modified bool) {
	prev := g.Status.DeepCopy()

	var status []gateway_v1.ListenerStatus
	for i := range g.Spec.Listeners {
		l := &g.Spec.Listeners[i]

		ls := gateway_v1.ListenerStatus{
			Name: l.Name,
			// XXX: adjust when we can accept/reject individual routes
			AttachedRoutes: int32(len(httpRoutes)),
		}

		// Copy over any existing listener status conditions. A newly-created Gateway will not have
		// any listener status conditions and so these will need to created from scratch.
		if len(prev.Listeners) > i && prev.Listeners[i].Name == l.Name {
			ls.Conditions = prev.Listeners[i].Conditions
		} else {
			upsertConditions(&ls.Conditions, g.Generation,
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
				metav1.Condition{
					Type:   string(gateway_v1.ListenerConditionResolvedRefs),
					Status: metav1.ConditionTrue,
					Reason: string(gateway_v1.ListenerConditionResolvedRefs),
				},
			)
		}

		listenerSupportedKinds(&ls, l, g.Generation)
		listenerCertificateRefs(&ls, g, l, availableCertificates)

		// XXX: reject any unsupported or conflicted listeners

		status = append(status, ls)
	}
	g.Status.Listeners = status

	return !equality.Semantic.DeepEqual(g.Status, prev)
}

// listenerSupportedKinds sets the SupportedKinds and updates the ResolvedRefs condition if any
// allowedRoutes Kinds are unsupported. (Currently the only supported kind is HTTPRoute.)
func listenerSupportedKinds(ls *gateway_v1.ListenerStatus, l *gateway_v1.Listener, observedGeneration int64) {
	ls.SupportedKinds = []gateway_v1.RouteGroupKind{{Kind: "HTTPRoute"}}

	if l.AllowedRoutes == nil || len(l.AllowedRoutes.Kinds) == 0 {
		return
	}

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
	ls.SupportedKinds = supported

	if !unsupportedKinds.Empty() {
		upsertCondition(&ls.Conditions, observedGeneration, metav1.Condition{
			Type:   string(gateway_v1.ListenerConditionResolvedRefs),
			Status: metav1.ConditionFalse,
			Reason: string(gateway_v1.ListenerReasonInvalidRouteKinds),
			Message: fmt.Sprintf("unsupported route kinds: %s (only HTTPRoute is supported)",
				strings.Join(unsupportedKinds.Slice(), ", ")),
		})
	}
}

// listenerCertificateRefs checks that all specified certificateRefs can be matched to available
// TLS Secrets and sets the ResolvedRefs condition if any are invalid.
func listenerCertificateRefs(
	ls *gateway_v1.ListenerStatus,
	g *gateway_v1.Gateway,
	l *gateway_v1.Listener,
	availableCertificates map[refKey]*corev1.Secret,
) {
	if l.TLS == nil {
		return
	}

	// XXX: cross-namespace permissions:
	// References to a resource in different namespace are invalid UNLESS there is a ReferenceGrant in the
	// target namespace that allows the certificate to be attached. If a ReferenceGrant does not allow this
	// reference, the “ResolvedRefs” condition MUST be set to False for this listener with the
	// “RefNotPermitted” reason.

	invalidRefs := set.New[refKey](0)
	for i := range l.TLS.CertificateRefs {
		k := refKeyForCertificateRef(g, &l.TLS.CertificateRefs[i])
		secret, ok := availableCertificates[k]
		if !ok || validateTLSSecret(secret) != nil {
			invalidRefs.Insert(k)
		}
	}

	if !invalidRefs.Empty() {
		invalidRefNames := slices.Map(invalidRefs.Slice(), func(k refKey) string { return k.Name })
		upsertCondition(&ls.Conditions, g.Generation, metav1.Condition{
			Type:    string(gateway_v1.ListenerConditionResolvedRefs),
			Status:  metav1.ConditionFalse,
			Reason:  string(gateway_v1.ListenerReasonInvalidCertificateRef),
			Message: "invalid certificate refs: " + strings.Join(invalidRefNames, ", "),
		})
	}
}

// groupKindFromRouteGroupKind normalizes a RouteGroupKind by translating a nil Kind to its default
// value. The result is suitable for equality comparison.
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

// validateTLSSecret checks that a Secret has "tls.crt" and "tls.key" data fields that can be
// parsed as an X.509 certificate and corresponding private key.
func validateTLSSecret(s *corev1.Secret) error {
	certPEM := s.Data["tls.crt"]
	keyPEM := s.Data["tls.key"]
	_, err := tls.X509KeyPair(certPEM, keyPEM)
	return err
}
