package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
type PolicyFilter struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec PolicyFilterSpec `json:"spec,omitempty"`

	// +kubebuilder:default={conditions: {{type: "Valid", status: "Unknown", reason:"Pending", message:"Waiting for controller", lastTransitionTime: "1970-01-01T00:00:00Z"}}}
	Status PolicyFilterStatus `json:"status,omitempty"`
}

type PolicyFilterSpec struct {
	// Policy rules in Pomerium Policy Language (PPL) syntax. May be expressed
	// in either YAML or JSON format.
	PPL string `json:"ppl,omitempty"`
}

type PolicyFilterStatus struct {
	// +optional
	// +listType=map
	// +listMapKey=type
	// +kubebuilder:validation:MaxItems=8
	// +kubebuilder:default={{type: "Valid", status: "Unknown", reason:"Pending", message:"Waiting for controller", lastTransitionTime: "1970-01-01T00:00:00Z"}}
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

func init() {
	SchemeBuilder.Register(&PolicyFilter{})
}
