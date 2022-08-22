package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

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
