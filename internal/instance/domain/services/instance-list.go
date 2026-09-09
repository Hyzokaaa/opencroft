package services

import (
	"context"
	"sort"

	"github.com/Hyzokaaa/opencroft/internal/instance/domain/entities"
	"github.com/Hyzokaaa/opencroft/internal/instance/domain/repositories"
)

type ListInstances struct {
	instances repositories.InstanceRepository
}

func NewListInstances(instances repositories.InstanceRepository) *ListInstances {
	return &ListInstances{instances: instances}
}

func (s *ListInstances) Execute(ctx context.Context) ([]*entities.Instance, error) {
	found, err := s.instances.FindAll(ctx)
	if err != nil {
		return nil, err
	}

	sort.Slice(found, func(a, b int) bool { return found[a].Name < found[b].Name })
	return found, nil
}
