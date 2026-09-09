package services

import (
	"context"
	"errors"

	"github.com/Hyzokaaa/opencroft/internal/instance/domain/repositories"
)

var ErrNotFound = errors.New("instance not found")

type DestroyInstance struct {
	instances repositories.InstanceRepository
}

func NewDestroyInstance(instances repositories.InstanceRepository) *DestroyInstance {
	return &DestroyInstance{instances: instances}
}

func (s *DestroyInstance) Execute(ctx context.Context, name string) error {
	existing, err := s.instances.FindByName(ctx, name)
	if err != nil {
		return err
	}
	if existing == nil {
		return ErrNotFound
	}
	return s.instances.Delete(ctx, name)
}
