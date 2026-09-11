package agent

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func aService() ServiceDTO {
	return ServiceDTO{
		Name: "backend", Repo: "https://github.com/user/app.git", Branch: "main",
		Install: []string{"npm ci"}, Build: []string{"npm run build"},
		Start: "npm start", Runtime: "node", Port: 3000,
		Packages: []string{"nodejs", "npm"},
	}
}

func planOf(t *testing.T, path string, body any) PlanResponse {
	t.Helper()

	recorder := post(t, path, body)
	if recorder.Code != http.StatusOK {
		t.Fatalf("%s was refused with %d: %s", path, recorder.Code, recorder.Body.String())
	}

	var response PlanResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	return response
}

func refused(t *testing.T, body ServiceDTO) *strings.Reader {
	t.Helper()

	recorder := post(t, "/instances/helpdesk/services/deploy/plan", body)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("accepted with %d: %s", recorder.Code, recorder.Body.String())
	}
	return strings.NewReader(recorder.Body.String())
}

// Everything the deployment does is on the container, but the agent runs as
// root on the host and a repository URL is an argument to git.
func TestTheAgentRefusesDangerousServiceFields(t *testing.T) {
	cases := map[string]func(*ServiceDTO){
		"a repo that is a flag":        func(s *ServiceDTO) { s.Repo = "--upload-pack=id" },
		"a repo that is a local path":  func(s *ServiceDTO) { s.Repo = "/etc/passwd" },
		"a repo with a separator":      func(s *ServiceDTO) { s.Repo = "https://x/a.git; id" },
		"a branch that is a flag":      func(s *ServiceDTO) { s.Branch = "--upload-pack=id" },
		"a branch with a substitution": func(s *ServiceDTO) { s.Branch = "main$(id)" },
		"a path that is relative":      func(s *ServiceDTO) { s.Path = "../../etc" },
		"a path with a space":          func(s *ServiceDTO) { s.Path = "/srv/a b" },
		"a package that is a flag":     func(s *ServiceDTO) { s.Packages = []string{"--allow-downgrades"} },
		"a package with a separator":   func(s *ServiceDTO) { s.Packages = []string{"nodejs;id"} },
		"a commit that is not one":     func(s *ServiceDTO) { s.Commit = "HEAD~1; id" },
		"a name with a slash":          func(s *ServiceDTO) { s.Name = "a/b" },
		"a name that is a flag":        func(s *ServiceDTO) { s.Name = "--force" },
		"no name at all":               func(s *ServiceDTO) { s.Name = "" },
	}

	for name, break_ := range cases {
		t.Run(name, func(t *testing.T) {
			body := aService()
			break_(&body)
			refused(t, body)
		})
	}
}

// The environment file is written with a heredoc and read by a shell. A value
// spanning lines, or carrying the marker, would end the file early and leave
// the rest to be run.
func TestTheEnvironmentCannotEscapeItsFile(t *testing.T) {
	cases := map[string]map[string]string{
		"a value spanning lines":   {"TOKEN": "a\nrm -rf /"},
		"a value carrying the end": {"TOKEN": "x\nCROFT_ENV\nid"},
		"a key that is not one":    {"TOKEN=x; id": "y"},
		"a key with a space":       {"MY TOKEN": "y"},
	}

	for name, env := range cases {
		t.Run(name, func(t *testing.T) {
			body := aService()
			body.Env = env
			refused(t, body)
		})
	}
}

// The expected text goes inside a shell case pattern.
func TestTheHealthCheckCannotCarryShellCharacters(t *testing.T) {
	for _, health := range []HealthDTO{
		{Path: "/health", Contains: "ok\"; id #"},
		{Path: "/health", Contains: "$(id)"},
		{Path: "/health; id", Contains: "ok"},
		{Path: "health", Contains: "ok"},
		{Path: "/health", Status: 9000},
	} {
		body := aService()
		body.Health = health
		refused(t, body)
	}
}

// Asking a port nobody said to listen on would wait thirty seconds and then
// blame the application.
func TestAHealthCheckNeedsAPort(t *testing.T) {
	body := aService()
	body.Port = 0
	body.Health = HealthDTO{Path: "/health"}

	refused(t, body)
}

// Deploy keys are not built yet, and an ssh URL would hang on a passphrase
// prompt somewhere nobody can see it.
func TestAPrivateRepositoryIsRefusedWithAReason(t *testing.T) {
	for _, url := range []string{"git@github.com:user/app.git", "ssh://git@example.com/app.git"} {
		body := aService()
		body.Repo = url

		recorder := post(t, "/instances/helpdesk/services/deploy/plan", body)
		if recorder.Code != http.StatusBadRequest {
			t.Fatalf("%s was accepted with %d", url, recorder.Code)
		}
		if !strings.Contains(recorder.Body.String(), "deploy key") {
			t.Errorf("the refusal does not say why: %s", recorder.Body.String())
		}
	}
}

// The unit file is written with a heredoc, so a newline in a command would
// produce a broken unit and a confusing failure much later.
func TestACommandHasToBeOneLine(t *testing.T) {
	body := aService()
	body.Start = "npm start\nExecStart=/bin/sh"

	refused(t, body)
}

// Without a start command there is nothing to deploy, and a unit with an empty
// ExecStart fails at the very end of a long plan.
func TestDeployingWithNothingToStartIsRefused(t *testing.T) {
	body := aService()
	body.Start = "   "

	recorder := post(t, "/instances/helpdesk/services/deploy/plan", body)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("accepted with %d", recorder.Code)
	}
	if !strings.Contains(recorder.Body.String(), "start") {
		t.Errorf("the refusal does not say what is missing: %s", recorder.Body.String())
	}
}

// The two plans are one list, built from the same code. If they were built
// separately, approving the first would stop meaning anything about the
// second.
func TestTheInspectPlanIsTheStartOfTheDeployPlan(t *testing.T) {
	body := aService()
	inspect := planOf(t, "/instances/helpdesk/services/inspect/plan", body).Plan
	deploy := planOf(t, "/instances/helpdesk/services/deploy/plan", body).Plan

	if len(inspect.Steps) == 0 || len(deploy.Steps) <= len(inspect.Steps) {
		t.Fatalf("inspect has %d steps, deploy has %d", len(inspect.Steps), len(deploy.Steps))
	}

	for i, step := range inspect.Steps {
		theirs := deploy.Steps[i].Shell()

		// Which runtime to install is exactly what inspecting is for, so the
		// package list is the one thing allowed to grow between them.
		if strings.Contains(step.Shell(), "apt-get install") {
			if !strings.HasPrefix(theirs, step.Shell()) {
				t.Errorf("the deployment installs something else entirely:\n  %s\n  %s",
					step.Shell(), theirs)
			}
			continue
		}
		if step.Shell() != theirs {
			t.Errorf("step %d differs:\n  inspect: %s\n  deploy:  %s", i+1, step.Shell(), theirs)
		}
	}
}

// Looking at a repository builds nothing and starts nothing. That is what
// makes it reasonable to approve before knowing what the deployment will be.
func TestLookingAtARepositoryOnlyFetchesIt(t *testing.T) {
	body := aService()
	body.Env = map[string]string{"JWT_SECRET": "s3cret"}
	body.Health = HealthDTO{Path: "/health"}

	steps := planOf(t, "/instances/helpdesk/services/inspect/plan", body).Plan.Steps

	text := ""
	for _, step := range steps {
		text += step.Shell() + "\n"
	}

	for _, forbidden := range []string{"systemctl", "npm ci", "npm run build", "JWT_SECRET", "curl -fsS"} {
		if strings.Contains(text, forbidden) {
			t.Errorf("inspecting would have run %q:\n%s", forbidden, text)
		}
	}
	if !strings.Contains(text, "snapshot") {
		t.Error("no snapshot, so even fetching would not be reversible")
	}
}

// A deployment that breaks the service is recoverable. One that breaks the
// database inside the container is not, unless there is a point to go back to.
func TestEveryDeploymentStartsWithASnapshot(t *testing.T) {
	steps := planOf(t, "/instances/helpdesk/services/deploy/plan", aService()).Plan.Steps

	if len(steps) == 0 || !strings.Contains(steps[0].Shell(), "snapshot") {
		t.Fatalf("the first step is %v", steps)
	}
}

// The container name reaches a command line, so it is checked here as
// everywhere else — the agent cannot assume the panel is the one calling.
func TestTheContainerNameIsCheckedOnTheDeployRoutes(t *testing.T) {
	for _, path := range []string{
		"/instances/--force/services/deploy/plan",
		"/instances/a;id/services/inspect/plan",
	} {
		if code := post(t, path, aService()).Code; code == http.StatusOK {
			t.Errorf("%s was accepted", path)
		}
	}
}

// Restoring reaches every service in the container, so the snapshot has to be
// one that is really there — not a name somebody passed in.
func TestRollingBackToASnapshotThatIsNotThereIsRefused(t *testing.T) {
	for _, snapshot := range []string{"", "../../etc", "a snapshot; id", "not-taken-by-us"} {
		recorder := post(t, "/instances/helpdesk/services/rollback/plan",
			map[string]string{"snapshot": snapshot})

		if recorder.Code == http.StatusOK {
			t.Errorf("%q was accepted", snapshot)
		}
	}
}
