package model

import (
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type Key struct {
	Kind string
	types.NamespacedName
}

// Registry is used to keep track of dependencies between kubernetes objects
// i.e. ingress depends on secret and service configurations
// no dependency subordination is tracked
type Registry interface {
	// Add registers a dependency between x,y
	Add(x, y Key)
	// Deps returns list of dependencies given object key has
	Deps(x Key) []Key
	DepsOfKind(x Key, kind string) []Key
	// DeleteCascade deletes key x and also any dependent keys that do not have other dependencies
	DeleteCascade(x Key)
}

type registry map[Key]map[Key]bool

// ObjectKey returns a registry key for a given kubernetes object
// the object must be properly initialized (GVK, name, namespace)
func ObjectKey(obj client.Object) Key {
	name := types.NamespacedName{Name: obj.GetName(), Namespace: obj.GetNamespace()}
	kind := obj.GetObjectKind().GroupVersionKind().Kind
	return Key{kind, name}
}

// Add registers dependency between x and y
func (r registry) Add(x, y Key) {
	r.add(x, y)
	r.add(y, x)
}

func (r registry) add(x, y Key) {
	rx := r[x]
	if rx == nil {
		rx = make(map[Key]bool)
		r[x] = rx
	}
	rx[y] = true
}

func (r registry) del(x, y Key) {
	rx := r[x]
	if len(rx) == 0 {
		delete(r, x)
		return
	}
	delete(rx, y)
	if len(rx) == 0 {
		delete(r, x)
	}
}

// Deps returns list of objects that are dependent
func (r registry) Deps(x Key) []Key {
	rx := r[x]
	keys := make([]Key, 0, len(rx))
	for k := range rx {
		keys = append(keys, k)
	}
	return keys
}

// DepsOfKind returns list of objects that are dependent and are of a particular kind
func (r registry) DepsOfKind(x Key, kind string) []Key {
	rx := r[x]
	keys := make([]Key, 0, len(rx))
	for k := range rx {
		if k.Kind == kind {
			keys = append(keys, k)
		}
	}
	return keys
}

func (r registry) DeleteCascade(x Key) {
	for k := range r[x] {
		r.del(x, k)
		r.del(k, x)
	}
}

func NewRegistry() Registry {
	return make(registry)
}
