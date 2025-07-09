// Package model contains common data structures between the controller and pomerium config reconciler
package model

import (
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/types"

	icsv1 "github.com/pomerium/ingress-controller/apis/ingress/v1"
	"github.com/pomerium/ingress-controller/util"
)

const (
	// Name allows customizing the human-readable route name
	Name = "name"
	// TLSCustomCASecret replaces https://pomerium.io/reference/#tls-custom-certificate-authority
	//nolint: gosec
	TLSCustomCASecret = "tls_custom_ca_secret"
	// TLSClientSecret replaces https://pomerium.io/reference/#tls-client-certificate
	//nolint: gosec
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
	// UDPUpstream indicates this route is a UDP service https://www.pomerium.com/docs/capabilities/udp/
	UDPUpstream = "udp_upstream"
	// SubtleAllowEmptyHost is a required annotation when creating an ingress containing
	// rules with an empty (catch-all) host, as it can cause unexpected behavior
	SubtleAllowEmptyHost = "subtle_allow_empty_host"
	// KubernetesServiceAccountTokenSecret allows k8s service authentication via pomerium
	//nolint: gosec
	KubernetesServiceAccountTokenSecret = "kubernetes_service_account_token_secret"
	// KubernetesServiceAccountTokenSecretKey defines key within the secret that contains token
	KubernetesServiceAccountTokenSecretKey = "token"
	// SetRequestHeadersSecret defines a secret to copy request headers from
	SetRequestHeadersSecret = "set_request_headers_secret"
	// SetResponseHeadersSecret defines a secret to copy response headers from
	SetResponseHeadersSecret = "set_response_headers_secret"
	// StorageConnectionStringKey represents a secret that must be present in the Storage Secret
	StorageConnectionStringKey = "connection"
	// CAKey is certificate authority secret key
	CAKey = "ca.crt"
	// SSHPrivateKey is the ssh privatekey secret key
	SSHPrivateKey = "ssh-privatekey"
	// MCPServer indicates this route is an MCP server without any additional configuration
	MCPServer = "mcp_server"
	// MCPClient indicates this route is an MCP client without any additional configuration
	MCPClient = "mcp_client"
	// MCPServerMaxRequestBytes sets the maximum request body size for MCP server routes
	MCPServerMaxRequestBytes = "mcp_server_max_request_bytes"
	// MCPServerUpstreamOAuth2Secret references a secret containing OAuth2 configuration for MCP server upstream authentication
	MCPServerUpstreamOAuth2Secret = "mcp_server_upstream_oauth2_secret" //nolint: gosec
	// MCPServerUpstreamOAuth2AuthURL sets the OAuth2 token URL for MCP server upstream authentication
	MCPServerUpstreamOAuth2AuthURL = "mcp_server_upstream_oauth2_auth_url"
	// MCPServerUpstreamOAuth2TokenURL sets the OAuth2 token URL for MCP server upstream authentication
	MCPServerUpstreamOAuth2TokenURL = "mcp_server_upstream_oauth2_token_url" //nolint: gosec
	// MCPServerUpstreamOAuth2Scopes sets the OAuth2 scopes for MCP server upstream authentication
	MCPServerUpstreamOAuth2Scopes = "mcp_server_upstream_oauth2_scopes"
	// MCPServerUpstreamOAuth2ClientIDKey defines the key within the OAuth2 secret that contains the client ID
	MCPServerUpstreamOAuth2ClientIDKey = "client_id"
	// MCPServerUpstreamOAuth2ClientSecretKey defines the key within the OAuth2 secret that contains the client secret
	MCPServerUpstreamOAuth2ClientSecretKey = "client_secret"
)

// SSHSecrets is a grouping of ssh-related secrets.
type SSHSecrets struct {
	HostKeys  []*corev1.Secret
	UserCAKey *corev1.Secret
}

// Validate validates that the ssh secrets are in the expected format.
func (s SSHSecrets) Validate() error {
	for _, hk := range s.HostKeys {
		if hk.Type != corev1.SecretTypeSSHAuth {
			return fmt.Errorf("ssh host key secret %s should be of type %s, got %s",
				util.GetNamespacedName(hk), corev1.SecretTypeSSHAuth, hk.Type)
		}
	}

	uk := s.UserCAKey
	if uk != nil {
		if uk.Type != corev1.SecretTypeSSHAuth {
			return fmt.Errorf("ssh user ca key secret %s should be of type %s, got %s",
				util.GetNamespacedName(uk), corev1.SecretTypeSSHAuth, uk.Type)
		}
	}

	return nil
}

// StorageSecrets is a convenience grouping of storage-related secrets
type StorageSecrets struct {
	// Secret contains storage connection string
	Secret *corev1.Secret
	// TLS contains optional
	TLS *corev1.Secret
	CA  *corev1.Secret
}

// Validate performs basic check of secrets
func (s StorageSecrets) Validate() error {
	if s.Secret == nil {
		return fmt.Errorf("storage secret is mandatory")
	} else if _, ok := s.Secret.Data[StorageConnectionStringKey]; !ok {
		return fmt.Errorf("storage secret %s should have %q key", util.GetNamespacedName(s.Secret), StorageConnectionStringKey)
	}
	if s.TLS != nil && s.TLS.Type != corev1.SecretTypeTLS {
		return fmt.Errorf("storage TLS secret %s should be of type %s, got %s", util.GetNamespacedName(s.TLS), corev1.SecretTypeTLS, s.TLS.Type)
	}
	if s.CA != nil {
		if _, ok := s.CA.Data[CAKey]; !ok {
			return fmt.Errorf("storage CA secret %s should have %s key", util.GetNamespacedName(s.CA), CAKey)
		}
	}
	return nil
}

// Config represents global configuration
type Config struct {
	// Settings define global settings parameters
	icsv1.Pomerium
	// Secrets are key secrets
	Secrets *corev1.Secret
	// CASecrets are ca secrets
	CASecrets []*corev1.Secret
	// Certs are fetched certs from settings.Certificates
	Certs map[types.NamespacedName]*corev1.Secret
	// RequestParams is a secret from Settings.IdentityProvider.RequestParams
	RequestParams *corev1.Secret
	// IdpSecret is Settings.IdentityProvider.Secret
	IdpSecret *corev1.Secret
	// IdpServiceAccount is Settings.IdentityProvider.ServiceAccountFromSecret
	IdpServiceAccount *corev1.Secret
	// SSHSecrets are secrets related to ssh.
	SSHSecrets SSHSecrets
	// StorageSecrets represent databroker storage settings
	StorageSecrets StorageSecrets
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

// IsUDPUpstream returns true is this route represents a UDP service https://www.pomerium.com/docs/capabilities/tcp/
func (ic *IngressConfig) IsUDPUpstream() bool {
	return ic.IsAnnotationSet(UDPUpstream)
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
