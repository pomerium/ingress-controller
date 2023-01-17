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

// RedisStorage defines REDIS databroker storage backend bootstrap parameters.
// Redis is supported for legacy deployments, new deployments should use PostgreSQL.
type RedisStorage struct {
	// Secret specifies a name of a Secret that must contain
	// <code>connection</code> key.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Type=string
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:Format="namespace/name"
	Secret string `json:"secret"`
	// TLSSecret should refer to a k8s secret of type <code>kubernetes.io/tls</code>
	// that would be used to perform TLS connection to REDIS.
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:Type=string
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:Format="namespace/name"
	TLSSecret *string `json:"tlsSecret"`
	// CASecret should refer to a k8s secret with key <code>ca.crt</code> that must be a PEM-encoded
	// certificate authority to use when connecting to the databroker storage engine.
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:Type=string
	// +kubebuilder:validation:Format="namespace/name"
	CASecret *string `json:"caSecret"`
	// TLSSkipVerify disables TLS certificate chain validation.
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:Type=boolean
	TLSSkipVerify bool `json:"tlsSkipVerify"`
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
// If Storage is specified, either `redis` or `postgresql` parameter should be set.
// Omit setting storage to use in-memory storage implementation.
type Storage struct {
	// Redis defines REDIS connection parameters
	// +kubebuilder:validation:Optional
	Redis *RedisStorage `json:"redis" deprecated:"redis"`

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
	// CallbackPath sets the path at which the authenticate service receives callback responses
	// from your identity provider. The value must exactly match one of the authorized redirect URIs for the OAuth 2.0 client.
	//
	// <p>This value is referred to as the redirect_url in the OpenIDConnect and OAuth2 specs.</p>
	// <p>Defaults to <code>/oauth2/callback</code></p>
	//
	// +optional
	CallbackPath *string `json:"callbackPath,omitempty"`
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
	// Secure if set to false, would make a cookie accessible over insecure protocols (HTTP).
	// Defaults to <code>true</code>.
	// +optional
	Secure *bool `json:"secure,omitempty"`
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
}

// PomeriumSpec defines Pomerium-specific configuration parameters.
type PomeriumSpec struct {
	// Authenticate sets authenticate service parameters
	// +kubebuilder:validation:Required
	Authenticate Authenticate `json:"authenticate"`

	// IdentityProvider configure single-sign-on authentication and user identity details
	// by integrating with your <a href="https://www.pomerium.com/docs/identity-providers/">Identity Provider</a>
	//
	// +kubebuilder:validation:Required
	IdentityProvider IdentityProvider `json:"identityProvider"`

	// Certificates is a list of secrets of type TLS to use
	// +kubebuilder:validation:Format="namespace/name"
	// +optional
	Certificates []string `json:"certificates"`

	// CASecret should refer to k8s secrets with key <code>ca.crt</code> containing a CA certificate.
	// +optional
	CASecrets []string `json:"caSecrets"`

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
	//		<a href="https://www.pomerium.com/docs/topics/getting-users-identity">Getting the user's identity</a>
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
	//
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Type=string
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:Format="namespace/name"
	Secrets string `json:"secrets"`

	// Storage defines persistent storage for sessions and other data.
	// See <a href="https://www.pomerium.com/docs/topics/data-storage">Storage</a> for details.
	// If no storage is specified, Pomerium would use a transient in-memory storage (not recommended for production).
	//
	// +kubebuilder:validation:Optional
	Storage *Storage `json:"storage,omitempty"`

	// Cookie defines Pomerium session cookie options.
	// +optional
	Cookie *Cookie `json:"cookie,omitempty"`

	// JWTClaimHeaders convert claims from the assertion token
	// into HTTP headers and adds them into JWT assertion header.
	// Please make sure to read
	// <a href="https://www.pomerium.com/docs/topics/getting-users-identity">
	// Getting User Identity</a> guide.
	//
	// +optional
	JWTClaimHeaders map[string]string `json:"jwtClaimHeaders,omitempty"`
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
