package agent

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"regexp"
	"strings"
	"testing"

	deployServices "github.com/Hyzokaaa/opencroft/internal/deploy/domain/services"
	"github.com/Hyzokaaa/opencroft/internal/shared/host"
)

const envOnDisk = "# written by hand\nPORT=3000\nJWT_SECRET=abc123\n"

// aServiceWithAnEnvironment is a service croft deployed, whose .env is on disk.
func aServiceWithAnEnvironment(t *testing.T) (*Server, *host.Fake) {
	t.Helper()
	server, fake := testServer()
	ctx := context.Background()
	for key, value := range map[string]string{
		"services":               "backend",
		"service.backend.repo":   "https://github.com/user/app.git",
		"service.backend.branch": "main",
		"service.backend.path":   "/srv/backend",
		"service.backend.start":  "node dist/main",
	} {
		_ = server.instances.Annotate(ctx, "helpdesk", key, value)
	}
	fake.Responses["lxc exec helpdesk -- cat /srv/backend/.env"] = envOnDisk
	return server, fake
}

func TestTheEnvironmentIsReadFromItsFile(t *testing.T) {
	server, _ := aServiceWithAnEnvironment(t)

	var found EnvironmentDTO
	recorder := serve(server, http.MethodGet, "/instances/helpdesk/services/backend/env", nil)
	if err := json.Unmarshal(recorder.Body.Bytes(), &found); err != nil {
		t.Fatalf("%v: %s", err, recorder.Body.String())
	}

	if found.File != "/srv/backend/.env" || found.Hash != deployServices.EnvHash(envOnDisk) || found.Applies != "restart" {
		t.Errorf("read %+v", found)
	}
	if len(found.Vars) != 2 || found.Vars[1].Key != "JWT_SECRET" || found.Vars[1].Value != "abc123" {
		t.Errorf("vars %+v", found.Vars)
	}
}

// Somebody edited the file over ssh after the panel opened it. Writing now
// would put back what they just changed.
func TestAChangeToAFileThatChangedSinceIsRefused(t *testing.T) {
	server, _ := aServiceWithAnEnvironment(t)

	recorder := serve(server, http.MethodPost, "/instances/helpdesk/services/backend/env/plan",
		EnvChangeDTO{Hash: deployServices.EnvHash("PORT=3000\n"), Vars: map[string]string{"PORT": "4000"}})
	if recorder.Code != http.StatusConflict {
		t.Errorf("answered %d: %s", recorder.Code, recorder.Body.String())
	}
}

var written = regexp.MustCompile(`printf %s '([A-Za-z0-9+/=]*)' \| base64 -d > (\S+)`)

// Only the variable that changed is rewritten — the comment and the others stay
// — and the step that writes it is one the history does not keep.
func TestAChangeRewritesOnlyWhatWasAsked(t *testing.T) {
	server, _ := aServiceWithAnEnvironment(t)

	recorder := serve(server, http.MethodPost, "/instances/helpdesk/services/backend/env/plan",
		EnvChangeDTO{Hash: deployServices.EnvHash(envOnDisk), Vars: map[string]string{"PORT": "4000", "JWT_SECRET": "abc123"}})
	if recorder.Code != http.StatusOK {
		t.Fatalf("answered %d: %s", recorder.Code, recorder.Body.String())
	}
	var response PlanResponse
	_ = json.Unmarshal(recorder.Body.Bytes(), &response)

	var content, restart string
	for _, step := range response.Plan.Steps {
		if m := written.FindStringSubmatch(step.Shell()); m != nil {
			if !step.Secret {
				t.Error("the step that writes the environment would be kept in the history")
			}
			decoded, _ := base64.StdEncoding.DecodeString(m[1])
			content = string(decoded)
		}
		if strings.Contains(step.Shell(), "systemctl restart croft-backend") {
			restart = step.Shell()
		}
	}

	if content != "# written by hand\nPORT=4000\nJWT_SECRET=abc123\n" {
		t.Errorf("the file would become:\n%s", content)
	}
	if restart == "" {
		t.Error("nothing makes the change take effect")
	}
}

// Once a service exists its environment is its file. A deployment of it — from
// Properties, or a request that still carries variables — never writes one, so
// it cannot put back values changed since.
func TestDeployingAnExistingServiceLeavesItsEnvironmentAlone(t *testing.T) {
	server, _ := aServiceWithAnEnvironment(t)

	request := aService()
	request.Env = map[string]string{"JWT_SECRET": "old"}

	var response PlanResponse
	_ = json.Unmarshal(serve(server, http.MethodPost, "/instances/helpdesk/services/deploy/plan", request).Body.Bytes(), &response)
	for _, step := range response.Plan.Steps {
		if written.MatchString(step.Shell()) {
			t.Errorf("a deployment of an existing service writes its environment: %s", step.Describe)
		}
	}

	// A new one is given its first environment by its first deployment.
	request.Name = "worker"
	_ = json.Unmarshal(serve(server, http.MethodPost, "/instances/helpdesk/services/deploy/plan", request).Body.Bytes(), &response)
	wrote := false
	for _, step := range response.Plan.Steps {
		wrote = wrote || written.MatchString(step.Shell())
	}
	if !wrote {
		t.Error("a new service is deployed without its environment")
	}
}

// A unit that reads no environment file has nothing to edit, and saying so is
// better than inventing a file it would never read.
func TestAnAdoptedUnitWithoutAFileSaysSo(t *testing.T) {
	server, _ := testServer()
	ctx := context.Background()
	for key, value := range map[string]string{
		"services":                     "backend",
		"service.backend.repo":         "https://github.com/user/app.git",
		"service.backend.branch":       "main",
		"service.backend.path":         "/opt/app",
		"service.backend.adopted-unit": "app",
	} {
		_ = server.instances.Annotate(ctx, "helpdesk", key, value)
	}

	recorder := serve(server, http.MethodGet, "/instances/helpdesk/services/backend/env", nil)
	if recorder.Code == http.StatusOK || !strings.Contains(recorder.Body.String(), "EnvironmentFile") {
		t.Errorf("answered %d: %s", recorder.Code, recorder.Body.String())
	}
}
