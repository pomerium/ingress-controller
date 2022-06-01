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

// IdentityProvider see https://www.pomerium.com/docs/identity-providers/
type IdentityProvider struct {
	// Provider one of accepted providers https://www.pomerium.com/reference/#identity-provider-name
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Enum=auth0;azure;google;okta;onelogin;oidc;ping;github
	Provider string `json:"provider"`
	// URL is identity provider url, see https://www.pomerium.com/reference/#identity-provider-url
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:Type=string
	// +kubebuilder:validation:Format=uri
	// +kubebuilder:validation:Pattern=`^https://`
	URL *string `json:"url"`
	// Secret refers to a k8s secret containing IdP provider specific parameters
	// and must contain at least `client_id` and `client_secret` map values,
	// an optional `service_account` field, mapped to https://www.pomerium.com/reference/#identity-provider-service-account
	// +kubebuilder:validation:Required
	Secret string `json:"secret"`
	// ServiceAccountFromSecret is a convenience way to build a value for `idp_service_account` from
	// secret map values, see https://www.pomerium.com/docs/identity-providers/
	// +optional
	ServiceAccountFromSecret *string `json:"serviceAccountFromSecret,omitempty"`
	// RequestParams see https://www.pomerium.com/reference/#identity-provider-request-params
	// +optional
	RequestParams map[string]string `json:"requestParams,omitempty"`
	// RequestParamsSecret is a reference to a secret for additional parameters you'd prefer not to provide in plaintext
	// +optional
	RequestParamsSecret *string `json:"requestParamsSecret,omitempty"`
	// Scopes see https://www.pomerium.com/reference/#identity-provider-scopes
	// +optional
	Scopes []string `json:"scopes,omitempty"`

	// Specifies refresh settings
	// +optional
	RefreshDirectory *RefreshDirectorySettings `json:"refresh_directory"`
}

// RefreshDirectorySettings defines how frequently should
type RefreshDirectorySettings struct {
	// +kubebuilder:validation:Format=duration
	Interval metav1.Duration `json:"interval"`
	// +kubebuilder:validation:Format=duration
	Timeout metav1.Duration `json:"timeout"`
}

// Timeouts regulate common timeout settings
type Timeouts struct {
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

// SettingsSpec defines the desired state of Settings
type SettingsSpec struct {
	// Authenticate see
	// +kubebuilder:validation:Required
	Authenticate Authenticate `json:"authenticate"`
	// IdentityProvider see https://www.pomerium.com/docs/identity-providers/
	// +kubebuilder:validation:Required
	IdentityProvider IdentityProvider `json:"identityProvider"`
	// Certificates is a list of secrets of type TLS to use
	// +optional
	Certificates []string `json:"certificates"`
}

//+kubebuilder:printcolumn:name="Last Reconciled",type=datetime,JSONPath=`.ts`

// RouteStatus provides high level status between the last observed ingress object and pomerium state
type RouteStatus struct {
	// Reconciled is true if Ingress resource was fully synced with pomerium state
	Reconciled bool `json:"reconciled"`
	// LastReconciled timestamp indicates when the ingress resource was last synced with pomerium
	LastReconciled *metav1.Time `json:"lastReconciled,omitempty"`
	// Error is reason most recent reconciliation failed for the route
	Error string `json:"error,omitempty"`
}

// SettingsStatus defines the observed state of Settings
type SettingsStatus struct {
	Routes map[string]RouteStatus `json:"ingress"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// Settings define runtime-configurable Pomerium settings
// that do not fall into the category of deployment parameters
type Settings struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   SettingsSpec   `json:"spec,omitempty"`
	Status SettingsStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// SettingsList contains a list of Settings
type SettingsList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Settings `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Settings{}, &SettingsList{})
}
