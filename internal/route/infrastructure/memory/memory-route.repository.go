package memory

import (
	"context"
	"sync"

	"github.com/Hyzokaaa/opencroft/internal/route/domain/entities"
	"github.com/Hyzokaaa/opencroft/internal/route/domain/enums"
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
