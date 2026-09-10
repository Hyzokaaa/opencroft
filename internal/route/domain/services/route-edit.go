package services

import (
	"context"
	"errors"
	"fmt"

	instanceRepositories "github.com/Hyzokaaa/opencroft/internal/instance/domain/repositories"
	"github.com/Hyzokaaa/opencroft/internal/route/domain/entities"
	"github.com/Hyzokaaa/opencroft/internal/route/domain/enums"
	"github.com/Hyzokaaa/opencroft/internal/route/domain/repositories"
	"github.com/Hyzokaaa/opencroft/internal/shared/plan"
)

var (
	ErrRouteUnknown = errors.New("no route for that domain")
	ErrRouteForeign = errors.New("that vhost was not created here, so croft will not rewrite it")
)

type EditRouteProps struct {
	Domain string
	Target string // container name; empty keeps the current one
	Port   int    // zero keeps the current one
}

type EditRoute struct {
	routes    repositories.RouteRepository
	instances instanceRepositories.InstanceRepository
}

func NewEditRoute(routes repositories.RouteRepository, instances instanceRepositories.InstanceRepository) *EditRoute {
	return &EditRoute{routes: routes, instances: instances}
}

// Prepare keeps everything the route already had except what is being
// changed. TLS in particular survives an edit: a domain that was on https
// stays on https, pointed somewhere else.
func (s *EditRoute) Prepare(ctx context.Context, props EditRouteProps) (*entities.Route, plan.Plan, error) {
	existing, err := s.routes.FindByDomain(ctx, props.Domain)
	if err != nil {
		return nil, plan.Plan{}, err
	}
	if existing == nil {
		return nil, plan.Plan{}, fmt.Errorf("%w: %s", ErrRouteUnknown, props.Domain)
	}

	// Writing our own file for a domain served from somebody else's vhost
	// would leave nginx with two blocks claiming the same name.
	if existing.State == enums.StateUnmanaged {
		return nil, plan.Plan{}, fmt.Errorf("%w: %s", ErrRouteForeign, existing.File)
	}

	target := existing.Target
	port := existing.Port

	if props.Target != "" {
		found, err := s.instances.FindByName(ctx, props.Target)
		if err != nil {
			return nil, plan.Plan{}, err
		}
		if found == nil {
			return nil, plan.Plan{}, fmt.Errorf("%w: %s", ErrTargetUnknown, props.Target)
		}
		if found.Address == "" {
			return nil, plan.Plan{}, fmt.Errorf("%w: %s", ErrTargetNoAddress, props.Target)
		}
		target = found.Address
	}

	if props.Port != 0 {
		if props.Port < 1 || props.Port > 65535 {
			return nil, plan.Plan{}, errors.New("the port must be between 1 and 65535")
		}
		port = props.Port
	}

	if target == existing.Target && port == existing.Port {
		return nil, plan.Plan{}, errors.New("nothing would change")
	}

	route := entities.NewRoute(entities.RouteProps{
		Domain: props.Domain,
		Target: target,
		Port:   port,
		SSL:    existing.SSL,
		State:  enums.StateManaged,
	})

	return route, s.routes.WritePlan(route), nil
}
