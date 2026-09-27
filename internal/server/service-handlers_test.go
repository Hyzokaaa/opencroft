package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Hyzokaaa/opencroft/internal/agent"
	"github.com/Hyzokaaa/opencroft/internal/shared/job"
	"github.com/Hyzokaaa/opencroft/internal/shared/plan"
)

// fakeDeployer stands where the agent would, and remembers what it was asked.
type fakeDeployer struct {
	mu    sync.Mutex
	asked []string
}

func (f *fakeDeployer) record(format string, args ...any) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.asked = append(f.asked, fmt.Sprintf(format, args...))
}

func (f *fakeDeployer) was(call string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, asked := range f.asked {
		if asked == call {
			return true
		}
	}
	return false
}

func onePlan() plan.Plan { return plan.New(plan.Command("Do it", "lxc", "exec", "x")) }

func (f *fakeDeployer) FindAll(context.Context, string) (agent.ServicesResponse, error) {
	return agent.ServicesResponse{}, nil
}
func (f *fakeDeployer) InspectPlan(context.Context, string, agent.ServiceDTO) (plan.Plan, error) {
	return onePlan(), nil
}
func (f *fakeDeployer) Inspect(context.Context, string, agent.ServiceDTO) (agent.DetectionDTO, error) {
	return agent.DetectionDTO{}, nil
}
func (f *fakeDeployer) DeployPlan(context.Context, string, agent.ServiceDTO) (plan.Plan, error) {
	return onePlan(), nil
}
func (f *fakeDeployer) Deploy(context.Context, string, agent.ServiceDTO, func(int, string)) error {
	return nil
}
func (f *fakeDeployer) RollbackPlan(context.Context, string, string) (plan.Plan, string, error) {
	return onePlan(), "", nil
}
func (f *fakeDeployer) Rollback(context.Context, string, string, func(int, string)) error { return nil }
func (f *fakeDeployer) Logs(context.Context, string, string, int) (string, error)         { return "", nil }
func (f *fakeDeployer) DestroyPlan(context.Context, string, string) (plan.Plan, string, error) {
	return onePlan(), "", nil
}
func (f *fakeDeployer) Destroy(context.Context, string, string, func(int, string)) error { return nil }

func (f *fakeDeployer) PowerPlan(_ context.Context, container, service string, action agent.PowerAction) (plan.Plan, error) {
	f.record("plan %s service %s/%s", action, container, service)
	return onePlan(), nil
}
func (f *fakeDeployer) Power(_ context.Context, container, service string, action agent.PowerAction, _ func(int, string)) error {
	f.record("run %s service %s/%s", action, container, service)
	return nil
}
func (f *fakeDeployer) UnitPowerPlan(_ context.Context, container, unit string, action agent.PowerAction) (plan.Plan, error) {
	f.record("plan %s unit %s/%s", action, container, unit)
	return onePlan(), nil
}
func (f *fakeDeployer) UnitPower(_ context.Context, container, unit string, action agent.PowerAction, _ func(int, string)) error {
	f.record("run %s unit %s/%s", action, container, unit)
	return nil
}
func (f *fakeDeployer) UnitLogs(_ context.Context, container, unit string, _ int) (string, error) {
	f.record("logs unit %s/%s", container, unit)
	return "started", nil
}
func (f *fakeDeployer) AdoptionOf(_ context.Context, container string, kind agent.Adoptable, key string) (agent.AdoptionDTO, error) {
	f.record("inspect %s %s/%s", kind, container, key)
	return agent.AdoptionDTO{}, nil
}
func (f *fakeDeployer) AdoptPlan(_ context.Context, container string, kind agent.Adoptable, key string, answer agent.AdoptDTO) (plan.Plan, error) {
	f.record("plan adopt %s %s/%s as %s", kind, container, key, answer.Name)
	return onePlan(), nil
}
func (f *fakeDeployer) Adopt(_ context.Context, container string, kind agent.Adoptable, key string, answer agent.AdoptDTO, _ func(int, string)) error {
	f.record("run adopt %s %s/%s as %s", kind, container, key, answer.Name)
	return nil
}

func testAPI() (http.Handler, *fakeDeployer, *job.Runner) {
	fake := &fakeDeployer{}
	next := 0
	jobs := job.NewRunner(func() string { next++; return fmt.Sprint(next) })
	return api(Deps{Services: fake, Jobs: jobs}), fake, jobs
}

func request(t *testing.T, handler http.Handler, method, path string) *httptest.ResponseRecorder {
	t.Helper()
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(method, path, nil))
	return recorder
}

// A button in the panel that nothing on the other side answers is worse than
// no button. Restart once shipped exactly like that: the agent served it, the
// panel never forwarded it, and every test passed because each half was only
// ever tested on its own.
func TestEveryPowerActionReachesTheAgent(t *testing.T) {
	cases := []struct{ kind, path string }{
		{"service", "/api/hosts/local/instances/helpdesk/services/backend/"},
		{"unit", "/api/hosts/local/instances/helpdesk/units/openhelpdesk-backend/"},
	}

	for _, action := range agent.PowerActions {
		for _, c := range cases {
			t.Run(string(action)+" "+c.kind, func(t *testing.T) {
				handler, fake, jobs := testAPI()
				subject := "backend"
				if c.kind == "unit" {
					subject = "openhelpdesk-backend"
				}

				planned := request(t, handler, http.MethodPost, c.path+string(action)+"?plan=1")
				if planned.Code != http.StatusOK {
					t.Fatalf("the plan was answered with %d: %s", planned.Code, planned.Body.String())
				}
				if !fake.was(fmt.Sprintf("plan %s %s helpdesk/%s", action, c.kind, subject)) {
					t.Fatalf("the agent was never asked for the plan: %v", fake.asked)
				}

				started := request(t, handler, http.MethodPost, c.path+string(action))
				if started.Code != http.StatusAccepted {
					t.Fatalf("running it was answered with %d: %s", started.Code, started.Body.String())
				}
				var body struct {
					JobId string `json:"jobId"`
				}
				if err := json.Unmarshal(started.Body.Bytes(), &body); err != nil {
					t.Fatal(err)
				}

				finished(t, jobs, body.JobId)
				if !fake.was(fmt.Sprintf("run %s %s helpdesk/%s", action, c.kind, subject)) {
					t.Fatalf("the job never asked the agent to do it: %v", fake.asked)
				}
			})
		}
	}
}

// Adopting is read, then planned, then run — each step a request of its own,
// and each one has to reach the agent, which is the only half that can look.
// A unit and a site go the same way, under paths of their own.
func TestAdoptingReachesTheAgent(t *testing.T) {
	for kind, key := range map[agent.Adoptable]string{
		agent.AdoptUnit: "openhelpdesk-backend",
		agent.AdoptSite: "dev.openhelpdesk.dev",
	} {
		t.Run(string(kind), func(t *testing.T) {
			handler, fake, jobs := testAPI()
			base := "/api/hosts/local/instances/helpdesk/" + string(kind) + "/" + key + "/"

			if code := request(t, handler, http.MethodGet, base+"adoption").Code; code != http.StatusOK {
				t.Fatalf("inspecting was answered with %d", code)
			}

			post := func(path string) *httptest.ResponseRecorder {
				recorder := httptest.NewRecorder()
				handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, path,
					strings.NewReader(`{"name":"web","build":["npm run build"]}`)))
				return recorder
			}

			if code := post(base + "adopt?plan=1").Code; code != http.StatusOK {
				t.Fatalf("planning was answered with %d", code)
			}
			started := post(base + "adopt")
			if started.Code != http.StatusAccepted {
				t.Fatalf("running was answered with %d: %s", started.Code, started.Body.String())
			}
			var body struct {
				JobId string `json:"jobId"`
			}
			_ = json.Unmarshal(started.Body.Bytes(), &body)
			finished(t, jobs, body.JobId)

			for _, call := range []string{
				fmt.Sprintf("inspect %s helpdesk/%s", kind, key),
				fmt.Sprintf("plan adopt %s helpdesk/%s as web", kind, key),
				fmt.Sprintf("run adopt %s helpdesk/%s as web", kind, key),
			} {
				if !fake.was(call) {
					t.Errorf("never asked: %s (asked %v)", call, fake.asked)
				}
			}
		})
	}
}

func TestTheLogsOfAFoundUnitReachTheAgent(t *testing.T) {
	handler, fake, _ := testAPI()

	recorder := request(t, handler, http.MethodGet,
		"/api/hosts/local/instances/helpdesk/units/openhelpdesk-backend/logs?lines=50")
	if recorder.Code != http.StatusOK {
		t.Fatalf("answered with %d: %s", recorder.Code, recorder.Body.String())
	}
	if !fake.was("logs unit helpdesk/openhelpdesk-backend") {
		t.Fatalf("the agent was never asked: %v", fake.asked)
	}
}

// Stopping takes down whatever the process serves, and the one place to say so
// is the plan screen, before anybody agrees to it.
func TestStoppingSaysWhatGoesDown(t *testing.T) {
	handler, _, _ := testAPI()

	recorder := request(t, handler, http.MethodPost,
		"/api/hosts/local/instances/helpdesk/services/backend/stop?plan=1")

	var body struct {
		Summary string `json:"summary"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(body.Summary, "goes down") {
		t.Errorf("the summary does not warn: %q", body.Summary)
	}
}

func finished(t *testing.T, jobs *job.Runner, id string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if snapshot, ok := jobs.Find(id); ok && snapshot.Status != job.StatusRunning {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("job %s never finished", id)
}
