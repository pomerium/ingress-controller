// Package model contains common data structures between the controller and pomerium config reconciler
package model

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gateway_v1 "sigs.k8s.io/gateway-api/apis/v1"
)

// GatewayConfig represents the entirety of the Gateway-defined configuration.
type GatewayConfig struct {
	Routes       []GatewayHTTPRouteConfig
	Certificates []*corev1.Secret
}

// GatewayHTTPRouteConfig represents a single Gateway-defined route together
// with all objects needed to translate it into Pomerium routes.
type GatewayHTTPRouteConfig struct {
	*gateway_v1.HTTPRoute

	// Hostnames this route should match. This may differ from the list of Hostnames in the
	// HTTPRoute Spec depending on the Gateway configuration. "All" is represented as "*".
	Hostnames []gateway_v1.Hostname

	// ValidBackendRefs determines which BackendRefs are allowed to be used for route "To" URLs.
	ValidBackendRefs BackendRefChecker

	// Services is a map of all known services in the cluster.
	Services map[types.NamespacedName]*corev1.Service
}

// BackendRefChecker is used to determine which BackendRefs are valid.
type BackendRefChecker interface {
	Valid(obj client.Object, r *gateway_v1.BackendRef) bool
}
