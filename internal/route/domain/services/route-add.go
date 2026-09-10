package services

import (
	"context"
	"errors"
	"fmt"
	"regexp"

	instanceEntities "github.com/Hyzokaaa/opencroft/internal/instance/domain/entities"
	instanceRepositories "github.com/Hyzokaaa/opencroft/internal/instance/domain/repositories"
	"github.com/Hyzokaaa/opencroft/internal/route/domain/entities"
	"github.com/Hyzokaaa/opencroft/internal/route/domain/enums"
	"github.com/Hyzokaaa/opencroft/internal/route/domain/repositories"
	"github.com/Hyzokaaa/opencroft/internal/shared/plan"
)

var (
	ErrDomainRequired  = errors.New("a domain is required")
	ErrDomainInvalid   = errors.New("that does not look like a domain name")
	ErrDomainTaken     = errors.New("something already serves that domain")
	ErrTargetUnknown   = errors.New("no container by that name")
	ErrTargetNoAddress = errors.New("that container has no address yet")
)

// Deliberately loose: this rejects obvious mistakes, not unusual but valid
// names. A domain that resolves is the real test, and only DNS can run it.
var domainPattern = regexp.MustCompile(`^[a-zA-Z0-9]([a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?(\.[a-zA-Z0-9]([a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?)+$`)

type AddRouteProps struct {
	Domain string
	Target string // container name
	Port   int    // 0 means whatever the container says it listens on
}

type AddRoute struct {
	routes    repositories.RouteRepository
	instances instanceRepositories.InstanceRepository
}

func NewAddRoute(routes repositories.RouteRepository, instances instanceRepositories.InstanceRepository) *AddRoute {
	return &AddRoute{routes: routes, instances: instances}
}

// Prepare works out the route and how it would be written, without writing it.
func (s *AddRoute) Prepare(ctx context.Context, props AddRouteProps) (*entities.Route, plan.Plan, error) {
	if props.Domain == "" {
		return nil, plan.Plan{}, ErrDomainRequired
	}
	if !domainPattern.MatchString(props.Domain) {
		return nil, plan.Plan{}, ErrDomainInvalid
	}

	// Refuse to shadow a domain nginx already answers on, whoever wrote it.
	existing, err := s.routes.FindByDomain(ctx, props.Domain)
	if err != nil {
		return nil, plan.Plan{}, err
	}
	if existing != nil {
		return nil, plan.Plan{}, fmt.Errorf("%w: %s", ErrDomainTaken, existing.File)
	}

	target, err := s.target(ctx, props.Target)
	if err != nil {
		return nil, plan.Plan{}, err
	}

	port := props.Port
	if port == 0 {
		port = target.Port
	}
	if port == 0 {
		port = 80
	}

	route := entities.NewRoute(entities.RouteProps{
		Domain: props.Domain,
		Target: target.Address,
		Port:   port,
		State:  enums.StateManaged,
	})

	return route, s.routes.WritePlan(route), nil
}

func (s *AddRoute) Execute(ctx context.Context, props AddRouteProps) (*entities.Route, error) {
	route, _, err := s.Prepare(ctx, props)
	if err != nil {
		return nil, err
	}
	if err := s.routes.Write(ctx, route); err != nil {
		return nil, err
	}
	return route, nil
}

func (s *AddRoute) target(ctx context.Context, name string) (*instanceEntities.Instance, error) {
	found, err := s.instances.FindByName(ctx, name)
	if err != nil {
		return nil, err
	}
	if found == nil {
		return nil, fmt.Errorf("%w: %s", ErrTargetUnknown, name)
	}
	if found.Address == "" {
		return nil, fmt.Errorf("%w: %s", ErrTargetNoAddress, name)
	}
	return found, nil
}
