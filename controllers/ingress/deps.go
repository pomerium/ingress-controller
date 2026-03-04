package ingress

import (
	"context"
	"fmt"
	"reflect"

	discoveryv1 "k8s.io/api/discovery/v1"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/pomerium/ingress-controller/model"
)

// getDependantIngressFn returns for a given object kind (i.e. a secret) a function
// that would return ingress objects keys that depend from this object
func (r *ingressController) getDependantIngressFn(kind string) handler.MapFunc {
	return func(ctx context.Context, a client.Object) []reconcile.Request {
		if !r.isWatching(a) {
			return nil
		}

		lookupKind := kind
		name := types.NamespacedName{Name: a.GetName(), Namespace: a.GetNamespace()}

		// For EndpointSlice, look up dependencies by the associated Service name.
		// EndpointSlices have a label "kubernetes.io/service-name" that identifies
		// which Service they belong to. Dependencies are registered under the Service,
		// not the EndpointSlice, so we need to translate the lookup.
		if kind == r.endpointSliceKind {
			serviceName := a.GetLabels()[discoveryv1.LabelServiceName]
			if serviceName == "" {
				log.FromContext(ctx).V(1).Info("EndpointSlice missing service-name label",
					"endpointSlice", name)
				return nil
			}
			name.Name = serviceName
			lookupKind = r.serviceKind
		}

		deps := r.DepsOfKind(model.Key{Kind: lookupKind, NamespacedName: name}, r.ingressKind)
		reqs := make([]reconcile.Request, 0, len(deps))
		for _, k := range deps {
			reqs = append(reqs, reconcile.Request{NamespacedName: k.NamespacedName})
		}
		log.FromContext(ctx).
			WithValues("kind", kind).V(5).
			Info("watch", "name", fmt.Sprintf("%s/%s", a.GetNamespace(), a.GetName()), "deps", reqs)
		return reqs
	}
}

func (r *ingressController) watchIngressClass() handler.MapFunc {
	return func(ctx context.Context, a client.Object) []reconcile.Request {
		logger := log.FromContext(ctx)
		ctx, cancel := context.WithTimeout(ctx, initialReconciliationTimeout)
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
