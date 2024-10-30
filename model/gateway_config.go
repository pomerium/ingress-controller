// Package model contains common data structures between the controller and pomerium config reconciler
package model

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	gateway_v1 "sigs.k8s.io/gateway-api/apis/v1"
)

// GatewayConfig represents the entirety of the Gateway-defined configuration.
type GatewayConfig struct {
	Routes       []GatewayHTTPRouteConfig
	Certificates []*corev1.Secret
}

// XXX: need to think more about incremental updates to certificates

// GatewayHTTPRouteConfig represents a single Gateway-defined route together
// with all objects needed to translate it into zero or more Pomerium routes.
type GatewayHTTPRouteConfig struct {
	*gateway_v1.HTTPRoute

	// Hostnames this route should match. This may differ from the list of Hostnames in the
	// HTTPRoute Spec depending on the Gateway configuration.
	// XXX: should we set this to {"*"} to represent matching all hostnames?
	Hostnames []gateway_v1.Hostname

	// XXX: these I copied from IngressConfig, need to make sure what's actually needed
	Endpoints map[types.NamespacedName]*corev1.Endpoints
	Secrets   map[types.NamespacedName]*corev1.Secret
	Services  map[types.NamespacedName]*corev1.Service
}

// XXX
