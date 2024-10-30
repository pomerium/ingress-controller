package pomerium

import (
	"context"

	"k8s.io/apimachinery/pkg/types"

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

type GatewayReconciler interface {
	// GatewaySetConfig updates the entire Gateway-defined route configuration.
	GatewaySetConfig(ctx context.Context, config *model.GatewayConfig) (changes bool, err error)

	// GatewayUpsertRoute updates a single Gateway-defined route.
	//GatewayUpsertRoute(ctx context.Context, route *model.GatewayHTTPRouteConfig) (changes bool, err error)

	// GatewayDeleteRoute deletes a single Gateway-defined route.
	//GatewayDeleteRoute(ctx context.Context, name types.NamespacedName) (changes bool, err error)
}

// ConfigReconciler only updates global parameters and does not deal with individual routes
type ConfigReconciler interface {
	// SetConfig updates just the shared config settings
	SetConfig(ctx context.Context, cfg *model.Config) (changes bool, err error)
}
