package repositories

import (
	"context"

	"github.com/Hyzokaaa/opencroft/internal/instance/domain/entities"
	"github.com/Hyzokaaa/opencroft/internal/shared/plan"
)

// InstanceRepository talks to the container runtime, not to a database.
// The operating system is the source of truth.
type InstanceRepository interface {
	FindAll(ctx context.Context) ([]*entities.Instance, error)
	FindByName(ctx context.Context, name string) (*entities.Instance, error)
	// CreatePlan and DeletePlan return the steps that Create and Delete will
	// walk. Showing a plan and running it are the same list, so they cannot
	// disagree.
	CreatePlan(instance *entities.Instance) plan.Plan
	DeletePlan(name string) plan.Plan
	StartPlan(name string) plan.Plan
	StopPlan(name string) plan.Plan

	Create(ctx context.Context, instance *entities.Instance) error
	Delete(ctx context.Context, name string) error
	Start(ctx context.Context, name string) error
	Stop(ctx context.Context, name string) error
	Annotate(ctx context.Context, name, key, value string) error

	// Annotations returns the desired state stored on the resource, all of it
	// at once. Asked one key at a time, reading a container meant starting
	// twenty processes to answer one page.
	Annotations(ctx context.Context, name string) (map[string]string, error)

	// AllocateAddress returns a free address in the static range.
	AllocateAddress(ctx context.Context) (string, error)

	// DefaultImage differs between LXD and Incus.
	DefaultImage() string
}
