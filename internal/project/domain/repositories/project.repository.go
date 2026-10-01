package repositories

import (
	"context"

	"github.com/Hyzokaaa/opencroft/internal/project/domain/entities"
	"github.com/Hyzokaaa/opencroft/internal/shared/plan"
)

// ProjectRepository keeps declarations and writes the label on containers.
// Every write is a plan first, so what is shown is what runs.
type ProjectRepository interface {
	FindAll(ctx context.Context) ([]entities.Declaration, error)
	DeclarePlan(declaration entities.Declaration) plan.Plan
	RemovePlan(name string) plan.Plan
	// AssignPlan labels a container with a project, or takes the label off
	// when project is empty.
	AssignPlan(instance, project string) plan.Plan
}
