package model

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/types"
)

func TestRegistry(t *testing.T) {
	r := NewRegistry()
	a := Key{Kind: "a", NamespacedName: types.NamespacedName{Name: "a", Namespace: "a"}}
	b := Key{Kind: "b", NamespacedName: types.NamespacedName{Name: "b", Namespace: "b"}}
	c := Key{Kind: "c", NamespacedName: types.NamespacedName{Name: "c", Namespace: "c"}}
	d := Key{Kind: "d", NamespacedName: types.NamespacedName{Name: "d", Namespace: "d"}}

	r.Add(a, a)
	r.Add(a, b)
	r.Add(a, c)
	r.Add(c, d)

	assert.ElementsMatch(t, []Key{b, c}, r.Deps(a))
	assert.ElementsMatch(t, []Key{a}, r.Deps(b))
	assert.ElementsMatch(t, []Key{a}, r.DepsOfKind(b, "a"))
	assert.ElementsMatch(t, []Key{a, d}, r.Deps(c))
	r.DeleteCascade(c)
	assert.ElementsMatch(t, []Key{b}, r.Deps(a))
	assert.Empty(t, r.Deps(d))
	r.DeleteCascade(a)
	if !assert.Empty(t, r.(*registry).items) {
		t.Logf("%+v", r)
	}
}
