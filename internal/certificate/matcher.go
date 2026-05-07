package certificate

import (
	"slices"
	"strings"

	"github.com/hashicorp/go-set/v3"
)

// A Matcher matches routes with certificate names.
type Matcher[Key comparable] interface {
	// MissingNames returns any route "from" hostnames which don't have
	// a matching certificate.
	MissingNames() []string
	Update(key Key, certificateNames []string, routeNames []string)
}

type matcher[Key comparable] struct {
	certificates NameIndex[Key]
	routes       NameIndex[Key]
	missing      *set.Set[string]
}

// NewMatcher creates a new Matcher.
func NewMatcher[Key comparable]() Matcher[Key] {
	return &matcher[Key]{
		certificates: NewNameIndex[Key](),
		routes:       NewNameIndex[Key](),
		missing:      set.New[string](0),
	}
}

func (m *matcher[Key]) MissingNames() []string {
	return m.missing.Slice()
}

func (m *matcher[Key]) Update(key Key, certificateNames []string, routeNames []string) {
	check := func(name string) {
		// first remove the name from missing
		m.missing.Remove(name)

		// for each route matching the given name
		for _, route := range m.routes.Lookup(name, true) {
			for _, n := range m.routes.Get(route) {
				// if there's no matching certificate, add the name
				if len(m.certificates.Lookup(n, false)) == 0 {
					m.missing.Insert(n)
				} else {
					m.missing.Remove(n)
				}
			}
		}
	}

	certificateNames = normalizeNames(certificateNames)
	routeNames = normalizeNames(routeNames)

	prev := set.From(m.certificates.Get(key))
	next := set.From(certificateNames)
	if len(certificateNames) == 0 {
		m.certificates.Remove(key)
	} else {
		m.certificates.Add(key, certificateNames)
	}
	// removed certificate name
	for name := range prev.Difference(next).Items() {
		check(name)
	}
	// added certificate name
	for name := range next.Difference(prev).Items() {
		check(name)
	}

	prev = set.From(m.routes.Get(key))
	next = set.From(routeNames)
	if len(routeNames) == 0 {
		m.routes.Remove(key)
	} else {
		m.routes.Add(key, routeNames)
	}
	// removed route name
	for name := range prev.Difference(next).Items() {
		check(name)
	}
	// added route name
	for name := range next.Difference(prev).Items() {
		check(name)
	}
}

func normalizeNames(names []string) []string {
	names = slices.Clone(names)
	for i := range names {
		names[i] = strings.ToLower(strings.TrimSuffix(names[i], "."))
	}
	return names
}
