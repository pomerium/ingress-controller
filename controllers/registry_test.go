package controllers_test

import (
	"testing"

	"github.com/pomerium/ingress-controller/controllers"
	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/types"
)

func TestRegistry(t *testing.T) {
	r := controllers.NewRegistry()
	a := controllers.Key{Kind: "a", NamespacedName: types.NamespacedName{Name: "a", Namespace: "a"}}
	b := controllers.Key{Kind: "b", NamespacedName: types.NamespacedName{Name: "b", Namespace: "b"}}
	c := controllers.Key{Kind: "c", NamespacedName: types.NamespacedName{Name: "c", Namespace: "c"}}
	d := controllers.Key{Kind: "d", NamespacedName: types.NamespacedName{Name: "d", Namespace: "d"}}

	r.Add(a, b)
	r.Add(a, c)
	r.Add(c, d)

	assert.ElementsMatch(t, []controllers.Key{b, c}, r.Deps(a))
	assert.ElementsMatch(t, []controllers.Key{a}, r.Deps(b))
	assert.ElementsMatch(t, []controllers.Key{a}, r.DepsOfKind(b, "a"))
	assert.ElementsMatch(t, []controllers.Key{a, d}, r.Deps(c))
	r.DeleteCascade(c)
	assert.ElementsMatch(t, []controllers.Key{b}, r.Deps(a))
	assert.Empty(t, r.Deps(d))
	r.DeleteCascade(a)
	assert.Empty(t, r)
}
