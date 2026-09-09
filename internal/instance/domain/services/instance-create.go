package services

import (
	"context"
	"errors"
	"fmt"
	"regexp"

	"github.com/Hyzokaaa/opencroft/internal/instance/domain/entities"
	"github.com/Hyzokaaa/opencroft/internal/instance/domain/enums"
	"github.com/Hyzokaaa/opencroft/internal/instance/domain/repositories"
	"github.com/Hyzokaaa/opencroft/internal/shared/id"
)

var (
	ErrNameRequired  = errors.New("instance name is required")
	ErrNameInvalid   = errors.New("instance name may only contain letters, digits and dashes")
	ErrAlreadyExists = errors.New("an instance with that name already exists")
)

var namePattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9-]{0,62}$`)

type CreateInstanceProps struct {
	Name     string
	Image    string
	Port     int
	CPULimit int
	MemLimit string
}

type CreateInstance struct {
	idGenerator id.Generator
	instances   repositories.InstanceRepository
}

func NewCreateInstance(idGenerator id.Generator, instances repositories.InstanceRepository) *CreateInstance {
	return &CreateInstance{idGenerator: idGenerator, instances: instances}
}

func (s *CreateInstance) Execute(ctx context.Context, props CreateInstanceProps) (*entities.Instance, error) {
	if props.Name == "" {
		return nil, ErrNameRequired
	}
	if !namePattern.MatchString(props.Name) {
		return nil, ErrNameInvalid
	}

	existing, err := s.instances.FindByName(ctx, props.Name)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		return nil, ErrAlreadyExists
	}

	address, err := s.instances.AllocateAddress(ctx)
	if err != nil {
		return nil, fmt.Errorf("allocating an address: %w", err)
	}

	instance := entities.NewInstance(entities.InstanceProps{
		Id:       s.idGenerator.Create(),
		Name:     props.Name,
		Image:    defaulted(props.Image, s.instances.DefaultImage()),
		Address:  address,
		Port:     defaultedInt(props.Port, 80),
		CPULimit: defaultedInt(props.CPULimit, 4),
		MemLimit: defaulted(props.MemLimit, "4GB"),
		Status:   enums.StatusRunning,
		Managed:  true,
	})

	if err := s.instances.Create(ctx, instance); err != nil {
		return nil, err
	}
	return instance, nil
}

func defaulted(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}

func defaultedInt(value, fallback int) int {
	if value == 0 {
		return fallback
	}
	return value
}
