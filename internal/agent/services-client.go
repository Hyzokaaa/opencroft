package agent

import (
	"context"
	"net/http"
	"net/url"
	"strconv"

	"github.com/Hyzokaaa/opencroft/internal/shared/plan"
)

// ServiceClient is the deploy side of the agent, seen from the unprivileged
// half. Everything here is a description of what is wanted; the agent is what
// turns that into commands.
type ServiceClient struct{ c *Client }

func (c *Client) Services() *ServiceClient { return &ServiceClient{c: c} }

func servicePath(container, suffix string) string {
	return "/instances/" + url.PathEscape(container) + "/services" + suffix
}

func (a *ServiceClient) FindAll(ctx context.Context, container string) (ServicesResponse, error) {
	var response ServicesResponse
	err := a.c.call(ctx, http.MethodGet, servicePath(container, ""), nil, &response)
	return response, err
}

func (a *ServiceClient) InspectPlan(ctx context.Context, container string, want ServiceDTO) (plan.Plan, error) {
	var response PlanResponse
	err := a.c.call(ctx, http.MethodPost, servicePath(container, "/inspect/plan"), want, &response)
	return response.Plan, err
}

func (a *ServiceClient) Inspect(ctx context.Context, container string, want ServiceDTO) (DetectionDTO, error) {
	var detection DetectionDTO
	err := a.c.call(ctx, http.MethodPost, servicePath(container, "/inspect"), want, &detection)
	return detection, err
}

func (a *ServiceClient) DeployPlan(ctx context.Context, container string, want ServiceDTO) (plan.Plan, error) {
	var response PlanResponse
	err := a.c.call(ctx, http.MethodPost, servicePath(container, "/deploy/plan"), want, &response)
	return response.Plan, err
}

func (a *ServiceClient) Deploy(ctx context.Context, container string, want ServiceDTO, report func(int, string)) error {
	return a.c.streamed(ctx, http.MethodPost, servicePath(container, "/deploy"), want, report)
}

func (a *ServiceClient) RollbackPlan(ctx context.Context, container, snapshot string) (plan.Plan, string, error) {
	var response RollbackResponse
	err := a.c.call(ctx, http.MethodPost, servicePath(container, "/rollback/plan"),
		rollbackRequest{Snapshot: snapshot}, &response)
	return response.Plan, response.Warning, err
}

func (a *ServiceClient) Rollback(ctx context.Context, container, snapshot string, report func(int, string)) error {
	return a.c.streamed(ctx, http.MethodPost, servicePath(container, "/rollback"),
		rollbackRequest{Snapshot: snapshot}, report)
}

func (a *ServiceClient) Logs(ctx context.Context, container, service string, lines int) (string, error) {
	var response LogsResponse
	err := a.c.call(ctx, http.MethodGet,
		servicePath(container, "/"+url.PathEscape(service)+"/logs")+"?lines="+strconv.Itoa(lines),
		nil, &response)
	return response.Lines, err
}

func (a *ServiceClient) DestroyPlan(ctx context.Context, container, service string) (plan.Plan, string, error) {
	var response RollbackResponse
	err := a.c.call(ctx, http.MethodGet,
		servicePath(container, "/"+url.PathEscape(service)+"/destroy/plan"), nil, &response)
	return response.Plan, response.Warning, err
}

func (a *ServiceClient) Destroy(ctx context.Context, container, service string, report func(int, string)) error {
	return a.c.streamed(ctx, http.MethodDelete,
		servicePath(container, "/"+url.PathEscape(service)), nil, report)
}
