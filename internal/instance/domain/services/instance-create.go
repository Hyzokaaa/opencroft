package services

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"time"

	"github.com/Hyzokaaa/opencroft/internal/instance/domain/entities"
	"github.com/Hyzokaaa/opencroft/internal/instance/domain/enums"
	"github.com/Hyzokaaa/opencroft/internal/instance/domain/repositories"
	"github.com/Hyzokaaa/opencroft/internal/shared/id"
	"github.com/Hyzokaaa/opencroft/internal/shared/plan"
)

var (
	ErrNameRequired  = errors.New("instance name is required")
	ErrNameInvalid   = errors.New("instance name may only contain letters, digits and dashes")
	ErrAlreadyExists = errors.New("an instance with that name already exists")
)

var namePattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9-]{0,62}$`)

// projectPattern is a project's name as the project module declares it. It is
// checked here too because it ends up on a command line.
var projectPattern = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,38}[a-z0-9])?$`)

var ErrProjectInvalid = errors.New("that is not a project name")

type CreateInstanceProps struct {
	Name     string
	Image    string
	Port     int
	CPULimit int
	MemLimit string
	Project  string
}

type CreateInstance struct {
	idGenerator id.Generator
	instances   repositories.InstanceRepository
}

func NewCreateInstance(idGenerator id.Generator, instances repositories.InstanceRepository) *CreateInstance {
	return &CreateInstance{idGenerator: idGenerator, instances: instances}
}

// Prepare validates and works out what would be created, without touching
// anything. Allocating an address is a read: it looks at what is already used.
func (s *CreateInstance) Prepare(ctx context.Context, props CreateInstanceProps) (*entities.Instance, plan.Plan, error) {
	if props.Name == "" {
		return nil, plan.Plan{}, ErrNameRequired
	}
	if !namePattern.MatchString(props.Name) {
		return nil, plan.Plan{}, ErrNameInvalid
	}

	if props.Project != "" && !projectPattern.MatchString(props.Project) {
		return nil, plan.Plan{}, ErrProjectInvalid
	}

	existing, err := s.instances.FindByName(ctx, props.Name)
	if err != nil {
		return nil, plan.Plan{}, err
	}
	if existing != nil {
		return nil, plan.Plan{}, ErrAlreadyExists
	}

	address, err := s.instances.AllocateAddress(ctx)
	if err != nil {
		return nil, plan.Plan{}, fmt.Errorf("allocating an address: %w", err)
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
		Created:  time.Now().UTC().Format("2006-01-02"),
		Managed:  true,
		Project:  props.Project,
	})

	return instance, s.instances.CreatePlan(instance), nil
}

func (s *CreateInstance) Execute(ctx context.Context, props CreateInstanceProps) (*entities.Instance, error) {
	instance, _, err := s.Prepare(ctx, props)
	if err != nil {
		return nil, err
	}

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
