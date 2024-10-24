package pomerium

import (
	"context"

	"k8s.io/apimachinery/pkg/types"
	gatev1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/pomerium/ingress-controller/model"
)

// IngressReconciler updates pomerium configuration based on provided network resources
// it is not expected to be thread safe
type IngressReconciler interface {
	// Upsert should update or create the pomerium routes corresponding to this ingress
	Upsert(ctx context.Context, ic *model.IngressConfig) (changes bool, err error)
	// Set configuration to match provided ingresses and shared config settings
	Set(ctx context.Context, ics []*model.IngressConfig) (changes bool, err error)
	// Delete should delete pomerium routes corresponding to this ingress name
	Delete(ctx context.Context, namespacedName types.NamespacedName) (changes bool, err error)
}

// ConfigReconciler only updates global parameters and does not deal with individual routes
type ConfigReconciler interface {
	// SetConfig updates just the shared config settings
	SetConfig(ctx context.Context, cfg *model.Config) (changes bool, err error)
}

type GatewayReconciler interface {
	Update(ctx context.Context, gateway *gatev1.Gateway) (changes bool, err error)
	// XXX: how do we handle deleting a Gateway?
}
