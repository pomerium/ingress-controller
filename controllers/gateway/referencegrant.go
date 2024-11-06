package gateway

import (
	"github.com/hashicorp/go-set/v3"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gateway_v1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"
)

// referenceGrantMap is a map representation of all ReferenceGrants. Keys represent a target object
// (corresponding to a ReferenceGrantTo) and the values represent source objects (corresponding to
// a ReferenceGrantFrom). There are a few subtleties:
//   - A refKey with an empty Name represents any object of that kind within the namespace.
//   - A refKey used as a key in this map may or may not have an empty Name, as a ReferenceGrantTo
//     contains an optional Name field.
//   - A refKey in one of the value collections should always have an empty Name, as a
//     ReferenceGrantFrom cannot reference a specific object by name.
type referenceGrantMap map[refKey]set.Collection[refKey]

func buildReferenceGrantMap(grants []gateway_v1beta1.ReferenceGrant) referenceGrantMap {
	m := referenceGrantMap{}
	for i := range grants {
		g := &grants[i]
		sourceSet := set.FromFunc(g.Spec.From, refKeyForReferenceGrantFrom)
		for _, to := range g.Spec.To {
			k := refKeyForReferenceGrantTo(g.Namespace, to)
			m[k] = safeUnion(m[k], sourceSet)
		}
	}
	return m
}

func (m referenceGrantMap) allowed(from client.Object, toKey refKey) bool {
	// A ReferenceGrant is not required for references within a single namespace.
	if from.GetNamespace() == toKey.Namespace {
		return true
	}

	fromKey := refKeyForObject(from)
	fromKey.Name = ""

	if s := m[toKey]; s != nil && s.Contains(fromKey) {
		return true // specific "To" object is allowed
	}
	toKey.Name = ""
	if s := m[toKey]; s != nil && s.Contains(fromKey) {
		return true // entire "To" namespace is allowed
	}
	return false
}

func safeUnion[T comparable](a, b set.Collection[T]) set.Collection[T] {
	if a == nil {
		return b
	} else if b == nil {
		return a
	}
	return a.Union(b)
}
