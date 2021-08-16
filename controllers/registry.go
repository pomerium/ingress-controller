package controllers

import (
	"sync"

	"k8s.io/apimachinery/pkg/types"
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
	// DeleteCascade deletes key x and also any dependent keys that do not have other dependencies
	DeleteCascade(x Key)
}

type registry map[Key]map[Key]bool

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

func (r registry) DeleteCascade(x Key) {
	for k := range r[x] {
		r.del(x, k)
		r.del(k, x)
	}
}

type safeRegistry struct {
	sync.RWMutex
	registry
}

func (sr *safeRegistry) Add(x, y Key) {
	sr.Lock()
	defer sr.Unlock()
	sr.registry.Add(x, y)
}

func (sr *safeRegistry) Deps(x Key) []Key {
	sr.RLock()
	defer sr.RUnlock()
	return sr.registry.Deps(x)
}

func (sr *safeRegistry) DeleteCascade(x Key) {
	sr.Lock()
	defer sr.Unlock()
	sr.registry.DeleteCascade(x)
}

func NewRegistry() Registry {
	return &safeRegistry{registry: make(registry)}
}
