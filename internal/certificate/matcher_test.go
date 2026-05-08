package certificate_test

import (
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/pomerium/ingress-controller/internal/certificate"
)

func TestMatcher(t *testing.T) {
	t.Parallel()

	names := func(m certificate.Matcher[int]) []string {
		ns := m.MissingNames()
		slices.Sort(ns)
		return ns
	}

	t.Run("basic", func(t *testing.T) {
		t.Parallel()
		m := certificate.NewMatcher[int]()
		assert.Empty(t, names(m))

		m.Update(1, nil, []string{"www.example.com"})
		assert.Equal(t, []string{"www.example.com"}, names(m))

		m.Update(2, []string{"www.example.com"}, nil)
		assert.Empty(t, names(m))

		m.Update(3, []string{"*.example.com"}, nil)
		assert.Empty(t, names(m))

		m.Update(2, nil, nil)
		assert.Empty(t, names(m))

		m.Update(3, nil, nil)
		assert.Equal(t, []string{"www.example.com"}, names(m))

		m.Update(1, nil, nil)
		assert.Empty(t, names(m))
	})

	t.Run("wildcard cert clears existing missing entry", func(t *testing.T) {
		t.Parallel()
		m := certificate.NewMatcher[int]()

		m.Update(1, nil, []string{"www.example.com"})
		assert.Equal(t, []string{"www.example.com"}, names(m))

		m.Update(2, []string{"*.example.com"}, nil)
		assert.Empty(t, names(m))
	})

	t.Run("multi-name route partial coverage", func(t *testing.T) {
		t.Parallel()
		m := certificate.NewMatcher[int]()

		m.Update(1, nil, []string{"www.example.com", "api.example.com"})
		assert.Equal(t, []string{"api.example.com", "www.example.com"}, names(m))

		m.Update(2, []string{"www.example.com"}, nil)
		assert.Equal(t, []string{"api.example.com"}, names(m))

		m.Update(2, []string{"*.example.com"}, nil)
		assert.Empty(t, names(m))

		m.Update(1, nil, []string{"www.example.com"})
		assert.Empty(t, names(m))
	})

	t.Run("shared name across routes", func(t *testing.T) {
		t.Parallel()
		m := certificate.NewMatcher[int]()

		m.Update(1, nil, []string{"www.example.com"})
		m.Update(2, nil, []string{"www.example.com"})
		assert.Equal(t, []string{"www.example.com"}, names(m))

		m.Update(1, nil, nil)
		assert.Equal(t, []string{"www.example.com"}, names(m),
			"removing one route should keep a name still referenced")

		m.Update(2, nil, nil)
		assert.Empty(t, names(m))
	})

	t.Run("wildcard cert added before route", func(t *testing.T) {
		t.Parallel()
		m := certificate.NewMatcher[int]()

		m.Update(1, []string{"*.example.com"}, nil)
		assert.Empty(t, names(m))

		m.Update(2, nil, []string{"www.example.com"})
		assert.Empty(t, names(m))

		m.Update(1, nil, nil)
		assert.Equal(t, []string{"www.example.com"}, names(m))
	})

	t.Run("wildcard route does not match cert", func(t *testing.T) {
		t.Parallel()
		m := certificate.NewMatcher[int]()

		m.Update(1, []string{"www.example.com"}, nil)
		assert.Empty(t, names(m))

		m.Update(2, nil, []string{"*.example.com"})
		assert.Equal(t, []string{"*.example.com"}, names(m))
	})

	t.Run("case insensitive", func(t *testing.T) {
		t.Parallel()
		m := certificate.NewMatcher[int]()

		m.Update(1, []string{"www.example.com"}, nil)
		assert.Empty(t, names(m))

		m.Update(2, nil, []string{"wWw.ExAmPlE.com"})
		assert.Empty(t, names(m))
	})

	t.Run("trailing dot", func(t *testing.T) {
		t.Parallel()
		m := certificate.NewMatcher[int]()

		m.Update(1, []string{"www.example.com."}, nil)
		assert.Empty(t, names(m))

		m.Update(2, nil, []string{"www.example.com"})
		assert.Empty(t, names(m))
	})
}
