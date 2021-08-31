package model

import (
	"sync"

	"k8s.io/apimachinery/pkg/types"
)

// Key is dependenciy key
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

type registryItems map[Key]map[Key]bool

type registry struct {
	sync.RWMutex
	items registryItems
}

// NewRegistry creates an empty registry safe for concurrent use
func NewRegistry() Registry {
	return &registry{
		items: make(registryItems),
	}
}

// Add registers dependency between x and y
func (r *registry) Add(x, y Key) {
	r.Lock()
	defer r.Unlock()

	r.items.add(x, y)
	r.items.add(y, x)
}

func (r registryItems) add(x, y Key) {
	rx := r[x]
	if rx == nil {
		rx = make(map[Key]bool)
		r[x] = rx
	}
	rx[y] = true
}

func (r registryItems) del(x, y Key) {
	rx := r[x]
	delete(rx, y)
	if len(rx) == 0 {
		delete(r, x)
	}
}

// Deps returns list of objects that are dependent
func (r *registry) Deps(x Key) []Key {
	r.RLock()
	defer r.RUnlock()

	rx := r.items[x]
	keys := make([]Key, 0, len(rx))
	for k := range rx {
		keys = append(keys, k)
	}
	return keys
}

// DepsOfKind returns list of objects that are dependent and are of a particular kind
func (r *registry) DepsOfKind(x Key, kind string) []Key {
	r.RLock()
	defer r.RUnlock()

	rx := r.items[x]
	keys := make([]Key, 0, len(rx))
	for k := range rx {
		if k.Kind == kind {
			keys = append(keys, k)
		}
	}
	return keys
}

func (r *registry) DeleteCascade(x Key) {
	r.Lock()
	defer r.Unlock()

	for k := range r.items[x] {
		r.items.del(x, k)
		r.items.del(k, x)
	}
}
