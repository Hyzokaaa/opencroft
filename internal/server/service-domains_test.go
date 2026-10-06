package server

import (
	"testing"

	routeEntities "github.com/Hyzokaaa/opencroft/internal/route/domain/entities"
	routeEnums "github.com/Hyzokaaa/opencroft/internal/route/domain/enums"
)

// A new domain for a service joins the route its domains already share — one
// croft manages, on https if there is one — and never a hand-written vhost.
func TestADomainJoinsTheRouteTheServiceAlreadyHas(t *testing.T) {
	route := func(domain string, port int, ssl bool, state routeEnums.ManagedState) *routeEntities.Route {
		return routeEntities.NewRoute(routeEntities.RouteProps{Domain: domain, Target: "10.0.0.2", Port: port, SSL: ssl, State: state})
	}
	routes := []*routeEntities.Route{
		route("hand.example.com", 3000, true, routeEnums.StateUnmanaged),
		route("plain.example.com", 3000, false, routeEnums.StateManaged),
		route("api.example.com", 8080, true, routeEnums.StateManaged),
		route("app.example.com", 3000, true, routeEnums.StateManaged),
	}
	if entry := entryRoute(routes, "10.0.0.2", 3000); entry == nil || entry.Domain != "app.example.com" {
		t.Errorf("joined %+v", entry)
	}
	if entry := entryRoute(routes, "10.0.0.2", 9000); entry != nil {
		t.Errorf("a service with no domain joined %s", entry.Domain)
	}
}

// An API reached as /api/ of another domain is said to be, so that a domain
// meant for the whole app is not given to its back half.
func TestAServiceBehindAPathIsSaidToBe(t *testing.T) {
	routes := []*routeEntities.Route{routeEntities.NewRoute(routeEntities.RouteProps{
		Domain: "app.example.com", Target: "10.0.0.2", Port: 80, State: routeEnums.StateManaged,
		Paths: []routeEntities.PathRoute{{Prefix: "/api/", Target: "10.0.0.2", Port: 3000, Strip: true}},
	})}
	if mount := mountedAt(routes, "10.0.0.2", 3000); mount != "app.example.com/api/" {
		t.Errorf("mounted at %q", mount)
	}
	if mount := mountedAt(routes, "10.0.0.2", 80); mount != "" {
		t.Errorf("the root is mounted at %q", mount)
	}
}
