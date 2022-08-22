package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// NamespaceSpec defines Pomerium Enterprise Console namespace
type NamespaceSpec struct {
	// Secret is a name of an Opaque Secret within same namespace, that must have two fields
	// token for Pomerium service account JWT token, and endpoint containing URL to API gRPC endpoint.
	Secret string `json:"secret"`
	// ParentID to create a new namespace below specified parent, mutually exclusive with RefID.
	ParentID *string `json:"parentId"`
	// RefID to refer to an existing Pomerium namespace, mutually exclusive with ParentID.
	RefID *string `json:"refId,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:resource:path=namespaces

// Namespace define Pomerium Enterprise Console namespace
type Namespace struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   NamespaceSpec  `json:"spec,omitempty"`
	Status ResourceStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// NamespaceList contains a list of Namespaces
type NamespaceList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Namespace `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Namespace{}, &NamespaceList{})
}
