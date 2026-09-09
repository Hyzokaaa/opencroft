package commands

import (
	"context"

	"github.com/Hyzokaaa/opencroft/internal/instance/domain/services"
)

type DestroyInstanceProps struct {
	Name string
}

type DestroyInstanceCommand struct {
	destroyInstance *services.DestroyInstance
}

func NewDestroyInstanceCommand(destroyInstance *services.DestroyInstance) *DestroyInstanceCommand {
	return &DestroyInstanceCommand{destroyInstance: destroyInstance}
}

func (c *DestroyInstanceCommand) Execute(ctx context.Context, props DestroyInstanceProps) error {
	return c.destroyInstance.Execute(ctx, props.Name)
}
