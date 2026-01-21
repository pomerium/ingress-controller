/*
Copyright 2021.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// IdentityProvider for single-sign-on authentication and user identity details by integrating with your downstream Identity Provider (IdP) of choice.
// That authentication integration is achieved using OAuth2, and OpenID Connect (OIDC).
// Where available, Pomerium also supports pulling additional data (like groups) using directory synchronization.
// An additional API token is required for directory sync. https://www.pomerium.com/docs/identity-providers/
type IdentityProvider struct {
	// Provider is the short-hand name of a built-in OpenID Connect (oidc) identity provider to be used for authentication.
	// To use a generic provider, set to <code>oidc</code>.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Enum=auth0;azure;github;gitlab;google;oidc;okta;onelogin;ping
	Provider string `json:"provider"`
	// URL is the base path to an identity provider's OpenID connect discovery document.
	// See <a href="https://pomerium.com/docs/identity-providers">Identity Providers</a> guides for details.
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:Type=string
	// +kubebuilder:validation:Format=uri
	// +kubebuilder:validation:Pattern=`^https://`
	URL *string `json:"url"`
	// Secret containing IdP provider specific parameters.
	// and must contain at least <code>client_id</code> and <code>client_secret</code> values.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Type=string
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:Format="namespace/name"
	Secret string `json:"secret"`
	// ServiceAccountFromSecret is no longer supported,
	// see <a href="https://docs.pomerium.com/docs/overview/upgrading#idp-directory-sync">Upgrade Guide</a>.
	// +optional
	ServiceAccountFromSecret *string `json:"serviceAccountFromSecret,omitempty" deprecated:"idp_directory_sync"`
	// RequestParams to be added as part of a sign-in request using OAuth2 code flow.
	//
	// +kubebuilder:validation:Format="namespace/name"
	// +optional
	RequestParams map[string]string `json:"requestParams,omitempty"`
	// RequestParamsSecret is a reference to a secret for additional parameters you'd prefer not to provide in plaintext.
	// +kubebuilder:validation:Format="namespace/name"
	// +optional
	RequestParamsSecret *string `json:"requestParamsSecret,omitempty"`
	// Scopes Identity provider scopes correspond to access privilege scopes
	// as defined in Section 3.3 of OAuth 2.0 RFC6749.
	// +optional
	Scopes []string `json:"scopes,omitempty"`

	// RefreshDirectory is no longer supported,
	// please see <a href="https://docs.pomerium.com/docs/overview/upgrading#idp-directory-sync">Upgrade Guide</a>.
	//
	// +optional
	RefreshDirectory *RefreshDirectorySettings `json:"refreshDirectory" deprecated:"idp_directory_sync"`
}

// RefreshDirectorySettings defines how frequently should directory update.
type RefreshDirectorySettings struct {
	// interval is the time that pomerium will sync your IDP directory.
	// +kubebuilder:validation:Format=duration
	Interval metav1.Duration `json:"interval"`
	// timeout is the maximum time allowed each run.
	// +kubebuilder:validation:Format=duration
	Timeout metav1.Duration `json:"timeout"`
}

// FileStorage defines File storage options.
type FileStorage struct {
	// Path defines the local file system path to store data.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Type=string
	// +kubebuilder:validation:MinLength=1
	Path string `json:"path"`
}

// PostgresStorage defines Postgres connection parameters.
type PostgresStorage struct {
	// Secret specifies a name of a Secret that must contain
	// <code>connection</code> key. See
	// <a href="https://www.postgresql.org/docs/current/libpq-connect.html#LIBPQ-CONNSTRING">DSN Format and Parameters</a>.
	// Do not set <code>sslrootcert</code>, <code>sslcert</code> and <code>sslkey</code> via connection string,
	// use <code>tlsSecret</code> and <code>caSecret</code> CRD options instead.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Type=string
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:Format="namespace/name"
	Secret string `json:"secret"`
	// TLSSecret should refer to a k8s secret of type <code>kubernetes.io/tls</code>
	// and allows to specify an optional client certificate and key,
	// by constructing <code>sslcert</code> and <code>sslkey</code> connection string
	// <a href="https://www.postgresql.org/docs/current/libpq-connect.html#LIBPQ-PARAMKEYWORDS">
	// parameter values</a>.
	//
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:Type=string
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:Format="namespace/name"
	TLSSecret *string `json:"tlsSecret"`
	// CASecret should refer to a k8s secret with key <code>ca.crt</code> containing CA certificate
	// that, if specified, would be used to populate <code>sslrootcert</code> parameter of the connection string.
	//
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:Type=string
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:Format="namespace/name"
	CASecret *string `json:"caSecret"`
}

// Storage defines persistent storage option for the databroker
// and is only applied for all-in-one pomerium bootstrap,
// and has no effect for the split-mode deployment.
// If Storage is specified, the `postgres` or `file` parameter should be set.
// Omit setting storage to use the in-memory storage implementation.
type Storage struct {
	// File specifies file storage options.
	// +kubebuilder:validation:Optional
	File *FileStorage `json:"file"`
	// Postgres specifies PostgreSQL database connection parameters
	// +kubebuilder:validation:Optional
	Postgres *PostgresStorage `json:"postgres"`
}

// Authenticate service configuration parameters
type Authenticate struct {
	// AuthenticateURL is a dedicated domain URL
	// the non-authenticated persons would be referred to.
	//
	// <p><ul>
	//  <li>You do not need to create a dedicated <code>Ingress</code> for this
	// 		virtual route, as it is handled by Pomerium internally. </li>
	//	<li>You do need create a secret with corresponding TLS certificate for this route
	//		and reference it via <a href="#prop-certificates"><code>certificates</code></a>.
	//		If you use <code>cert-manager</code> with <code>HTTP01</code> challenge,
	//		you may use <code>pomerium</code> <code>ingressClass</code> to solve it.</li>
	// </ul></p>
	//
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Type=string
	// +kubebuilder:validation:Format=uri
	// +kubebuilder:validation:Pattern=`^https://`
	URL string `json:"url"`
}

// Cookie customizes HTTP cookie set by Pomerium.
// note that cookie_secret is part of the main configuration secret
type Cookie struct {
	// Name sets the Pomerium session cookie name.
	// Defaults to <code>_pomerium</code>
	// +optional
	Name *string `json:"name,omitempty"`
	// Domain defaults to the same host that set the cookie.
	// If you specify the domain explicitly, then subdomains would also be included.
	// +optional
	Domain *string `json:"domain,omitempty"`
	// HTTPOnly if set to <code>false</code>, the cookie would be accessible from within the JavaScript.
	// Defaults to <code>true</code>.
	// +optional
	HTTPOnly *bool `json:"httpOnly,omitempty"`
	// Expire sets cookie and Pomerium session expiration time.
	// Once session expires, users would have to re-login.
	// If you change this parameter, existing sessions are not affected.
	// <p>See <a href="https://www.pomerium.com/docs/enterprise/about#session-management">Session Management</a>
	// (Enterprise) for a more fine-grained session controls.</p>
	// <p>Defaults to 14 hours.</p>
	// +kubebuilder:validation:Format=duration
	// +optional
	Expire *metav1.Duration `json:"expire,omitempty"`
	// SameSite sets the SameSite option for cookies.
	// Defaults to <code></code>.
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:Type=string
	// +kubebuilder:validation:Enum=strict;lax;none
	SameSite *string `json:"sameSite,omitempty"`
}

// MatchSubjectAltNames can be used to add an additional constraint when validating client certificates.
type MatchSubjectAltNames struct {
	DNS               string `json:"dns,omitempty"`
	Email             string `json:"email,omitempty"`
	IPAddress         string `json:"ipAddress,omitempty"`
	URI               string `json:"uri,omitempty"`
	UserPrincipalName string `json:"userPrincipalName,omitempty"`
}

// DownstreamMTLS defines downstream MTLS configuration parameters.
type DownstreamMTLS struct {
	// CA is a bundle of PEM-encoded X.509 certificates that will be treated as trust anchors when verifying client certificates.
	// +optional
	CA []byte `json:"ca,omitempty"`
	// CRL is a bundle of PEM-encoded certificate revocation lists to be consulted during certificate validation.
	// +optional
	CRL []byte `json:"crl,omitempty"`
	// Enforcement controls Pomerium's behavior when a client does not present a trusted client certificate.
	// +optional
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:Type=string
	// +kubebuilder:validation:Enum=policy_with_default_deny;policy;reject_connection
	Enforcement *string `json:"enforcement,omitempty"`
	// Match Subject Alt Names can be used to add an additional constraint when validating client certificates.
	// +optional
	MatchSubjectAltNames *MatchSubjectAltNames `json:"matchSubjectAltNames,omitempty"`
	// MaxVerifyDepth sets a limit on the depth of a certificate chain presented by the client.
	// +optional
	MaxVerifyDepth *uint32 `json:"maxVerifyDepth,omitempty"`
}

// PomeriumSpec defines Pomerium-specific configuration parameters.
type PomeriumSpec struct {
	// AccessLogFields sets the <a href="https://www.pomerium.com/docs/reference/access-log-fields">access fields</a> to log.
	AccessLogFields *[]string `json:"accessLogFields,omitempty"`

	// Authenticate sets authenticate service parameters.
	// If not specified, a Pomerium-hosted authenticate service would be used.
	// +kubebuilder:validation:Optional
	Authenticate *Authenticate `json:"authenticate"`

	// AuthorizeLogFields sets the <a href="https://www.pomerium.com/docs/reference/authorize-log-fields">authorize fields</a> to log.
	AuthorizeLogFields *[]string `json:"authorizeLogFields,omitempty"`

	// Certificates is a list of secrets of type TLS to use
	// +kubebuilder:validation:Format="namespace/name"
	// +optional
	Certificates []string `json:"certificates"`

	// CASecret should refer to k8s secrets with key <code>ca.crt</code> containing a CA certificate.
	// +optional
	CASecrets []string `json:"caSecrets"`

	// Cookie defines Pomerium session cookie options.
	// +optional
	Cookie *Cookie `json:"cookie,omitempty"`

	// IdentityProvider configure single-sign-on authentication and user identity details
	// by integrating with your <a href="https://www.pomerium.com/docs/identity-providers/">Identity Provider</a>
	//
	// +kubebuilder:validation:Optional
	IdentityProvider *IdentityProvider `json:"identityProvider"`

	// JWTClaimHeaders convert claims from the assertion token
	// into HTTP headers and adds them into JWT assertion header.
	// Please make sure to read
	// <a href="https://www.pomerium.com/docs/capabilities/getting-users-identity">
	// Getting User Identity</a> guide.
	//
	// +optional
	JWTClaimHeaders map[string]string `json:"jwtClaimHeaders,omitempty"`

	// MCPAllowedClientIDDomains specifies the allowed domains for MCP client ID metadata URLs.
	// This is required when MCP is enabled.
	// See <a href="https://www.pomerium.com/docs/reference/mcp">MCP Settings</a>.
	// +optional
	MCPAllowedClientIDDomains []string `json:"mcpAllowedClientIdDomains,omitempty"`

	// PassIdentityHeaders sets the <a href="https://www.pomerium.com/docs/reference/pass-identity-headers">pass identity headers</a> option.
	PassIdentityHeaders *bool `json:"passIdentityHeaders,omitempty"`

	// ProgrammaticRedirectDomains specifies a list of domains that can be used for
	// <a href="https://www.pomerium.com/docs/capabilities/programmatic-access">programmatic redirects</a>.
	ProgrammaticRedirectDomains []string `json:"programmaticRedirectDomains,omitempty"`

	// RuntimeFlags sets the <a href="https://www.pomerium.com/docs/reference/runtime-flags">runtime flags</a> to enable/disable certain features.
	RuntimeFlags map[string]bool `json:"runtimeFlags,omitempty"`

	// Secrets references a Secret with Pomerium bootstrap parameters.
	//
	// <p>
	// <ul>
	// 	<li><a href="https://pomerium.com/docs/reference/shared-secret"><code>shared_secret</code></a>
	//		- secures inter-Pomerium service communications.
	//	</li>
	// 	<li><a href="https://pomerium.com/docs/reference/cookie-secret"><code>cookie_secret</code></a>
	//		- encrypts Pomerium session browser cookie.
	//		See also other <a href="#cookie">Cookie</a> parameters.
	//	</li>
	// 	<li><a href="https://pomerium.com/docs/reference/signing-key"><code>signing_key</code></a>
	//		signs Pomerium JWT assertion header. See
	//		<a href="https://www.pomerium.com/docs/capabilities/getting-users-identity">Getting the user's identity</a>
	//		guide.
	//	</li>
	// </ul>
	// </p>
	// <p>
	// In a default Pomerium installation manifest, they would be generated via a
	// <a href="https://github.com/pomerium/ingress-controller/blob/main/config/gen_secrets/job.yaml">one-time job</a>
	// and stored in a <code>pomerium/bootstrap</code> Secret.
	// You may re-run the job to rotate the secrets, or update the Secret values manually.
	// </p>
	// <p>
	// When defining the Secret in a manifest, put raw values in <code>stringData</code> so
	// Kubernetes base64-encodes them. Use <code>data</code> only when values are already
	// base64-encoded.
	// </p>
	// <p>
	// Example: <code>stringData.shared_secret</code> and <code>stringData.cookie_secret</code> are
	// raw strings, while <code>data.signing_key</code> is base64-encoded.
	// </p>
	//
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Type=string
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:Format="namespace/name"
	Secrets string `json:"secrets"`

	// SetResponseHeaders specifies a mapping of HTTP Header to be added globally to all managed routes and pomerium's authenticate service.
	// +optional
	// See <a href="https://www.pomerium.com/docs/reference/set-response-headers">Set Response Headers</a>
	SetResponseHeaders map[string]string `json:"setResponseHeaders,omitempty"`

	// Storage defines persistent storage for sessions and other data.
	// See <a href="https://www.pomerium.com/docs/internals/data-storage">Storage</a> for details.
	// If no storage is specified, Pomerium would use a transient in-memory storage (not recommended for production).
	//
	// +kubebuilder:validation:Optional
	Storage *Storage `json:"storage,omitempty"`

	// Timeout specifies the <a href="https://www.pomerium.com/docs/reference/global-timeouts">global timeouts</a> for all routes.
	Timeouts *Timeouts `json:"timeouts,omitempty"`

	// UseProxyProtocol enables <a href="https://www.pomerium.com/docs/reference/use-proxy-protocol">Proxy Protocol</a> support.
	UseProxyProtocol *bool `json:"useProxyProtocol,omitempty"`

	// CodecType sets the <a href="https://www.pomerium.com/docs/reference/codec-type">Codec Type</a>.
	//
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:Enum=auto;http1;http2;http3
	CodecType *string `json:"codecType,omitempty"`

	// BearerTokenFormat sets the <a href="https://www.pomerium.com/docs/reference/bearer-token-format">Bearer Token Format</a>.
	//
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:Enum=default;idp_access_token;idp_identity_token
	BearerTokenFormat *string `json:"bearerTokenFormat,omitempty"`

	// IDPAccessTokenAllowedAudiences specifies the
	// <a href="https://www.pomerium.com/docs/reference/idp-access-token-allowed-audiences">idp access token allowed audiences</a>
	// list.
	IDPAccessTokenAllowedAudiences *[]string `json:"idpAccessTokenAllowedAudiences,omitempty"`

	// OTEL sets the <a href="https://www.pomerium.com/docs/reference/tracing">OpenTelemetry Tracing</a>.
	OTEL *OTEL `json:"otel,omitempty"`

	// DownstreamMTLS sets the <a href="https://www.pomerium.com/docs/reference/downstream-mtls-settings">Downstream MTLS Settings</a>.
	DownstreamMTLS *DownstreamMTLS `json:"downstreamMtls,omitempty"`

	// CircuitBreakerThresholds sets the circuit breaker thresholds settings.
	CircuitBreakerThresholds *CircuitBreakerThresholds `json:"circuitBreakerThresholds,omitempty"`

	// DataBroker sets the databroker settings.
	DataBroker *DataBroker `json:"dataBroker,omitempty"`

	// DNS sets the dns settings.
	DNS *DNS `json:"dns,omitempty"`

	// SSH sets the ssh settings.
	SSH *SSH `json:"ssh,omitempty"`
}

// OTEL configures OpenTelemetry.
type OTEL struct {
	// An OTLP/gRPC or OTLP/HTTP base endpoint URL with optional port.<br/>Example: `http://localhost:4318`
	//
	// +kubebuilder:validation:Required
	Endpoint string `json:"endpoint"`

	// Valid values are `"grpc"` or `"http/protobuf"`.
	//
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Enum=grpc;http/protobuf
	Protocol string `json:"protocol"`

	// Extra headers
	Headers map[string]string `json:"headers,omitempty"`

	// Export request timeout duration
	//
	// +kubebuilder:validation:Format=duration
	// +kubebuilder:validation:Optional
	Timeout *metav1.Duration `json:"timeout,omitempty"`

	// Sampling sets sampling probability between [0, 1].
	//
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:Format=number
	Sampling *string `json:"sampling,omitempty"`

	// ResourceAttributes sets the additional attributes to be added to the trace.
	//
	// +kubebuilder:validation:Optional
	ResourceAttributes map[string]string `json:"resourceAttributes,omitempty"`

	// BSPScheduleDelay sets interval between two consecutive exports
	//
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:Format=duration
	BSPScheduleDelay *metav1.Duration `json:"bspScheduleDelay,omitempty"`

	// BSPMaxExportBatchSize sets the maximum number of spans to export in a single batch
	//
	// +kubebuilder:validation:Optional
	BSPMaxExportBatchSize *int32 `json:"bspMaxExportBatchSize,omitempty"`

	// LogLevel sets the log level for the OpenTelemetry SDK.
	//
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:Enum=trace;debug;info;warn;error
	LogLevel *string `json:"logLevel,omitempty"`
}

// Timeouts allows to configure global timeouts for all routes.
type Timeouts struct {
	// Read specifies the amount of time for the entire request stream to be received from the client.
	// +kubebuilder:validation:Format=duration
	// +optional
	Read *metav1.Duration `json:"read,omitempty"`

	// Write specifies max stream duration is the maximum time that a stream’s lifetime will span.
	// An HTTP request/response exchange fully consumes a single stream.
	// Therefore, this value must be greater than read_timeout as it covers both request and response time.
	// +kubebuilder:validation:Format=duration
	// +optional
	Write *metav1.Duration `json:"write,omitempty"`

	// Idle specifies the time at which a downstream or upstream connection will be terminated if there are no active streams.
	// +kubebuilder:validation:Format=duration
	// +optional
	Idle *metav1.Duration `json:"idle,omitempty"`
}

// CircuitBreakerThresholds are the circuit breaker thresholds.
type CircuitBreakerThresholds struct {
	// MaxConnections sets the maximum number of connections that Envoy will
	// make to the upstream cluster. If not specified, the default is 1024.
	//
	// +kubebuilder:validation:Optional
	MaxConnections *uint32 `json:"maxConnections,omitempty"`
	// MaxPendingRequests sets the maximum number of pending requests that
	// Envoy will allow to the upstream cluster. If not specified, the
	// default is 1024. This limit is applied as a connection limit for
	// non-HTTP traffic.
	//
	// +kubebuilder:validation:Optional
	MaxPendingRequests *uint32 `json:"maxPendingRequests,omitempty"`
	// MaxRequests sets the maximum number of parallel requests that Envoy
	// will make to the upstream cluster. If not specified, the default is
	// 1024. This limit does not apply to non-HTTP traffic.
	//
	// +kubebuilder:validation:Optional
	MaxRequests *uint32 `json:"maxRequests,omitempty"`
	// MaxRetries sets the maximum number of parallel retries that Envoy
	// will allow to the upstream cluster. If not specified, the default is 3.
	//
	// +kubebuilder:validation:Optional
	MaxRetries *uint32 `json:"maxRetries,omitempty"`
	// MaxConnectionPools sets the maximum number of connection pools per
	// cluster that Envoy will concurrently support at once. If not specified,
	// the default is unlimited. Set this for clusters which create a large
	// number of connection pools.
	//
	// +kubebuilder:validation:Optional
	MaxConnectionPools *uint32 `json:"maxConnectionPools"`
}

// DataBroker are the databroker settings.
type DataBroker struct {
	// ClusterLeaderID defines the cluster leader in a clustered databroker.
	// +kubebuilder:validation:Optional
	ClusterLeaderID *string `json:"clusterLeaderId,omitempty"`
}

// DNS are the dns settings.
type DNS struct {
	// LookupFamily is the DNS IP address resolution policy.
	//
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:Enum=auto;v4_only;v6_only;v4_preferred;all
	LookupFamily *string `json:"lookupFamily,omitempty"`
	// FailureRefreshRate is the rate at which DNS lookups are refreshed when requests are failing.
	//
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:Format=duration
	FailureRefreshRate *metav1.Duration `json:"failureRefreshRate,omitempty"`
	// QueryTimeout is the amount of time each name server is given to respond to a query on the first try of any given server.
	//
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:Format=duration
	QueryTimeout *metav1.Duration `json:"queryTimeout,omitempty"`
	// QueryTries is the maximum number of query attempts the resolver will make before giving up. Each attempt may use a different name server.
	//
	// +kubebuilder:validation:Optional
	QueryTries *uint32 `json:"queryTries,omitempty"`
	// RefreshRate is the rate at which DNS lookups are refreshed.
	//
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:Format=duration
	RefreshRate *metav1.Duration `json:"refreshRate,omitempty"`
	// UDPMaxQueries caps the number of UDP based DNS queries on a single port.
	//
	// +kubebuilder:validation:Optional
	UDPMaxQueries *uint32 `json:"udpMaxQueries,omitempty"`
	// UseTCP uses TCP for all DNS queries instead of the default protocol UDP.
	//
	// +kubebuilder:validation:Optional
	UseTCP *bool `json:"useTcp,omitempty"`
}

// SSH are the ssh settings.
type SSH struct {
	// +kubebuilder:validation:Optional
	HostKeySecrets *[]string `json:"hostKeySecrets"`
	// +kubebuilder:validation:Optional
	UserCAKeySecret *string `json:"userCaKeySecret"`
}

// ResourceStatus represents the outcome of the latest attempt to reconcile
// relevant Kubernetes resource with Pomerium.
type ResourceStatus struct {
	// ObservedGeneration represents the <code>.metadata.generation</code> that was last presented to Pomerium.
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
	// ObservedAt is when last reconciliation attempt was made.
	ObservedAt metav1.Time `json:"observedAt,omitempty"`
	// Reconciled is whether this object generation was successfully synced with pomerium.
	Reconciled bool `json:"reconciled"`
	// Error that prevented latest observedGeneration to be synchronized with Pomerium.
	// +optional
	Error *string `json:"error"`
	// Warnings while parsing the resource.
	// +optional
	Warnings []string `json:"warnings"`
}

// PomeriumStatus represents configuration and Ingress status.
type PomeriumStatus struct {
	// Routes provide per-Ingress status.
	Routes map[string]ResourceStatus `json:"ingress,omitempty"`
	// SettingsStatus represent most recent main configuration reconciliation status.
	SettingsStatus *ResourceStatus `json:"settingsStatus,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:resource:path=pomerium
//+kubebuilder:resource:scope=Cluster

// Pomerium define runtime-configurable Pomerium settings
// that do not fall into the category of deployment parameters
type Pomerium struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   PomeriumSpec   `json:"spec,omitempty"`
	Status PomeriumStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// PomeriumList contains a list of Settings
type PomeriumList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Pomerium `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Pomerium{}, &PomeriumList{})
}
