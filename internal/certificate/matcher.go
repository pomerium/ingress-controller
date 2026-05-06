package certificate

// A Matcher matches routes with certificate names.
type Matcher interface {
	// MissingNames returns any route "from" hostnames which don't have
	// a matching certificate.
	MissingNames() []string
	Update(reference [2]string, certificateNames []string, routeNames []string)
}

type matcher struct {
	certificates NameIndex[[2]string]
	missing      NameIndex[[2]string]
	routes       NameIndex[[2]string]
}

func NewMatcher() Matcher {
	return &matcher{
		certificates: NewNameIndex[[2]string](),
		missing:      NewNameIndex[[2]string](),
		routes:       NewNameIndex[[2]string](),
	}
}

func (m *matcher) MissingNames() []string {
	return m.missing.Names()
}

func (m *matcher) Update(reference [2]string, certificateNames []string, routeNames []string) {
}
