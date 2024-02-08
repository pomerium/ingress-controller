// Package deps implements dependencies management
package deps

import (
	"context"

	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/pomerium/ingress-controller/model"
)

// GetDependantMapFunc produces list of dependencies for reconciliation of a given kind
func GetDependantMapFunc(r model.Registry, kind string) handler.MapFunc {
	return func(ctx context.Context, obj client.Object) []reconcile.Request {
		key := model.Key{
			Kind:           kind,
			NamespacedName: types.NamespacedName{Name: obj.GetName(), Namespace: obj.GetNamespace()},
		}
		deps := r.Deps(key)
		reqs := make([]reconcile.Request, 0, len(deps))
		for _, k := range deps {
			reqs = append(reqs, reconcile.Request{NamespacedName: k.NamespacedName})
		}
		log.FromContext(ctx).V(1).Info("watch deps", "src", key, "dst", reqs, "deps", r.Deps(key))
		return reqs
	}
}
