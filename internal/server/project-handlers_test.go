package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Hyzokaaa/opencroft/internal/agent"
	"github.com/Hyzokaaa/opencroft/internal/shared/job"
	"github.com/Hyzokaaa/opencroft/internal/shared/plan"
)

type fakeProjector struct{ fakeDeployer }

func (f *fakeProjector) List(context.Context) ([]agent.ProjectDTO, error) {
	f.record("list")
	return []agent.ProjectDTO{{Name: "open-helpdesk", Declared: true, Instances: []string{"web"}}}, nil
}
func (f *fakeProjector) DeclarePlan(_ context.Context, name, description string) (plan.Plan, error) {
	f.record("plan declare %s: %s", name, description)
	return onePlan(), nil
}
func (f *fakeProjector) Declare(_ context.Context, name, description string, _ func(int, string)) error {
	f.record("run declare %s: %s", name, description)
	return nil
}
func (f *fakeProjector) RemovePlan(_ context.Context, name string) (plan.Plan, error) {
	f.record("plan remove %s", name)
	return onePlan(), nil
}
func (f *fakeProjector) Remove(_ context.Context, name string, _ func(int, string)) error {
	f.record("run remove %s", name)
	return nil
}
func (f *fakeProjector) AssignPlan(_ context.Context, instance, project string) (plan.Plan, error) {
	f.record("plan assign %s to %q", instance, project)
	return onePlan(), nil
}
func (f *fakeProjector) Assign(_ context.Context, instance, project string, _ func(int, string)) error {
	f.record("run assign %s to %q", instance, project)
	return nil
}

func send(handler http.Handler, method, path string, body any) *httptest.ResponseRecorder {
	encoded, _ := json.Marshal(body)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(method, path, bytes.NewReader(encoded)))
	return recorder
}

// Every project button reaches the agent, for its plan and for running it —
// the panel only forwards, and the agent decides.
func TestEveryProjectActionReachesTheAgent(t *testing.T) {
	cases := []struct {
		method, path string
		body         any
		plan, run    string
	}{
		{http.MethodPost, "/api/hosts/local/projects", agent.ProjectDTO{Name: "billing", Description: "Invoices"},
			"plan declare billing: Invoices", "run declare billing: Invoices"},
		{http.MethodDelete, "/api/hosts/local/projects/billing", nil,
			"plan remove billing", "run remove billing"},
		{http.MethodPut, "/api/hosts/local/instances/web/project", agent.AssignmentDTO{Project: "billing"},
			`plan assign web to "billing"`, `run assign web to "billing"`},
		{http.MethodPut, "/api/hosts/local/instances/web/project", agent.AssignmentDTO{},
			`plan assign web to ""`, `run assign web to ""`},
	}

	for _, c := range cases {
		t.Run(c.plan, func(t *testing.T) {
			fake := &fakeProjector{}
			next := 0
			jobs := job.NewRunner(func() string { next++; return fmt.Sprint(next) }, nil)
			handler := api(Deps{Projects: fake, Jobs: jobs})

			if planned := send(handler, c.method, c.path+"?plan=1", c.body); planned.Code != http.StatusOK {
				t.Fatalf("the plan was answered with %d: %s", planned.Code, planned.Body.String())
			}
			if !fake.was(c.plan) {
				t.Fatalf("the agent was never asked for the plan: %v", fake.asked)
			}

			started := send(handler, c.method, c.path, c.body)
			if started.Code != http.StatusAccepted {
				t.Fatalf("running it was answered with %d: %s", started.Code, started.Body.String())
			}
			var body struct {
				JobId string `json:"jobId"`
			}
			_ = json.Unmarshal(started.Body.Bytes(), &body)
			finished(t, jobs, body.JobId)
			if !fake.was(c.run) {
				t.Errorf("the agent was never asked to do it: %v", fake.asked)
			}
		})
	}

	handler := api(Deps{Projects: &fakeProjector{}})
	if listed := send(handler, http.MethodGet, "/api/hosts/local/projects", nil); listed.Code != http.StatusOK {
		t.Errorf("listing answered %d", listed.Code)
	}
}
