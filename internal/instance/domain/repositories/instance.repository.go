package repositories

import (
	"context"

	"github.com/Hyzokaaa/opencroft/internal/instance/domain/entities"
)

// InstanceRepository talks to the container runtime, not to a database.
// The operating system is the source of truth.
type InstanceRepository interface {
	FindAll(ctx context.Context) ([]*entities.Instance, error)
	FindByName(ctx context.Context, name string) (*entities.Instance, error)
	Create(ctx context.Context, instance *entities.Instance) error
	Delete(ctx context.Context, name string) error
	Start(ctx context.Context, name string) error
	Stop(ctx context.Context, name string) error
	Annotate(ctx context.Context, name, key, value string) error

	// AllocateAddress returns a free address in the static range.
	AllocateAddress(ctx context.Context) (string, error)

	// DefaultImage differs between LXD and Incus.
	DefaultImage() string
}
