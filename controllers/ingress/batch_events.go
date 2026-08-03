package ingress

import (
	"context"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func (r *ingressController) batchSignalPredicate(source string) predicate.Predicate {
	if r.batchCoordinator == nil {
		return predicate.Funcs{}
	}
	return predicate.Funcs{
		CreateFunc: func(event.CreateEvent) bool {
			r.batchCoordinator.Signal(source)
			return true
		},
		UpdateFunc: func(event.UpdateEvent) bool {
			r.batchCoordinator.Signal(source)
			return true
		},
		DeleteFunc: func(event.DeleteEvent) bool {
			r.batchCoordinator.Signal(source)
			return true
		},
		GenericFunc: func(event.GenericEvent) bool {
			r.batchCoordinator.Signal(source)
			return true
		},
	}
}

func (r *ingressController) batchSignalMap(
	source string,
	mapper handler.MapFunc,
) handler.MapFunc {
	if r.batchCoordinator == nil {
		return mapper
	}
	return func(ctx context.Context, obj client.Object) []reconcile.Request {
		requests := mapper(ctx, obj)
		if len(requests) > 0 {
			r.batchCoordinator.Signal(source)
		}
		return requests
	}
}
