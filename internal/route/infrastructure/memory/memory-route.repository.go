package memory

import (
	"context"
	"sync"

	"github.com/Hyzokaaa/opencroft/internal/route/domain/entities"
	"github.com/Hyzokaaa/opencroft/internal/route/domain/enums"
	"github.com/Hyzokaaa/opencroft/internal/shared/plan"
)

// MemoryRouteRepository backs the demo mode and the tests.
type MemoryRouteRepository struct {
	mu     sync.RWMutex
	routes map[string]*entities.Route
}

func NewMemoryRouteRepository() *MemoryRouteRepository {
	return &MemoryRouteRepository{routes: map[string]*entities.Route{}}
}

func NewDemoRouteRepository() *MemoryRouteRepository {
	r := NewMemoryRouteRepository()

	seed := []entities.RouteProps{
		{Domain: "soporte.example.com", Target: "10.146.38.200", Port: 3000, SSL: true,
			State: enums.StateManaged, File: "/etc/nginx/croft.d/soporte.example.com.conf"},
		{Domain: "www.example.com", Target: "10.146.38.201", Port: 80, SSL: true,
			State: enums.StateManaged, File: "/etc/nginx/croft.d/www.example.com.conf"},
		// Edited by hand after generation: shown, never overwritten.
		{Domain: "staging.example.com", Target: "10.146.38.202", Port: 8080, SSL: false,
			State: enums.StateAdopted, File: "/etc/nginx/croft.d/staging.example.com.conf"},
		// Written by someone else entirely.
		{Domain: "old.example.com", Target: "10.146.38.51", Port: 80, SSL: false,
			State: enums.StateUnmanaged, File: "/etc/nginx/croft.d/old.example.com.conf"},
	}

	for _, props := range seed {
		route := entities.NewRoute(props)
		r.routes[route.Domain] = route
	}
	return r
}

func (r *MemoryRouteRepository) FindAll(_ context.Context) ([]*entities.Route, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	all := make([]*entities.Route, 0, len(r.routes))
	for _, route := range r.routes {
		all = append(all, route)
	}
	return all, nil
}

func (r *MemoryRouteRepository) FindByDomain(_ context.Context, domain string) (*entities.Route, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if found, ok := r.routes[domain]; ok {
		return found, nil
	}
	return nil, nil
}

func (r *MemoryRouteRepository) Write(_ context.Context, route *entities.Route) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.routes[route.Domain] = route
	return nil
}

func (r *MemoryRouteRepository) Remove(_ context.Context, domain string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	delete(r.routes, domain)
	return nil
}

func (r *MemoryRouteRepository) Reload(_ context.Context) error { return nil }

// The demo plan says what the real driver would run, so the plan screen can be
// shown without nginx present.
func (r *MemoryRouteRepository) WritePlan(route *entities.Route) plan.Plan {
	path := "/etc/nginx/croft.d/" + route.Domain + ".conf"

	return plan.New(
		plan.WriteFile("Write the vhost, with a hash of its own contents", path,
			"server {\n    listen 80;\n    server_name "+route.Domain+";\n    …\n}\n"),
		plan.Command("Check nginx accepts it", "nginx", "-t"),
		plan.Command("Reload nginx", "systemctl", "reload", "nginx"),
	)
}

func (r *MemoryRouteRepository) RemovePlan(domain string) plan.Plan {
	return plan.New(
		plan.Command("Remove the vhost", "rm", "-f", "/etc/nginx/croft.d/"+domain+".conf"),
		plan.Command("Check nginx accepts it", "nginx", "-t"),
		plan.Command("Reload nginx", "systemctl", "reload", "nginx"),
	)
}
