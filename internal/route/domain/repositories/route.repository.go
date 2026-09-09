package repositories

import (
	"context"

	"github.com/Hyzokaaa/opencroft/internal/route/domain/entities"
)

// RouteRepository reads and writes the proxy's own configuration files.
type RouteRepository interface {
	FindAll(ctx context.Context) ([]*entities.Route, error)
	FindByDomain(ctx context.Context, domain string) (*entities.Route, error)
	Write(ctx context.Context, route *entities.Route) error
	Remove(ctx context.Context, domain string) error
	Reload(ctx context.Context) error
}
