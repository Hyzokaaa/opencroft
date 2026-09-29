package services

import (
	"context"
	"errors"
	"fmt"
	"regexp"

	instanceRepositories "github.com/Hyzokaaa/opencroft/internal/instance/domain/repositories"
	"github.com/Hyzokaaa/opencroft/internal/route/domain/entities"
	"github.com/Hyzokaaa/opencroft/internal/route/domain/enums"
	"github.com/Hyzokaaa/opencroft/internal/route/domain/repositories"
	"github.com/Hyzokaaa/opencroft/internal/shared/plan"
)

var (
	ErrPrefixInvalid = errors.New("a path is a prefix like /api/: it starts and ends with a slash")
	ErrPathUnknown   = errors.New("that domain has no such path")
	ErrRouteAdopted  = errors.New("that vhost was edited by hand after croft wrote it, so croft no longer rewrites it")
)

// A prefix nginx matches as written. It ends in a slash so that /api/ does not
// also swallow /apiary; the root is the domain's own target, not a path. No
// segment is only dots: /api/../ is not a place, and nginx would never match it.
var prefixPattern = regexp.MustCompile(`^/([A-Za-z0-9_~-][A-Za-z0-9._~-]*/)+$`)

type SetPathProps struct {
	Domain string
	Prefix string
	Target string // container name
	Port   int    // 0 means whatever the container says it listens on
	Strip  bool
}

// RoutePaths sends a prefix of a domain somewhere other than the rest of it:
// the web at /, its API at /api/, in different places — or the same container
// on another port. Without it, a web and an API behind one domain would need
// a second nginx inside a container to split them, and a second place for
// every header a websocket needs.
type RoutePaths struct {
	routes    repositories.RouteRepository
	instances instanceRepositories.InstanceRepository
}

func NewRoutePaths(routes repositories.RouteRepository, instances instanceRepositories.InstanceRepository) *RoutePaths {
	return &RoutePaths{routes: routes, instances: instances}
}

// ours is a route croft may rewrite: one it wrote and nobody has edited since.
func (s *RoutePaths) ours(ctx context.Context, domain string) (*entities.Route, error) {
	existing, err := s.routes.FindByDomain(ctx, domain)
	if err != nil {
		return nil, err
	}
	switch {
	case existing == nil:
		return nil, fmt.Errorf("%w: %s", ErrRouteUnknown, domain)
	case existing.State == enums.StateUnmanaged:
		return nil, fmt.Errorf("%w: %s — take it over first", ErrRouteForeign, existing.File)
	case existing.State == enums.StateAdopted:
		return nil, fmt.Errorf("%w: %s", ErrRouteAdopted, existing.File)
	}
	return existing, nil
}

// PrepareSet adds a path, or points an existing one somewhere else.
func (s *RoutePaths) PrepareSet(ctx context.Context, props SetPathProps) (*entities.Route, plan.Plan, error) {
	if !prefixPattern.MatchString(props.Prefix) {
		return nil, plan.Plan{}, ErrPrefixInvalid
	}
	existing, err := s.ours(ctx, props.Domain)
	if err != nil {
		return nil, plan.Plan{}, err
	}

	target, err := containerNamed(ctx, s.instances, props.Target)
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
	if port < 1 || port > 65535 {
		return nil, plan.Plan{}, errors.New("the port must be between 1 and 65535")
	}

	wanted := entities.PathRoute{Prefix: props.Prefix, Target: target.Address, Port: port, Strip: props.Strip}
	if current, ok := existing.Path(props.Prefix); ok && current == wanted {
		return nil, plan.Plan{}, errors.New("nothing would change")
	}

	route := existing.WithPath(wanted)
	route.State = enums.StateManaged
	return route, s.routes.WritePlan(route), nil
}

// PrepareRemove sends a path back to wherever the rest of the domain goes.
func (s *RoutePaths) PrepareRemove(ctx context.Context, domain, prefix string) (*entities.Route, plan.Plan, error) {
	existing, err := s.ours(ctx, domain)
	if err != nil {
		return nil, plan.Plan{}, err
	}
	if _, ok := existing.Path(prefix); !ok {
		return nil, plan.Plan{}, fmt.Errorf("%w: %s%s", ErrPathUnknown, domain, prefix)
	}

	route := existing.WithoutPath(prefix)
	route.State = enums.StateManaged
	return route, s.routes.WritePlan(route), nil
}
