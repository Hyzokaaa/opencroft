package agent

import (
	"context"
	"net/http"
	"net/url"

	"github.com/Hyzokaaa/opencroft/internal/shared/plan"
)

// DatabaseClient is the database side of the agent, seen from the
// unprivileged half. Everything here describes what is wanted; the agent is
// what turns that into commands, and the password never comes back across.
type DatabaseClient struct{ c *Client }

func (c *Client) Databases() *DatabaseClient { return &DatabaseClient{c: c} }

func databasePath(container, suffix string) string {
	return "/instances/" + url.PathEscape(container) + "/databases" + suffix
}

func (a *DatabaseClient) FindAll(ctx context.Context, container string) (DatabasesResponse, error) {
	var response DatabasesResponse
	err := a.c.call(ctx, http.MethodGet, databasePath(container, ""), nil, &response)
	return response, err
}

func (a *DatabaseClient) ProvisionPlan(
	ctx context.Context, container string, want DatabaseDTO,
) (plan.Plan, error) {
	var response PlanResponse
	err := a.c.call(ctx, http.MethodPost, databasePath(container, "/plan"), want, &response)
	return response.Plan, err
}

func (a *DatabaseClient) Provision(
	ctx context.Context, container string, want DatabaseDTO, report func(int, string),
) error {
	return a.c.streamed(ctx, http.MethodPost, databasePath(container, ""), want, report)
}

func (a *DatabaseClient) DestroyPlan(
	ctx context.Context, container, database string,
) (plan.Plan, string, error) {
	var response RollbackResponse
	err := a.c.call(ctx, http.MethodGet,
		databasePath(container, "/"+url.PathEscape(database)+"/destroy/plan"), nil, &response)
	return response.Plan, response.Warning, err
}

func (a *DatabaseClient) Destroy(
	ctx context.Context, container, database string, report func(int, string),
) error {
	return a.c.streamed(ctx, http.MethodDelete,
		databasePath(container, "/"+url.PathEscape(database)), nil, report)
}

// ── Connecting to one in another container ───────────────────────────────────

func (a *DatabaseClient) Shareable(ctx context.Context, container string) ([]OfferDTO, error) {
	var out []OfferDTO
	err := a.c.call(ctx, http.MethodGet, databasePath(container, "/shareable"), nil, &out)
	return out, err
}

func (a *DatabaseClient) SharePlan(ctx context.Context, container string, want ShareDTO) (plan.Plan, error) {
	var response PlanResponse
	err := a.c.call(ctx, http.MethodPost, databasePath(container, "/connect/plan"), want, &response)
	return response.Plan, err
}

func (a *DatabaseClient) Share(ctx context.Context, container string, want ShareDTO, report func(int, string)) error {
	return a.c.streamed(ctx, http.MethodPost, databasePath(container, "/connect"), want, report)
}

// ── Taking on a database found in the container ──────────────────────────────

func (a *DatabaseClient) Found(ctx context.Context, container string) ([]FoundDatabaseDTO, error) {
	var out []FoundDatabaseDTO
	err := a.c.call(ctx, http.MethodGet, databasePath(container, "/found"), nil, &out)
	return out, err
}

func (a *DatabaseClient) AdoptPlan(ctx context.Context, container string, want AdoptDatabaseDTO) (plan.Plan, error) {
	var response PlanResponse
	err := a.c.call(ctx, http.MethodPost, databasePath(container, "/adopt/plan"), want, &response)
	return response.Plan, err
}

func (a *DatabaseClient) Adopt(ctx context.Context, container string, want AdoptDatabaseDTO, report func(int, string)) error {
	return a.c.streamed(ctx, http.MethodPost, databasePath(container, "/adopt"), want, report)
}
