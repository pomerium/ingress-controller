// Package model contains common data structures between the controller and pomerium config reconciler
package model

import (
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/types"
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
)

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

// IsPathRegex returns true if paths in the Ingress spec should be treated as regular expressions
func (ic *IngressConfig) IsPathRegex() bool {
	return ic.IsAnnotationSet(PathRegex)
}

// UseServiceProxy disables use of endpoints and would use standard k8s service proxy instead
func (ic *IngressConfig) UseServiceProxy() bool {
	return ic.IsAnnotationSet(UseServiceProxy)
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

// TLSCert represents a parsed TLS secret
type TLSCert struct {
	Key  []byte
	Cert []byte
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

// ParseTLSCerts decodes K8s TLS secret
func (ic *IngressConfig) ParseTLSCerts() ([]*TLSCert, error) {
	certs := make([]*TLSCert, 0, len(ic.Secrets))

	for _, secret := range ic.Secrets {
		if secret.Type != corev1.SecretTypeTLS {
			return nil, fmt.Errorf("secret=%s, expected type %s, got %s", secret.Name, corev1.SecretTypeTLS, secret.Type)
		}
		certs = append(certs, &TLSCert{
			Key:  secret.Data[corev1.TLSPrivateKeyKey],
			Cert: secret.Data[corev1.TLSCertKey],
		})
	}

	return certs, nil
}
