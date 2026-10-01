package agent

import (
	"context"
	"net/http"
	"net/url"

	"github.com/Hyzokaaa/opencroft/internal/shared/plan"
)

// ProjectClient is the project side of the agent, seen from the panel.
type ProjectClient struct{ c *Client }

func (c *Client) Projects() *ProjectClient { return &ProjectClient{c: c} }

func projectPath(name, suffix string) string {
	return "/projects/" + url.PathEscape(name) + suffix
}

func assignmentPath(instance, suffix string) string {
	return "/instances/" + url.PathEscape(instance) + "/project" + suffix
}

func (a *ProjectClient) List(ctx context.Context) ([]ProjectDTO, error) {
	var out []ProjectDTO
	err := a.c.call(ctx, http.MethodGet, "/projects", nil, &out)
	return out, err
}

func (a *ProjectClient) DeclarePlan(ctx context.Context, name, description string) (plan.Plan, error) {
	var response PlanResponse
	err := a.c.call(ctx, http.MethodPut, projectPath(name, "/plan"), ProjectDTO{Description: description}, &response)
	return response.Plan, err
}

func (a *ProjectClient) Declare(ctx context.Context, name, description string, report func(int, string)) error {
	return a.c.streamed(ctx, http.MethodPut, projectPath(name, ""), ProjectDTO{Description: description}, report)
}

func (a *ProjectClient) RemovePlan(ctx context.Context, name string) (plan.Plan, error) {
	var response PlanResponse
	err := a.c.call(ctx, http.MethodGet, projectPath(name, "/remove/plan"), nil, &response)
	return response.Plan, err
}

func (a *ProjectClient) Remove(ctx context.Context, name string, report func(int, string)) error {
	return a.c.streamed(ctx, http.MethodDelete, projectPath(name, ""), nil, report)
}

func (a *ProjectClient) AssignPlan(ctx context.Context, instance, project string) (plan.Plan, error) {
	var response PlanResponse
	err := a.c.call(ctx, http.MethodPut, assignmentPath(instance, "/plan"), AssignmentDTO{Project: project}, &response)
	return response.Plan, err
}

func (a *ProjectClient) Assign(ctx context.Context, instance, project string, report func(int, string)) error {
	return a.c.streamed(ctx, http.MethodPut, assignmentPath(instance, ""), AssignmentDTO{Project: project}, report)
}
