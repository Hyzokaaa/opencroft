package services

import (
	"context"
	"sort"

	"github.com/Hyzokaaa/opencroft/internal/route/domain/entities"
	"github.com/Hyzokaaa/opencroft/internal/route/domain/repositories"
)

type ListRoutes struct {
	routes repositories.RouteRepository
}

func NewListRoutes(routes repositories.RouteRepository) *ListRoutes {
	return &ListRoutes{routes: routes}
}

func (s *ListRoutes) Execute(ctx context.Context) ([]*entities.Route, error) {
	found, err := s.routes.FindAll(ctx)
	if err != nil {
		return nil, err
	}
	sort.Slice(found, func(a, b int) bool { return found[a].Domain < found[b].Domain })
	return found, nil
}
