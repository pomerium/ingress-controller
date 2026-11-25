package ingress

import (
	"context"
	"fmt"
	"reflect"

	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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

		name := types.NamespacedName{Name: a.GetName(), Namespace: a.GetNamespace()}
		deps := r.DepsOfKind(model.Key{Kind: kind, NamespacedName: name}, r.ingressKind)
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

// getDependantIngressFromEndpointSlice returns the ingresses that depend on the Service
// the EndpointSlice belongs to. Unlike other objects, EndpointSlices have generated names,
// and are associated with their Service via the kubernetes.io/service-name label.
// Ingresses track the Service they refer to as a dependency, so a change to any of its
// EndpointSlices is a change to the Service endpoints.
func (r *ingressController) getDependantIngressFromEndpointSlice(ctx context.Context, a client.Object) []reconcile.Request {
	svcName := a.GetLabels()[discoveryv1.LabelServiceName]
	if svcName == "" {
		return nil
	}
	svc := &corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: svcName, Namespace: a.GetNamespace()}}
	return r.getDependantIngressFn(r.serviceKind)(ctx, svc)
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
