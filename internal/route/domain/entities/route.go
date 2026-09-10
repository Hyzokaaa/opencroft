package entities

import "github.com/Hyzokaaa/opencroft/internal/route/domain/enums"

// Route maps a domain to a container address and port.
type Route struct {
	Domain string
	Target string
	Port   int
	SSL    bool
	// Certificates overrides where the certificate is read from.
	Certificates string
	State        enums.ManagedState
	File         string
	Content      string
}

type RouteProps struct {
	Domain       string
	Target       string
	Port         int
	SSL          bool
	Certificates string
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
		State:        props.State,
		File:         props.File,
		Content:      props.Content,
	}
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
