package ingress

import (
	"context"
	"fmt"
	"reflect"

	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/pomerium/ingress-controller/model"
)

func (r *ingressController) updateDependencies(ic *model.IngressConfig) {
	ingKey := model.ObjectKey(ic.Ingress, r.Scheme)
	r.DeleteCascade(ingKey)

	for _, s := range ic.Secrets {
		r.Add(ingKey, model.ObjectKey(s, r.Scheme))
	}
	for _, s := range ic.Services {
		k := model.ObjectKey(s, r.Scheme)
		r.Add(ingKey, k)
		k.Kind = r.endpointsKind
		r.Add(ingKey, k)
	}

	if r.updateStatusFromService != nil {
		r.Add(ingKey, model.Key{NamespacedName: *r.updateStatusFromService, Kind: r.serviceKind})
	}

}

// getDependantIngressFn returns for a given object kind (i.e. a secret) a function
// that would return ingress objects keys that depend from this object
func (r *ingressController) getDependantIngressFn(kind string) func(a client.Object) []reconcile.Request {
	logger := log.FromContext(context.Background()).WithValues("kind", kind)

	return func(a client.Object) []reconcile.Request {
		if !r.isWatching(a) {
			return nil
		}

		name := types.NamespacedName{Name: a.GetName(), Namespace: a.GetNamespace()}
		deps := r.DepsOfKind(model.Key{Kind: kind, NamespacedName: name}, r.ingressKind)
		reqs := make([]reconcile.Request, 0, len(deps))
		for _, k := range deps {
			reqs = append(reqs, reconcile.Request{NamespacedName: k.NamespacedName})
		}
		logger.V(5).Info("watch", "name", fmt.Sprintf("%s/%s", a.GetNamespace(), a.GetName()), "deps", reqs)
		return reqs
	}
}

func (r *ingressController) watchIngressClass(string) func(a client.Object) []reconcile.Request {
	logger := log.FromContext(context.Background())

	return func(a client.Object) []reconcile.Request {
		ctx, cancel := context.WithTimeout(context.Background(), initialReconciliationTimeout)
		defer cancel()

		_ = r.initComplete.yield(ctx)

		ic, ok := a.(*networkingv1.IngressClass)
		if !ok {
			logger.Error(fmt.Errorf("got %s", reflect.TypeOf(a)), "expected IngressClass")
			return nil
		}
		if ic.Spec.Controller != r.controllerName {
			return nil
		}
		il := new(networkingv1.IngressList)
		err := r.Client.List(ctx, il)
		if err != nil {
			logger.Error(err, "list")
			return nil
		}
		deps := make([]reconcile.Request, 0, len(il.Items))
		for i := range il.Items {
			deps = append(deps, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      il.Items[i].Name,
					Namespace: il.Items[i].Namespace,
				},
			})
		}
		logger.V(5).Info("watch", "deps", deps, "ingressClass", a.GetName())
		return deps
	}
}
