package generic

import (
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

// NewPredicateFuncs is a wrapper around predicate.NewTypedPredicateFuncs[T]
// that converts the typed predicate functions back to untyped variants, suitable
// for use in the current controller-runtime API.
//
// When controller-runtime is updated to use generic builders, this function
// can be removed. See https://github.com/kubernetes-sigs/controller-runtime/pull/2784
func NewPredicateFuncs[T client.Object](f func(T) bool) predicate.TypedFuncs[client.Object] {
	return asUntypedPredicateFuncs(predicate.NewTypedPredicateFuncs(f))
}

func asUntypedPredicateFuncs[T client.Object](p predicate.TypedFuncs[T]) predicate.TypedFuncs[client.Object] {
	return predicate.TypedFuncs[client.Object]{
		CreateFunc: func(e event.TypedCreateEvent[client.Object]) bool {
			return p.CreateFunc(event.TypedCreateEvent[T]{
				Object: e.Object.(T),
			})
		},
		DeleteFunc: func(e event.TypedDeleteEvent[client.Object]) bool {
			return p.DeleteFunc(event.TypedDeleteEvent[T]{
				Object:             e.Object.(T),
				DeleteStateUnknown: e.DeleteStateUnknown,
			})
		},
		UpdateFunc: func(e event.TypedUpdateEvent[client.Object]) bool {
			return p.UpdateFunc(event.TypedUpdateEvent[T]{
				ObjectOld: e.ObjectOld.(T),
				ObjectNew: e.ObjectNew.(T),
			})
		},
		GenericFunc: func(e event.TypedGenericEvent[client.Object]) bool {
			return p.GenericFunc(event.TypedGenericEvent[T]{
				Object: e.Object.(T),
			})
		},
	}
}
