package model

import (
	"sync"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func KeyForObject(obj client.Object) Key {
	return Key{
		Kind:           obj.GetObjectKind().GroupVersionKind().Kind,
		NamespacedName: namespacedNameForObject(obj),
	}
}

func namespacedNameForObject(obj client.Object) types.NamespacedName {
	return types.NamespacedName{
		Name:      obj.GetName(),
		Namespace: obj.GetNamespace(),
	}
}

// TLSSecretsMap tracks dependencies on TLS secrets, in order to know if some
// modification removes the last dependency on a secret.
type TLSSecretsMap struct {
	mu sync.Mutex

	// Mapping from entities to the names of the TLS secrets they reference.
	deps map[Key]map[types.NamespacedName]struct{}

	// Reverse mapping from TLS secret names to the entities that reference them.
	reverseDeps map[types.NamespacedName]map[Key]struct{}
}

func NewTLSSecretsMap() *TLSSecretsMap {
	return &TLSSecretsMap{
		deps:        make(map[Key]map[types.NamespacedName]struct{}),
		reverseDeps: make(map[types.NamespacedName]map[Key]struct{}),
	}
}

func (m *TLSSecretsMap) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()
	clear(m.deps)
	clear(m.reverseDeps)
}

// UpdateIngress updates the TLS secret dependencies for ic and returns the
// names of any newly-unreferenced secrets.
func (m *TLSSecretsMap) UpdateIngress(ic *IngressConfig) []types.NamespacedName {
	currentSecrets := make(map[types.NamespacedName]struct{})
	for n, s := range ic.Secrets {
		if s.Type == corev1.SecretTypeTLS {
			currentSecrets[n] = struct{}{}
		}
	}
	return m.UpdateEntity(KeyForObject(ic), currentSecrets)
}

// UpdateConfig updates the TLS secret dependencies for cfg and returns the
// names of any newly-unreferenced secrets.
func (m *TLSSecretsMap) UpdateConfig(cfg *Config) []types.NamespacedName {
	currentSecrets := make(map[types.NamespacedName]struct{})
	for n := range cfg.Certs {
		currentSecrets[n] = struct{}{}
	}
	for _, s := range cfg.CASecrets {
		currentSecrets[namespacedNameForObject(s)] = struct{}{}
	}
	return m.UpdateEntity(KeyForObject(cfg), currentSecrets)
}

// UpdateEntity updates the TLS secret dependencies for n and returns the
// names of any newly-unreferenced secrets.
func (m *TLSSecretsMap) UpdateEntity(
	n Key, newDeps map[types.NamespacedName]struct{},
) []types.NamespacedName {
	m.mu.Lock()
	defer m.mu.Unlock()

	previousSecrets := m.deps[n]
	m.deps[n] = newDeps

	for s := range newDeps {
		ensureMapEntry(m.reverseDeps, s)
		m.reverseDeps[s][n] = struct{}{}

		delete(previousSecrets, s)
	}

	return m.removeReverseDeps(n, previousSecrets)
}

// RemoveEntity updates the secrets map to remove all current dependencies of
// the given entity and returns the names of any newly-unreferenced secrets.
func (m *TLSSecretsMap) RemoveEntity(n Key) []types.NamespacedName {
	m.mu.Lock()
	defer m.mu.Unlock()

	previousSecrets := m.deps[n]
	m.deps[n] = make(map[types.NamespacedName]struct{})

	return m.removeReverseDeps(n, previousSecrets)
}

func (m *TLSSecretsMap) removeReverseDeps(
	n Key, previousSecrets map[types.NamespacedName]struct{},
) []types.NamespacedName {
	var unreferencedSecrets []types.NamespacedName
	for s := range previousSecrets {
		delete(m.reverseDeps[s], n)
		if len(m.reverseDeps[s]) == 0 {
			unreferencedSecrets = append(unreferencedSecrets, s)
		}
	}
	return unreferencedSecrets
}

func (m *TLSSecretsMap) Add(entity Key, secret types.NamespacedName) {
	m.mu.Lock()
	defer m.mu.Unlock()
	ensureMapEntry(m.deps, entity)
	m.deps[entity][secret] = struct{}{}
	ensureMapEntry(m.reverseDeps, secret)
	m.reverseDeps[secret][entity] = struct{}{}
}

func ensureMapEntry[A, B comparable](m map[A]map[B]struct{}, k A) {
	if _, exists := m[k]; !exists {
		m[k] = make(map[B]struct{})
	}
}
