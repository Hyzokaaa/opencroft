package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func withBackend(t *testing.T, server *Server, extra map[string]string) {
	t.Helper()
	values := map[string]string{
		"services":                "backend",
		"service.backend.repo":    "https://github.com/user/app.git",
		"service.backend.branch":  "dev",
		"service.backend.path":    "/srv/backend",
		"service.backend.install": "npm ci",
		"service.backend.start":   "node dist/main",
		"service.backend.port":    "3000",
	}
	for key, value := range extra {
		values[key] = value
	}
	for key, value := range values {
		if err := server.instances.Annotate(context.Background(), "helpdesk", key, value); err != nil {
			t.Fatal(err)
		}
	}
}

func askPolicy(t *testing.T, server *Server, service string, enabled bool) *httptest.ResponseRecorder {
	t.Helper()
	encoded, _ := json.Marshal(SnapshotPolicyDTO{Enabled: enabled})
	recorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(recorder, httptest.NewRequest(http.MethodPut,
		"/instances/helpdesk/services/"+service+"/snapshot-policy/plan", bytes.NewReader(encoded)))
	return recorder
}

func shell(t *testing.T, recorder *httptest.ResponseRecorder) string {
	t.Helper()
	var response PlanResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	return response.Plan.Shell()
}

// Turning snapshots off or on is read before it is approved like every other
// write, and it is one command: the key, set or unset. Anything more would be
// the agent deciding things in the middle of a plan.
func TestTurningSnapshotsOffAndOnIsOneCommandEach(t *testing.T) {
	server, _ := testServer()
	withBackend(t, server, nil)

	off := askPolicy(t, server, "backend", false)
	if off.Code != http.StatusOK {
		t.Fatalf("answered %d: %s", off.Code, off.Body.String())
	}
	if got := shell(t, off); got != "lxc config set helpdesk user.croft.service.backend.snapshot off" {
		t.Errorf("turning it off runs:\n%s", got)
	}

	withBackend(t, server, map[string]string{"service.backend.snapshot": "off"})
	on := askPolicy(t, server, "backend", true)
	if on.Code != http.StatusOK {
		t.Fatalf("answered %d: %s", on.Code, on.Body.String())
	}
	if got := shell(t, on); got != "lxc config unset helpdesk user.croft.service.backend.snapshot" {
		t.Errorf("turning it back on runs:\n%s", got)
	}

	if again := askPolicy(t, server, "backend", false); again.Code != http.StatusBadRequest {
		t.Errorf("turning off what is already off was accepted with %d", again.Code)
	}
}

// The service name reaches a command line, and the agent cannot assume the
// panel is the one calling: a name that is not a service there is refused,
// and so is one shaped like a flag or a substitution.
func TestTheSnapshotPolicyIsOnlyForAServiceThatIsThere(t *testing.T) {
	server, fake := testServer()
	withBackend(t, server, nil)

	for _, name := range []string{"nothing", "-rf", "$(id)", "backend;reboot"} {
		if recorder := askPolicy(t, server, name, false); recorder.Code != http.StatusBadRequest &&
			recorder.Code != http.StatusNotFound {
			t.Errorf("%q was accepted with %d", name, recorder.Code)
		}
	}
	for _, command := range fake.Commands {
		if strings.Contains(command, "config set") {
			t.Errorf("something was written: %s", command)
		}
	}
}

// Whether snapshots are taken changes nothing that runs. If it counted towards
// what was deployed, turning it off would mark the service as having changes
// not deployed — and upgrading croft would mark every service so.
func TestTurningSnapshotsOffLeavesNothingPending(t *testing.T) {
	config := map[string]string{
		full("backend", "repo"):  "https://github.com/user/app.git",
		full("backend", "start"): "node dist/main",
	}
	config[full("backend", "deployed")] = Fingerprint(config, "backend")

	config[full("backend", snapshotKey)] = snapshotOff
	if pending(config, "backend") {
		t.Error("turning snapshots off shows as a change not deployed")
	}
}

// The way back is turned off through its own request, read and approved. A
// deployment that could carry it would switch it off in passing, inside a plan
// that was approved for something else.
func TestADeploymentCannotTurnTheWayBackOff(t *testing.T) {
	body := map[string]any{
		"name": "backend", "repo": "https://github.com/user/app.git", "branch": "main",
		"install": []string{"npm ci"}, "start": "npm start", "port": 3000,
		"noSnapshot": true,
	}
	steps := planOf(t, "/instances/helpdesk/services/deploy/plan", body).Plan.Steps
	if len(steps) == 0 || !strings.Contains(steps[0].Shell(), "snapshot") {
		t.Fatalf("the first step is %v", steps)
	}
}

// A service that chose no snapshots deploys without one, read from the
// container rather than from the request.
func TestAServiceWithoutSnapshotsRedeploysWithoutOne(t *testing.T) {
	server, fake := testServer()
	withBackend(t, server, map[string]string{"service.backend.snapshot": "off"})
	fake.Responses["lxc config get helpdesk user.croft.service.backend.snapshot"] = "off"

	recorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(recorder,
		httptest.NewRequest(http.MethodGet, "/instances/helpdesk/services/backend/redeploy/plan", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("answered %d: %s", recorder.Code, recorder.Body.String())
	}
	if body := shell(t, recorder); strings.Contains(body, "lxc snapshot") {
		t.Errorf("a snapshot is taken anyway:\n%s", body)
	}
}

// Known good names a snapshot to go back to. With none taken, naming this
// deployment's would point at something that does not exist, and the panel
// would offer a restore that fails.
func TestWithoutASnapshotNothingIsCalledKnownGood(t *testing.T) {
	server, fake := testServer()
	withBackend(t, server, map[string]string{
		"service.backend.snapshot": "off",
		"service.backend.healthy":  "croft-deploy-backend-20261001-120000",
	})
	fake.Responses["lxc config get helpdesk user.croft.service.backend.snapshot"] = "off"

	service := aService()
	service.Health = HealthDTO{Path: "/health"}
	encoded, _ := json.Marshal(service)
	recorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(recorder,
		httptest.NewRequest(http.MethodPost, "/instances/helpdesk/services/deploy", bytes.NewReader(encoded)))
	if strings.Contains(recorder.Body.String(), `"failed":true`) {
		t.Fatalf("the deployment failed: %s", recorder.Body.String())
	}

	config, _ := server.instances.Annotations(context.Background(), "helpdesk")
	if got := config[full("backend", "healthy")]; got != "croft-deploy-backend-20261001-120000" {
		t.Errorf("known good is now %q", got)
	}
}

// Without snapshots, a deployment that passed its check still leaves a way
// back: its commit. Known good used to stay at the last snapshot, weeks old,
// beside a deployment that had just worked.
func TestADeploymentThatWorkedIsKnownGoodByItsCommit(t *testing.T) {
	server, fake := testServer()
	withBackend(t, server, map[string]string{"service.backend.snapshot": "off"})
	fake.Responses["lxc config get helpdesk user.croft.service.backend.snapshot"] = "off"
	fake.Responses["lxc exec helpdesk -- git -c safe.directory=/srv/backend -C /srv/backend rev-parse HEAD"] = "6d9447d0c1e2f3a4b5c6d7e8f9a0b1c2d3e4f5a6"

	service := aService()
	service.Health = HealthDTO{Path: "/health"}
	encoded, _ := json.Marshal(service)
	recorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(recorder,
		httptest.NewRequest(http.MethodPost, "/instances/helpdesk/services/deploy", bytes.NewReader(encoded)))
	if strings.Contains(recorder.Body.String(), `"failed":true`) {
		t.Fatalf("the deployment failed: %s", recorder.Body.String())
	}

	config, _ := server.instances.Annotations(context.Background(), "helpdesk")
	if got := config[full("backend", "good")]; got != "6d9447d0c1e2f3a4b5c6d7e8f9a0b1c2d3e4f5a6" {
		t.Errorf("the commit that worked is recorded as %q", got)
	}
	if config[full("backend", "good-at")] == "" {
		t.Error("when it worked is not recorded")
	}
}

// A deployment with nothing to check proves nothing, so it names no commit
// as good either.
func TestADeploymentWithNoCheckIsNotKnownGood(t *testing.T) {
	server, fake := testServer()
	withBackend(t, server, map[string]string{"service.backend.snapshot": "off"})
	fake.Responses["lxc config get helpdesk user.croft.service.backend.snapshot"] = "off"
	fake.Responses["lxc exec helpdesk -- git -c safe.directory=/srv/backend -C /srv/backend rev-parse HEAD"] = "6d9447d0c1e2f3a4b5c6d7e8f9a0b1c2d3e4f5a6"

	encoded, _ := json.Marshal(aService())
	recorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(recorder,
		httptest.NewRequest(http.MethodPost, "/instances/helpdesk/services/deploy", bytes.NewReader(encoded)))

	config, _ := server.instances.Annotations(context.Background(), "helpdesk")
	if got := config[full("backend", "good")]; got != "" {
		t.Errorf("an unchecked deployment is called good: %q", got)
	}
}
