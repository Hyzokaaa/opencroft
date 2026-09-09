package entities

import "github.com/Hyzokaaa/opencroft/internal/route/domain/enums"

// Route maps a domain to a container address and port.
type Route struct {
	Domain  string
	Target  string
	Port    int
	SSL     bool
	State   enums.ManagedState
	File    string
	Content string
}

type RouteProps struct {
	Domain  string
	Target  string
	Port    int
	SSL     bool
	State   enums.ManagedState
	File    string
	Content string
}

func NewRoute(props RouteProps) *Route {
	return &Route{
		Domain:  props.Domain,
		Target:  props.Target,
		Port:    props.Port,
		SSL:     props.SSL,
		State:   props.State,
		File:    props.File,
		Content: props.Content,
	}
}

// Editable reports whether we may rewrite this file without asking.
func (r *Route) Editable() bool {
	return r.State == enums.StateManaged
}
