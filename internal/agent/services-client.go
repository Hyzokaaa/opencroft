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

func (a *ServiceClient) Environment(ctx context.Context, container, service string) (EnvironmentDTO, error) {
	var found EnvironmentDTO
	err := a.c.call(ctx, http.MethodGet,
		servicePath(container, "/"+url.PathEscape(service)+"/env"), nil, &found)
	return found, err
}

func (a *ServiceClient) EnvironmentPlan(ctx context.Context, container, service string, change EnvChangeDTO) (plan.Plan, error) {
	var response PlanResponse
	err := a.c.call(ctx, http.MethodPost,
		servicePath(container, "/"+url.PathEscape(service)+"/env/plan"), change, &response)
	return response.Plan, err
}

func (a *ServiceClient) ChangeEnvironment(ctx context.Context, container, service string, change EnvChangeDTO, report func(int, string)) error {
	return a.c.streamed(ctx, http.MethodPost,
		servicePath(container, "/"+url.PathEscape(service)+"/env"), change, report)
}

func (a *ServiceClient) RedeployPlan(ctx context.Context, container, service string) (plan.Plan, error) {
	var response PlanResponse
	err := a.c.call(ctx, http.MethodGet,
		servicePath(container, "/"+url.PathEscape(service)+"/redeploy/plan"), nil, &response)
	return response.Plan, err
}

func (a *ServiceClient) Redeploy(ctx context.Context, container, service string, report func(int, string)) error {
	return a.c.streamed(ctx, http.MethodPost,
		servicePath(container, "/"+url.PathEscape(service)+"/redeploy"), nil, report)
}

func (a *ServiceClient) PowerPlan(ctx context.Context, container, service string, action PowerAction) (plan.Plan, error) {
	var response PlanResponse
	err := a.c.call(ctx, http.MethodGet,
		servicePath(container, "/"+url.PathEscape(service)+"/"+string(action)+"/plan"), nil, &response)
	return response.Plan, err
}

func (a *ServiceClient) Power(ctx context.Context, container, service string, action PowerAction, report func(int, string)) error {
	return a.c.streamed(ctx, http.MethodPost,
		servicePath(container, "/"+url.PathEscape(service)+"/"+string(action)), nil, report)
}

// Units are what the container runs that croft did not deploy. They are
// reached by their own name, which carries none of the guarantees a
// service's does, so they live under a path of their own.
func unitPath(container, unit, suffix string) string {
	return "/instances/" + url.PathEscape(container) + "/units/" + url.PathEscape(unit) + suffix
}

func (a *ServiceClient) UnitPowerPlan(ctx context.Context, container, unit string, action PowerAction) (plan.Plan, error) {
	var response PlanResponse
	err := a.c.call(ctx, http.MethodGet, unitPath(container, unit, "/"+string(action)+"/plan"), nil, &response)
	return response.Plan, err
}

func (a *ServiceClient) UnitPower(ctx context.Context, container, unit string, action PowerAction, report func(int, string)) error {
	return a.c.streamed(ctx, http.MethodPost, unitPath(container, unit, "/"+string(action)), nil, report)
}

func adoptPath(container string, kind Adoptable, key, suffix string) string {
	return "/instances/" + url.PathEscape(container) + "/" + string(kind) + "/" + url.PathEscape(key) + suffix
}

func (a *ServiceClient) AdoptionOf(ctx context.Context, container string, kind Adoptable, key string) (AdoptionDTO, error) {
	var found AdoptionDTO
	err := a.c.call(ctx, http.MethodGet, adoptPath(container, kind, key, "/adoption"), nil, &found)
	return found, err
}

func (a *ServiceClient) AdoptPlan(ctx context.Context, container string, kind Adoptable, key string, answer AdoptDTO) (plan.Plan, error) {
	var response PlanResponse
	err := a.c.call(ctx, http.MethodPost, adoptPath(container, kind, key, "/adopt/plan"), answer, &response)
	return response.Plan, err
}

func (a *ServiceClient) Adopt(ctx context.Context, container string, kind Adoptable, key string, answer AdoptDTO, report func(int, string)) error {
	return a.c.streamed(ctx, http.MethodPost, adoptPath(container, kind, key, "/adopt"), answer, report)
}

func (a *ServiceClient) UnitLogs(ctx context.Context, container, unit string, lines int) (string, error) {
	var response LogsResponse
	err := a.c.call(ctx, http.MethodGet,
		unitPath(container, unit, "/logs")+"?lines="+strconv.Itoa(lines), nil, &response)
	return response.Lines, err
}

// ── Properties, saved without deploying ──────────────────────────────────────

func (a *ServiceClient) ConfigurePlan(ctx context.Context, container string, want ServiceDTO) (plan.Plan, error) {
	var response PlanResponse
	err := a.c.call(ctx, http.MethodPut,
		servicePath(container, "/"+url.PathEscape(want.Name)+"/properties/plan"), want, &response)
	return response.Plan, err
}

func (a *ServiceClient) Configure(ctx context.Context, container string, want ServiceDTO, report func(int, string)) error {
	return a.c.streamed(ctx, http.MethodPut,
		servicePath(container, "/"+url.PathEscape(want.Name)+"/properties"), want, report)
}
