package repositories

import (
	"context"

	"github.com/Hyzokaaa/opencroft/internal/route/domain/entities"
	"github.com/Hyzokaaa/opencroft/internal/shared/plan"
)

// RouteRepository reads and writes the proxy's own configuration files.
type RouteRepository interface {
	FindAll(ctx context.Context) ([]*entities.Route, error)
	FindByDomain(ctx context.Context, domain string) (*entities.Route, error)

	// WritePlan and RemovePlan describe what the write will do. Write and
	// Remove walk those same steps.
	WritePlan(route *entities.Route) plan.Plan
	RemovePlan(domain string) plan.Plan

	Write(ctx context.Context, route *entities.Route) error
	Remove(ctx context.Context, domain string) error
	Reload(ctx context.Context) error
}
