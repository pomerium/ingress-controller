package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// PomeriumServiceAgentic is the agentic authorization server.
const PomeriumServiceAgentic = "agentic"

// PomeriumServiceSpec names a service Pomerium runs itself.
type PomeriumServiceSpec struct {
	// Service is the Pomerium-internal service to route to.
	//
	// "agentic" is the agentic authorization server. Its endpoints have no
	// authorization of their own: the policies of the Ingresses routing to it
	// are what decide who may summon, exchange and approve.
	//
	// +kubebuilder:validation:Enum=agentic
	Service string `json:"service"`
}

// PomeriumService makes a service Pomerium runs itself available as an
// Ingress backend in its namespace. An Ingress path routes to it with
//
//	backend:
//	  resource:
//	    apiGroup: ingress.pomerium.io
//	    kind: PomeriumService
//	    name: <this object's name>
//
// Whoever may create a PomeriumService in a namespace may put that service
// behind Ingresses there with policies of their choosing, so creating one is
// meant to be restricted by RBAC.
//
// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Namespaced
// +kubebuilder:printcolumn:name="Service",type=string,JSONPath=`.spec.service`
type PomeriumService struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec PomeriumServiceSpec `json:"spec"`
}

//+kubebuilder:object:root=true

// PomeriumServiceList contains a list of PomeriumService.
type PomeriumServiceList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []PomeriumService `json:"items"`
}

func init() {
	SchemeBuilder.Register(&PomeriumService{}, &PomeriumServiceList{})
}
