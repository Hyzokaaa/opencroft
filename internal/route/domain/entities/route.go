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
	Paths   []PathRoute
	State   enums.ManagedState
	File    string
	Content string

	// nil when nobody checked whether anything is listening.
	answers *bool
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
