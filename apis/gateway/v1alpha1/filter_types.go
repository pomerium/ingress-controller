// Package v1alpha1 contains custom resource definitions for use with the Gateway API.
//
//nolint:lll
package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// PolicyFilter represents a Pomerium policy that can be attached to a particular route defined
// via the Kubernetes Gateway API.
//
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
type PolicyFilter struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// Spec defines the content of the policy.
	Spec PolicyFilterSpec `json:"spec,omitempty"`

	// Status contains the status of the policy (e.g. is the policy valid).
	Status PolicyFilterStatus `json:"status,omitempty"`
}

// PolicyFilterSpec defines policy rules.
type PolicyFilterSpec struct {
	// Policy rules in Pomerium Policy Language (PPL) syntax. May be expressed
	// in either YAML or JSON format.
	PPL string `json:"ppl,omitempty"`
}

// PolicyFilterStatus represents the state of a PolicyFilter.
type PolicyFilterStatus struct {
	// Conditions describe the current state of the PolicyFilter.
	//
	// +optional
	// +listType=map
	// +listMapKey=type
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

func init() {
	SchemeBuilder.Register(&PolicyFilter{})
}
