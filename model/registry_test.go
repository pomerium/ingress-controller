package model_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/types"

	"github.com/pomerium/ingress-controller/model"
)

func TestRegistry(t *testing.T) {
	r := model.NewRegistry()
	a := model.Key{Kind: "a", NamespacedName: types.NamespacedName{Name: "a", Namespace: "a"}}
	b := model.Key{Kind: "b", NamespacedName: types.NamespacedName{Name: "b", Namespace: "b"}}
	c := model.Key{Kind: "c", NamespacedName: types.NamespacedName{Name: "c", Namespace: "c"}}
	d := model.Key{Kind: "d", NamespacedName: types.NamespacedName{Name: "d", Namespace: "d"}}

	r.Add(a, b)
	r.Add(a, c)
	r.Add(c, d)

	assert.ElementsMatch(t, []model.Key{b, c}, r.Deps(a))
	assert.ElementsMatch(t, []model.Key{a}, r.Deps(b))
	assert.ElementsMatch(t, []model.Key{a}, r.DepsOfKind(b, "a"))
	assert.ElementsMatch(t, []model.Key{a, d}, r.Deps(c))
	r.DeleteCascade(c)
	assert.ElementsMatch(t, []model.Key{b}, r.Deps(a))
	assert.Empty(t, r.Deps(d))
	r.DeleteCascade(a)
	assert.Empty(t, r)
}
