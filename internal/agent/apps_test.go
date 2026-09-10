package agent

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func validDeploy() deployRequest {
	return deployRequest{
		inspectRequest: inspectRequest{
			Repo:   "https://github.com/user/app.git",
			Branch: "main",
			Path:   "/srv/app",
		},
		Install:  []string{"npm ci"},
		Build:    []string{"npm run build"},
		Start:    "npm start",
		Runtime:  "node",
		Port:     3000,
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

// Everything the deployment does is on the container. The agent runs as root
// on the host, and a repository URL is an argument to git.
func TestTheAgentRefusesDangerousSources(t *testing.T) {
	cases := map[string]func(*deployRequest){
		"a repo that is a flag":        func(d *deployRequest) { d.Repo = "--upload-pack=id" },
		"a repo that is a local path":  func(d *deployRequest) { d.Repo = "/etc/passwd" },
		"a repo with a separator":      func(d *deployRequest) { d.Repo = "https://x/a.git; id" },
		"a branch that is a flag":      func(d *deployRequest) { d.Branch = "--upload-pack=id" },
		"a branch with a substitution": func(d *deployRequest) { d.Branch = "main$(id)" },
		"a path that is relative":      func(d *deployRequest) { d.Path = "../../etc" },
		"a path with a space":          func(d *deployRequest) { d.Path = "/srv/a b" },
		"a package that is a flag":     func(d *deployRequest) { d.Packages = []string{"--allow-downgrades"} },
		"a package with a separator":   func(d *deployRequest) { d.Packages = []string{"nodejs;id"} },
	}

	for name, break_ := range cases {
		t.Run(name, func(t *testing.T) {
			body := validDeploy()
			break_(&body)

			if code := post(t, "/apps/helpdesk/deploy/plan", body).Code; code != http.StatusBadRequest {
				t.Errorf("accepted with %d", code)
			}
		})
	}
}

// Deploy keys are not built yet, and an ssh URL would hang on a passphrase
// prompt somewhere nobody can see it. Saying so is better than trying.
func TestAPrivateRepositoryIsRefusedWithAReason(t *testing.T) {
	for _, url := range []string{"git@github.com:user/app.git", "ssh://git@example.com/app.git"} {
		body := validDeploy()
		body.Repo = url

		recorder := post(t, "/apps/helpdesk/deploy/plan", body)
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
	for _, command := range []string{"npm start\nExecStart=/bin/sh", "npm start\r\nid"} {
		body := validDeploy()
		body.Start = command

		if code := post(t, "/apps/helpdesk/deploy/plan", body).Code; code != http.StatusBadRequest {
			t.Errorf("a command spanning lines was accepted with %d", code)
		}
	}
}

// Without a start command there is nothing to deploy, and a unit with an empty
// ExecStart fails at the very end of a long plan.
func TestDeployingWithNothingToStartIsRefused(t *testing.T) {
	body := validDeploy()
	body.Start = "   "

	recorder := post(t, "/apps/helpdesk/deploy/plan", body)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("accepted with %d", recorder.Code)
	}
	if !strings.Contains(recorder.Body.String(), "start") {
		t.Errorf("the refusal does not say what is missing: %s", recorder.Body.String())
	}
}

// The two plans are one list: the first is the opening of the second, built
// from the same code. If they were built separately, approving the first would
// stop meaning anything about the second.
//
// The one thing that legitimately grows between them is the package list —
// which runtime to install is exactly what inspecting is for.
func TestTheInspectPlanIsTheStartOfTheDeployPlan(t *testing.T) {
	body := validDeploy()
	inspect := planOf(t, "/apps/helpdesk/inspect/plan", body.inspectRequest).Plan
	deploy := planOf(t, "/apps/helpdesk/deploy/plan", body).Plan

	if len(inspect.Steps) == 0 || len(deploy.Steps) <= len(inspect.Steps) {
		t.Fatalf("inspect has %d steps, deploy has %d", len(inspect.Steps), len(deploy.Steps))
	}

	for i, step := range inspect.Steps {
		theirs := deploy.Steps[i].Shell()

		if strings.Contains(step.Shell(), "apt-get install") {
			if !strings.HasPrefix(theirs, step.Shell()) {
				t.Errorf("the deployment installs something else entirely:\n  inspect: %s\n  deploy:  %s",
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
	steps := planOf(t, "/apps/helpdesk/inspect/plan", validDeploy().inspectRequest).Plan.Steps

	text := ""
	for _, step := range steps {
		text += step.Shell() + "\n"
	}

	for _, forbidden := range []string{"systemctl", "npm ci", "npm run build"} {
		if strings.Contains(text, forbidden) {
			t.Errorf("inspecting would have run %q:\n%s", forbidden, text)
		}
	}
	if !strings.Contains(text, "snapshot") {
		t.Error("no snapshot, so even fetching would not be reversible")
	}
}

// A deployment that breaks the application is recoverable. One that breaks the
// database inside the container is not, unless there is a point to go back to.
func TestEveryDeploymentStartsWithASnapshot(t *testing.T) {
	steps := planOf(t, "/apps/helpdesk/deploy/plan", validDeploy()).Plan.Steps

	if len(steps) == 0 || !strings.Contains(steps[0].Shell(), "snapshot") {
		t.Fatalf("the first step is %v", steps)
	}
}

// The container name reaches a command line, so it is checked here as
// everywhere else — the agent cannot assume the panel is the one calling.
func TestTheContainerNameIsCheckedOnTheDeployRoutes(t *testing.T) {
	for _, path := range []string{"/apps/--force/deploy/plan", "/apps/a;id/inspect/plan"} {
		if code := post(t, path, validDeploy()).Code; code == http.StatusOK {
			t.Errorf("%s was accepted", path)
		}
	}
}
