package agent

import (
	"context"
	"net/http"
	"net/url"
	"strconv"

	"github.com/Hyzokaaa/opencroft/internal/shared/plan"
)

// AppClient is the deploy half of the agent, seen from the unprivileged side.
// Everything here is a description of what is wanted; the agent is what turns
// that into commands.
type AppClient struct{ c *Client }

func (c *Client) Apps() *AppClient { return &AppClient{c: c} }

// Inspect is the first plan: fetch the code so there is something to look at.
type Inspect struct {
	Repo   string `json:"repo"`
	Branch string `json:"branch"`
	Path   string `json:"path"`
}

// Deploy is the second plan: what was found, as commands somebody can edit
// before any of it runs.
type Deploy struct {
	Inspect

	Install  []string `json:"install"`
	Build    []string `json:"build"`
	Start    string   `json:"start"`
	Runtime  string   `json:"runtime"`
	Port     int      `json:"port"`
	Packages []string `json:"packages"`
}

func appPath(name, suffix string) string {
	return "/apps/" + url.PathEscape(name) + suffix
}

func (a *AppClient) InspectPlan(ctx context.Context, name string, want Inspect) (plan.Plan, error) {
	var response PlanResponse
	err := a.c.call(ctx, http.MethodPost, appPath(name, "/inspect/plan"), want, &response)
	return response.Plan, err
}

func (a *AppClient) Inspect(ctx context.Context, name string, want Inspect) (DetectionDTO, error) {
	var detection DetectionDTO
	err := a.c.call(ctx, http.MethodPost, appPath(name, "/inspect"), want, &detection)
	return detection, err
}

func (a *AppClient) DeployPlan(ctx context.Context, name string, want Deploy) (plan.Plan, error) {
	var response PlanResponse
	err := a.c.call(ctx, http.MethodPost, appPath(name, "/deploy/plan"), want, &response)
	return response.Plan, err
}

func (a *AppClient) Deploy(ctx context.Context, name string, want Deploy, report func(int, string)) error {
	return a.c.streamed(ctx, http.MethodPost, appPath(name, "/deploy"), want, report)
}

func (a *AppClient) Find(ctx context.Context, name string) (AppDTO, error) {
	var app AppDTO
	err := a.c.call(ctx, http.MethodGet, appPath(name, ""), nil, &app)
	return app, err
}

func (a *AppClient) Logs(ctx context.Context, name string, lines int) (string, error) {
	var response LogsResponse
	err := a.c.call(ctx, http.MethodGet,
		appPath(name, "/logs")+"?lines="+strconv.Itoa(lines), nil, &response)
	return response.Lines, err
}
