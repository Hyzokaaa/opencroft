// Package services decides what a project is on this host and what changing
// one does. Nothing here runs anything: it answers with plans.
package services

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	instanceRepositories "github.com/Hyzokaaa/opencroft/internal/instance/domain/repositories"
	"github.com/Hyzokaaa/opencroft/internal/project/domain/entities"
	"github.com/Hyzokaaa/opencroft/internal/project/domain/repositories"
	"github.com/Hyzokaaa/opencroft/internal/shared/plan"
)

var (
	ErrProjectUnknown  = errors.New("no project by that name")
	ErrProjectNotEmpty = errors.New("that project still has containers")
	ErrInstanceUnknown = errors.New("no container by that name")
	ErrUndeclared      = errors.New("that project is not declared yet — declare it first")
	ErrNothingToChange = errors.New("nothing would change")
)

type Projects struct {
	projects  repositories.ProjectRepository
	instances instanceRepositories.InstanceRepository
}

func NewProjects(projects repositories.ProjectRepository, instances instanceRepositories.InstanceRepository) *Projects {
	return &Projects{projects: projects, instances: instances}
}

// List is every project on the host: the ones declared, and the ones only a
// container's label names. Both are real — a declaration with no containers
// is a project waiting for its first one; a label with no declaration is a
// project that arrived with a container.
func (s *Projects) List(ctx context.Context) ([]entities.Project, error) {
	declared, err := s.projects.FindAll(ctx)
	if err != nil {
		return nil, err
	}
	instances, err := s.instances.FindAll(ctx)
	if err != nil {
		return nil, err
	}

	byName := map[string]*entities.Project{}
	for _, d := range declared {
		byName[d.Name] = &entities.Project{Name: d.Name, Description: d.Description, Declared: true}
	}
	for _, instance := range instances {
		if instance.Project == "" {
			continue
		}
		project, ok := byName[instance.Project]
		if !ok {
			project = &entities.Project{Name: instance.Project}
			byName[instance.Project] = project
		}
		project.Instances = append(project.Instances, instance.Name)
	}

	out := make([]entities.Project, 0, len(byName))
	for _, project := range byName {
		sort.Strings(project.Instances)
		out = append(out, *project)
	}
	sort.Slice(out, func(a, b int) bool { return out[a].Name < out[b].Name })
	return out, nil
}

func (s *Projects) find(ctx context.Context, name string) (*entities.Project, error) {
	all, err := s.List(ctx)
	if err != nil {
		return nil, err
	}
	for i := range all {
		if all[i].Name == name {
			return &all[i], nil
		}
	}
	return nil, nil
}

// PrepareDeclare creates a project, declares one that only a label named, or
// changes a declared one's description.
func (s *Projects) PrepareDeclare(ctx context.Context, declaration entities.Declaration) (plan.Plan, error) {
	declaration.Description = strings.TrimSpace(declaration.Description)
	if err := declaration.Validate(); err != nil {
		return plan.Plan{}, err
	}
	existing, err := s.find(ctx, declaration.Name)
	if err != nil {
		return plan.Plan{}, err
	}
	if existing != nil && existing.Declared && existing.Description == declaration.Description {
		return plan.Plan{}, ErrNothingToChange
	}
	return s.projects.DeclarePlan(declaration), nil
}

// PrepareRemove forgets an empty project. One with containers is refused
// rather than leaving their labels naming nothing: moving them out first is
// the decision that has to be made on purpose.
func (s *Projects) PrepareRemove(ctx context.Context, name string) (plan.Plan, error) {
	existing, err := s.find(ctx, name)
	if err != nil {
		return plan.Plan{}, err
	}
	switch {
	case existing == nil || !existing.Declared:
		return plan.Plan{}, fmt.Errorf("%w: %s", ErrProjectUnknown, name)
	case len(existing.Instances) > 0:
		return plan.Plan{}, fmt.Errorf("%w: move %s out of it first", ErrProjectNotEmpty,
			strings.Join(existing.Instances, ", "))
	}
	return s.projects.RemovePlan(name), nil
}

// PrepareAssign puts a container in a project, or in none when project is
// empty. Only into a declared one: a typo would otherwise make a project.
func (s *Projects) PrepareAssign(ctx context.Context, instance, project string) (plan.Plan, error) {
	found, err := s.instances.FindByName(ctx, instance)
	if err != nil {
		return plan.Plan{}, err
	}
	if found == nil {
		return plan.Plan{}, fmt.Errorf("%w: %s", ErrInstanceUnknown, instance)
	}
	if found.Project == project {
		return plan.Plan{}, ErrNothingToChange
	}
	if project != "" {
		target, err := s.find(ctx, project)
		if err != nil {
			return plan.Plan{}, err
		}
		if target == nil || !target.Declared {
			return plan.Plan{}, fmt.Errorf("%w: %s", ErrUndeclared, project)
		}
	}
	return s.projects.AssignPlan(instance, project), nil
}
