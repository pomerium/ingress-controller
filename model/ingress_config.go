// Package model contains common data structures between the controller and pomerium config reconciler
package model

import (
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/types"

	icsv1 "github.com/pomerium/ingress-controller/apis/ingress/v1"
)

const (
	// TLSCustomCASecret replaces https://pomerium.io/reference/#tls-custom-certificate-authority
	// nolint: gosec
	TLSCustomCASecret = "tls_custom_ca_secret"
	// TLSClientSecret replaces https://pomerium.io/reference/#tls-client-certificate
	// nolint: gosec
	TLSClientSecret = "tls_client_secret"
	// TLSDownstreamClientCASecret replaces https://pomerium.io/reference/#tls-downstream-client-certificate-authority
	TLSDownstreamClientCASecret = "tls_downstream_client_ca_secret"
	// TLSServerName is annotation to override TLS server name
	TLSServerName = "tls_server_name"
	// SecureUpstream indicate that service communication should happen over HTTPS
	SecureUpstream = "secure_upstream"
	// PathRegex indicates that paths of ImplementationSpecific type should be treated as regular expression
	PathRegex = "path_regex"
	// UseServiceProxy will use standard k8s service proxy as upstream, opposed to individual endpoints
	UseServiceProxy = "service_proxy_upstream"
	// TCPUpstream indicates this route is a TCP service https://www.pomerium.com/docs/tcp/
	TCPUpstream = "tcp_upstream"
	// KubernetesServiceAccountTokenSecret allows k8s service authentication via pomerium
	// nolint: gosec
	KubernetesServiceAccountTokenSecret = "kubernetes_service_account_token_secret"
	// KubernetesServiceAccountTokenSecretKey defines key within the secret that contains token
	KubernetesServiceAccountTokenSecretKey = "token"
	// SetRequestHeadersSecret defines a secret to copy request headers from
	SetRequestHeadersSecret = "set_request_headers_secret"
	// SetResponseHeadersSecret defines a secret to copy response headers from
	SetResponseHeadersSecret = "set_response_headers_secret"
)

// Config represents global configuration
type Config struct {
	// Settings define global settings parameters
	icsv1.Settings
	// Certs are fetched certs from settings.Certificates
	Certs map[types.NamespacedName]*corev1.Secret
}

// IngressConfig represents ingress and all other required resources
type IngressConfig struct {
	AnnotationPrefix string
	*networkingv1.Ingress
	Endpoints map[types.NamespacedName]*corev1.Endpoints
	Secrets   map[types.NamespacedName]*corev1.Secret
	Services  map[types.NamespacedName]*corev1.Service
}

// IsAnnotationSet checks if a boolean annotation is set to true
func (ic *IngressConfig) IsAnnotationSet(name string) bool {
	return strings.ToLower(ic.Ingress.Annotations[fmt.Sprintf("%s/%s", ic.AnnotationPrefix, name)]) == "true"
}

// IsSecureUpstream returns true if upstream endpoints should be HTTPS
func (ic *IngressConfig) IsSecureUpstream() bool {
	return ic.IsAnnotationSet(SecureUpstream)
}

// IsTCPUpstream returns true is this route represents a TCP service https://www.pomerium.com/docs/tcp/
func (ic *IngressConfig) IsTCPUpstream() bool {
	return ic.IsAnnotationSet(TCPUpstream)
}

// IsPathRegex returns true if paths in the Ingress spec should be treated as regular expressions
func (ic *IngressConfig) IsPathRegex() bool {
	return ic.IsAnnotationSet(PathRegex)
}

// UseServiceProxy disables use of endpoints and would use standard k8s service proxy instead
func (ic *IngressConfig) UseServiceProxy() bool {
	return ic.IsAnnotationSet(UseServiceProxy)
}

// GetNamespacedName returns namespaced name of a resource
func (ic *IngressConfig) GetNamespacedName(name string) types.NamespacedName {
	return types.NamespacedName{Namespace: ic.Ingress.Namespace, Name: name}
}

// GetIngressNamespacedName returns name of that ingress in a namespaced format
func (ic *IngressConfig) GetIngressNamespacedName() types.NamespacedName {
	return types.NamespacedName{Namespace: ic.Ingress.Namespace, Name: ic.Ingress.Name}
}

// GetServicePortByName returns service named port
func (ic *IngressConfig) GetServicePortByName(name types.NamespacedName, port string) (int32, error) {
	svc, ok := ic.Services[name]
	if !ok {
		return 0, fmt.Errorf("service %s was not pre-fetched, this is a bug", name.String())
	}

	for _, servicePort := range svc.Spec.Ports {
		if servicePort.Name == port {
			return servicePort.Port, nil
		}
	}

	return 0, fmt.Errorf("could not find port %s on service %s", port, name.String())
}

const (
	httpSolverLabel = "acme.cert-manager.io/http01-solver"
)

// IsHTTP01Solver checks if this ingress is marked by the cert-manager
// as ACME HTTP01 challenge solver, as it need be handled separately
// namely, publicly accessed and no TLS cert should be required
func IsHTTP01Solver(ingress *networkingv1.Ingress) bool {
	return strings.ToLower(ingress.Labels[httpSolverLabel]) == "true"
}

// Clone creates a deep copy of the ingress config
func (ic *IngressConfig) Clone() *IngressConfig {
	dst := &IngressConfig{
		AnnotationPrefix: ic.AnnotationPrefix,
		Ingress:          ic.Ingress.DeepCopy(),
		Endpoints:        make(map[types.NamespacedName]*corev1.Endpoints, len(ic.Endpoints)),
		Secrets:          make(map[types.NamespacedName]*corev1.Secret, len(ic.Secrets)),
		Services:         make(map[types.NamespacedName]*corev1.Service, len(ic.Services)),
	}

	for k, v := range ic.Secrets {
		dst.Secrets[k] = v.DeepCopy()
	}

	for k, v := range ic.Services {
		dst.Services[k] = v.DeepCopy()
	}

	return dst
}
