package certificate_test

import (
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/pomerium/ingress-controller/internal/certificate"
)

func TestNameIndex(t *testing.T) {
	t.Parallel()

	idx := certificate.NewNameIndex[int]()

	idx.Add(1, []string{"*.example.com"})
	keys := idx.Lookup("www.example.com")
	slices.Sort(keys)
	assert.Equal(t, []int{1}, keys)

	keys = idx.Lookup("wWw.ExaMPle.com")
	slices.Sort(keys)
	assert.Equal(t, []int{1}, keys, "should be case insensitive")

	keys = idx.Lookup("www.example.com.")
	slices.Sort(keys)
	assert.Equal(t, []int{1}, keys, "should strip trailing dots")

	idx.Add(2, []string{"www.example.com"})
	keys = idx.Lookup("www.example.com")
	slices.Sort(keys)
	assert.Equal(t, []int{1, 2}, keys)

	names := idx.Get(2)
	slices.Sort(names)
	assert.Equal(t, []string{"www.example.com"}, names)

	idx.Remove(1)
	keys = idx.Lookup("www.example.com")
	slices.Sort(keys)
	assert.Equal(t, []int{2}, keys)

	idx.Add(3, []string{"a.example.com"})
	idx.Add(4, []string{"b.example.com"})
	idx.Add(5, []string{"c.example.com"})
	keys = idx.Lookup("*.example.com")
	slices.Sort(keys)
	assert.Equal(t, []int{2, 3, 4, 5}, keys)
	keys = idx.Lookup("*.com")
	slices.Sort(keys)
	assert.Empty(t, keys, "should only support single-level wildcards")

	keys = idx.Keys()
	slices.Sort(keys)
	assert.Equal(t, []int{2, 3, 4, 5}, keys)

	names = idx.Names()
	slices.Sort(names)
	assert.Equal(t, []string{"a.example.com", "b.example.com", "c.example.com", "www.example.com"}, names)

	idx.Add(6, []string{"localhost"})
	keys = idx.Lookup("localhost")
	assert.Equal(t, []int{6}, keys)
	keys = idx.Lookup("*")
	assert.Equal(t, []int{6}, keys)

	idx.Remove(2)
	idx.Remove(3)
	idx.Remove(4)
	idx.Remove(5)
	idx.Remove(6)

	keys = idx.Lookup("*")
	assert.Empty(t, keys)
}
