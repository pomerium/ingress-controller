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

// SettingsSpec defines the desired state of Settings
type SettingsSpec struct {
}

//+kubebuilder:printcolumn:name="Last Reconciled",type=datetime,JSONPath=`.ts`

// RouteStatus reconciliation status between Ingress spec and Pomerium state
type RouteStatus struct {
	// Reconciled is true if Ingress resource was fully synced with pomerium state
	Reconciled bool `json:"reconciled"`
	// LastReconciled timestamp indicates when the ingress resource was last synced with pomerium
	LastReconciled *metav1.Time `json:"ts,omitempty"`
	// Error is reason most recent reconciliation failed for the route
	Error string `json:"error,omitempty"`
}

// SettingsStatus defines the observed state of Settings
type SettingsStatus struct {
	Routes map[string]RouteStatus `json:"routes"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// Settings is the Schema for the settings API
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
