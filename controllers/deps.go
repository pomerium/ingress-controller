//Package controllers implements shared functions for k8s controllers
package controllers

import (
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/pomerium/ingress-controller/model"
)

// GetDependantMapFunc produces list of dependencies for reconciliation of a given kind
func GetDependantMapFunc(r model.Registry, kind string) handler.MapFunc {
	return func(obj client.Object) []reconcile.Request {
		name := types.NamespacedName{Name: obj.GetName(), Namespace: obj.GetNamespace()}
		deps := r.DepsOfKind(model.Key{
			Kind:           obj.GetObjectKind().GroupVersionKind().Kind,
			NamespacedName: name,
		}, kind)
		reqs := make([]reconcile.Request, 0, len(deps))
		for _, k := range deps {
			reqs = append(reqs, reconcile.Request{NamespacedName: k.NamespacedName})
		}
		return reqs
	}
}
