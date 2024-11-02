package gateway

import (
	"crypto/tls"
	"fmt"
	"strings"

	"github.com/hashicorp/go-set/v3"
	"github.com/pomerium/ingress-controller/model"
	"github.com/pomerium/pomerium/pkg/slices"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	gateway_v1 "sigs.k8s.io/gateway-api/apis/v1"
)

type listenerAndStatus struct {
	listener   *gateway_v1.Listener
	status     *gateway_v1.ListenerStatus
	generation int64
}

// processListener adds routes and certificates associated with a single Listener to the
// GatewayConfig and updates the ListenerStatus.
func processListener(
	config *model.GatewayConfig,
	o *objects,
	gatewayKey refKey,
	l listenerAndStatus,
) {
	l.status.Name = l.listener.Name

	g := o.Gateways[gatewayKey]

	// Some status conditions should always be present. Set them first to the optimistic value.
	// The following steps will update these conditions if needed.
	upsertConditions(&l.status.Conditions, g.Generation,
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
	setListenerStatusSupportedKinds(l)
	processCertificateRefs(config, o, g, l)
}

// setListenerStatusSupportedKinds sets the status SupportedKinds and updates the conditions if any
// allowedRoutes kinds are unsupported. (Currently the only supported kind is HTTPRoute.)
func setListenerStatusSupportedKinds(l listenerAndStatus) {
	// If allowedRoutes is unset, there is no restriction on allowed route kinds.
	allowed := l.listener.AllowedRoutes
	if allowed == nil || len(allowed.Kinds) == 0 {
		l.status.SupportedKinds = []gateway_v1.RouteGroupKind{{Kind: "HTTPRoute"}}
		return
	}

	supported := make([]gateway_v1.RouteGroupKind, 0)
	unsupportedKinds := set.New[string](0)
	for _, k := range allowed.Kinds {
		if groupKindFromRouteGroupKind(&k) == (schema.GroupKind{
			Group: gateway_v1.GroupName,
			Kind:  "HTTPRoute",
		}) {
			supported = []gateway_v1.RouteGroupKind{k}
		} else {
			unsupportedKinds.Insert(string(k.Kind))
		}
	}
	l.status.SupportedKinds = supported

	if !unsupportedKinds.Empty() {
		upsertConditions(&l.status.Conditions, l.generation,
			metav1.Condition{
				Type:   string(gateway_v1.ListenerConditionResolvedRefs),
				Status: metav1.ConditionFalse,
				Reason: string(gateway_v1.ListenerReasonInvalidRouteKinds),
				Message: fmt.Sprintf("unsupported route kinds: %s (only HTTPRoute is supported)",
					strings.Join(unsupportedKinds.Slice(), ", ")),
			},
			metav1.Condition{
				Type:   string(gateway_v1.ListenerConditionProgrammed),
				Status: metav1.ConditionFalse,
				Reason: string(gateway_v1.ListenerReasonInvalid),
			},
		)
	}
}

// processCertificateRefs checks that all specified certificateRefs can be matched to available
// TLS Secrets and sets the "ResolvedRefs" status condition if any are invalid.
func processCertificateRefs(
	config *model.GatewayConfig,
	o *objects,
	g *gateway_v1.Gateway,
	l listenerAndStatus,
) {
	if l.listener.TLS == nil {
		return
	}

	// XXX: cross-namespace permissions:
	// References to a resource in different namespace are invalid UNLESS there is a ReferenceGrant in the
	// target namespace that allows the certificate to be attached. If a ReferenceGrant does not allow this
	// reference, the “ResolvedRefs” condition MUST be set to False for this listener with the
	// “RefNotPermitted” reason.

	invalidRefs := set.New[refKey](0)
	for i := range l.listener.TLS.CertificateRefs {
		k := refKeyForCertificateRef(g, &l.listener.TLS.CertificateRefs[i])
		secret, ok := o.TLSSecrets[k]
		if ok && validateTLSSecret(secret) == nil {
			config.Certificates = append(config.Certificates, secret)
		} else {
			invalidRefs.Insert(k)
		}
	}

	if !invalidRefs.Empty() {
		// XXX: does this invalidate the listener as a whole?
		invalidRefNames := slices.Map(invalidRefs.Slice(), func(k refKey) string { return k.Name })
		upsertCondition(&l.status.Conditions, l.generation, metav1.Condition{
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
