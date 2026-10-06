package entities

import "github.com/Hyzokaaa/opencroft/internal/route/domain/enums"

// Route maps a domain to a container address and port — and, optionally, some
// of its paths somewhere else.
type Route struct {
	Domain string
	Target string
	Port   int
	SSL    bool
	// Certificates overrides where the certificate is read from.
	Certificates string
	// Paths send a prefix of the domain elsewhere: /api/ to a backend while
	// the rest goes to the web. The longest prefix that matches wins, which is
	// how nginx itself decides.
	Paths []PathRoute
	// Aliases are more names for the same thing: a customer's own domain
	// answering exactly as this one does, paths and websockets included.
	// Each has a certificate of its own, since nobody can vouch for another
	// party's domain in one certificate they do not control.
	Aliases []Alias
	State   enums.ManagedState
	File    string
	Content string

	// nil when nobody checked whether anything is listening.
	answers *bool
}

// Alias is one more name a route answers on. Until its certificate exists it
// is served over http, so that the authority can reach it to issue one.
type Alias struct {
	Domain string
	SSL    bool
	// Certificates is where its certificate is read from: its own, in
	// croft's directory, or a wildcard that already covers it.
	Certificates string
}

// CertDir is where the alias's certificate lives.
func (a Alias) CertDir() string {
	if a.Certificates != "" {
		return a.Certificates
	}
	return "/var/lib/croft/certificates/" + a.Domain
}

// PathRoute is one prefix of a domain, served from somewhere of its own.
type PathRoute struct {
	Prefix string
	Target string
	Port   int
	// Strip takes the prefix off before passing the request on, for a backend
	// that answers /tickets and knows nothing of being mounted under /api/.
	Strip bool
}

type RouteProps struct {
	Domain       string
	Target       string
	Port         int
	SSL          bool
	Certificates string
	Paths        []PathRoute
	Aliases      []Alias
	State        enums.ManagedState
	File         string
	Content      string
}

func NewRoute(props RouteProps) *Route {
	return &Route{
		Domain:       props.Domain,
		Target:       props.Target,
		Port:         props.Port,
		SSL:          props.SSL,
		Certificates: props.Certificates,
		Paths:        props.Paths,
		Aliases:      props.Aliases,
		State:        props.State,
		File:         props.File,
		Content:      props.Content,
	}
}

// WithPath is the route with prefix served from p, replacing whatever that
// prefix pointed at before.
func (r *Route) WithPath(p PathRoute) *Route {
	next := *r
	next.Paths = []PathRoute{}
	for _, existing := range r.Paths {
		if existing.Prefix != p.Prefix {
			next.Paths = append(next.Paths, existing)
		}
	}
	next.Paths = append(next.Paths, p)
	return &next
}

// WithoutPath is the route with prefix served like the rest of the domain.
func (r *Route) WithoutPath(prefix string) *Route {
	next := *r
	next.Paths = []PathRoute{}
	for _, existing := range r.Paths {
		if existing.Prefix != prefix {
			next.Paths = append(next.Paths, existing)
		}
	}
	return &next
}

// WithAlias is the route answering on one more name, or with that name's
// certificate changed.
func (r *Route) WithAlias(a Alias) *Route {
	next := *r
	next.Aliases = []Alias{}
	for _, existing := range r.Aliases {
		if existing.Domain != a.Domain {
			next.Aliases = append(next.Aliases, existing)
		}
	}
	next.Aliases = append(next.Aliases, a)
	return &next
}

// WithoutAlias is the route no longer answering on that name.
func (r *Route) WithoutAlias(domain string) *Route {
	next := *r
	next.Aliases = []Alias{}
	for _, existing := range r.Aliases {
		if existing.Domain != domain {
			next.Aliases = append(next.Aliases, existing)
		}
	}
	return &next
}

func (r *Route) Alias(domain string) (Alias, bool) {
	for _, a := range r.Aliases {
		if a.Domain == domain {
			return a, true
		}
	}
	return Alias{}, false
}

func (r *Route) Path(prefix string) (PathRoute, bool) {
	for _, p := range r.Paths {
		if p.Prefix == prefix {
			return p, true
		}
	}
	return PathRoute{}, false
}

// Editable reports whether we may rewrite this file without asking.
func (r *Route) Editable() bool {
	return r.State == enums.StateManaged
}

// CertDir is where the certificate for this domain lives. Ours by default;
// certbot's when it issued the certificate and we are only reading it.
func (r *Route) CertDir() string {
	if r.Certificates != "" {
		return r.Certificates
	}
	return "/var/lib/croft/certificates/" + r.Domain
}

// Answers is false when nothing accepts a connection where this route points.
// It is a fact about the world, not about configuration, so it is only known
// on the side that can open a socket.
func (r *Route) SetAnswers(answers bool) { r.answers = &answers }

func (r *Route) Answers() (bool, bool) {
	if r.answers == nil {
		return false, false
	}
	return *r.answers, true
}
