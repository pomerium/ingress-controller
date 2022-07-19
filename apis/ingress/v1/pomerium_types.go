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
	// Provider one of accepted providers - see https://www.pomerium.com/reference/#identity-provider-name.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Enum=auth0;azure;google;okta;onelogin;oidc;ping;github
	Provider string `json:"provider"`
	// URL is identity provider url, see https://www.pomerium.com/reference/#identity-provider-url.
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:Type=string
	// +kubebuilder:validation:Format=uri
	// +kubebuilder:validation:Pattern=`^https://`
	URL *string `json:"url"`
	// Secret containing IdP provider specific parameters
	// and must contain at least client_id and client_secret values,
	// an optional `service_account` field, mapped to https://www.pomerium.com/reference/#identity-provider-service-account
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Type=string
	// +kubebuilder:validation:MinLength=1
	Secret string `json:"secret"`
	// ServiceAccountFromSecret is a convenience way to build a value for `idp_service_account` from
	// secret map values, see https://www.pomerium.com/docs/identity-providers/
	// +optional
	ServiceAccountFromSecret *string `json:"serviceAccountFromSecret,omitempty"`
	// RequestParams see https://www.pomerium.com/reference/#identity-provider-request-params
	// +optional
	RequestParams map[string]string `json:"requestParams,omitempty"`
	// RequestParamsSecret is a reference to a secret for additional parameters you'd prefer not to provide in plaintext.
	// +optional
	RequestParamsSecret *string `json:"requestParamsSecret,omitempty"`
	// Scopes see https://www.pomerium.com/reference/#identity-provider-scopes.
	// +optional
	Scopes []string `json:"scopes,omitempty"`

	// refreshDirectory specifis
	// +optional
	RefreshDirectory *RefreshDirectorySettings `json:"refreshDirectory"`
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
// Redis is supported for legacy deployment, and new deployments should use PostgreSQL.
type RedisStorage struct {
	// Secret specifies a name of a Secret that must contain
	// `connection` key.
	// see https://www.pomerium.com/docs/reference/data-broker-storage-connection-string
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Type=string
	// +kubebuilder:validation:MinLength=1
	Secret string `json:"secret"`
	// TLSSecret should refer to a k8s secret of type `kubernetes.io/tls`
	// and allows to specify an optional databroker storage client certificate and key, see
	// - https://www.pomerium.com/docs/reference/data-broker-storage-certificate-file
	// - https://www.pomerium.com/docs/reference/data-broker-storage-certificate-key-file
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:Type=string
	// +kubebuilder:validation:MinLength=1
	TLSSecret *string `json:"tlsSecret"`
	// CASecret should refer to a k8s secret with key `ca.crt` that must be a PEM-encoded
	// certificate authority to use when connecting to the databroker storage engine
	// see https://www.pomerium.com/docs/reference/data-broker-storage-certificate-authority
	//
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:Type=string
	CASecret *string `json:"caSecret"`
	// TLSSkipVerify disables TLS certificate chain validation
	// see https://www.pomerium.com/docs/reference/data-broker-storage-tls-skip-verify
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:Type=boolean
	TLSSkipVerify bool `json:"tlsSkipVerify"`
}

// PostgresStorage defines Postgres connection parameters.
type PostgresStorage struct {
	// Secret specifies a name of a Secret that must contain
	// `connection` key
	// for the connection DSN format and parameters, see
	// https://www.postgresql.org/docs/current/libpq-connect.html#LIBPQ-CONNSTRING
	// the following keywords are not allowed to be part of the parameters,
	// as they must be populated via `tlsCecret` and `caSecret` fields
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Type=string
	// +kubebuilder:validation:MinLength=1
	Secret string `json:"secret"`
	// TLSSecret should refer to a k8s secret of type `kubernetes.io/tls`
	// and allows to specify an optional client certificate and key,
	// by constructing `sslcert` and `sslkey` connection string parameter values
	// see https://www.postgresql.org/docs/current/libpq-connect.html#LIBPQ-PARAMKEYWORDS
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:Type=string
	// +kubebuilder:validation:MinLength=1
	TLSSecret *string `json:"tlsSecret"`
	// CASecret should refer to a k8s secret with key `ca.crt` containing CA certificate
	// that, if specified, would be used to populate `sslrootcert` parameter of the connection string
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:Type=string
	// +kubebuilder:validation:MinLength=1
	CASecret *string `json:"caSecret"`
}

// Storage defines persistent storage option for the databroker
// and is only applied for all-in-one pomerium bootstrap,
// and has no effect for the split-mode deployment.
// If Storage is specified, either `redis`` or `postgresql` parameter should be set.
// Omit setting storage to use in-memory storage implementation.
type Storage struct {
	// Redis defines REDIS connection parameters
	// +kubebuilder:validation:Optional
	Redis *RedisStorage `json:"redis"`

	// Postgres specifies PostgreSQL database connection parameters
	// +kubebuilder:validation:Optional
	Postgres *PostgresStorage `json:"postgres"`
}

// Authenticate service configuration parameters
type Authenticate struct {
	// AuthenticateURL should be publicly accessible URL
	// the non-authenticated persons would be referred to
	// see https://www.pomerium.com/reference/#authenticate-service-url
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Type=string
	// +kubebuilder:validation:Format=uri
	// +kubebuilder:validation:Pattern=`^https://`
	URL string `json:"url"`
	// CallbackPath see https://www.pomerium.com/reference/#authenticate-callback-path
	// +optional
	CallbackPath *string `json:"callbackPath,omitempty"`
}

// PomeriumSpec defines the desired state of Settings
type PomeriumSpec struct {
	// Authenticate sets authenticate service parameters
	// +kubebuilder:validation:Required
	Authenticate Authenticate `json:"authenticate"`

	// IdentityProvider see https://www.pomerium.com/docs/identity-providers/
	// +kubebuilder:validation:Required
	IdentityProvider IdentityProvider `json:"identityProvider"`

	// Certificates is a list of secrets of type TLS to use
	// +optional
	Certificates []string `json:"certificates"`

	// Secrets references a Secret that must have the following keys
	// - shared_secret
	// - cookie_secret
	// - signing_key
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Type=string
	// +kubebuilder:validation:MinLength=1
	Secrets string `json:"secrets"`

	// Storage defines persistent storage for sessions and other data
	// it will use in-memory if none specified
	// see https://www.pomerium.com/docs/topics/data-storage
	// +kubebuilder:validation:Optional
	Storage *Storage `json:"storage"`
}

// ResourceStatus represents the outcome of the latest attempt to reconcile it with Pomerium.
type ResourceStatus struct {
	// ObservedGeneration represents the .metadata.generation that was last presented to Pomerium.
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
	// ObservedAt is when last reconciliation attempt was made.
	ObservedAt metav1.Time `json:"observedAt,omitempty"`
	// Reconciled is whether this object generation was successfully synced with pomerium.
	Reconciled bool `json:"reconciled"`
	// Error that prevented latest observedGeneration to be synchronized with Pomerium.
	// +optional
	Error *string `json:"error"`
}

// PomeriumStatus defines the observed state of Settings
type PomeriumStatus struct {
	// Routes provide per-Ingress status.
	Routes map[string]ResourceStatus `json:"ingress,omitempty"`
	// settingsStatus represent most recent main configuration reconciliation status.
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
