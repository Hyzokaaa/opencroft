package container

import (
	"strings"
	"testing"
	"time"

	"github.com/Hyzokaaa/opencroft/internal/app/domain/entities"
)

func nodeApp() *entities.App {
	return entities.NewApp(entities.AppProps{
		Source:   entities.Source{Repo: "https://github.com/user/app.git", Branch: "main"},
		Install:  []string{"npm ci"},
		Build:    []string{"npm run build"},
		Start:    "npm start",
		Runtime:  "node",
		Port:     3000,
		Packages: []string{"nodejs", "npm"},
	})
}

func shells(steps []string) string { return strings.Join(steps, "\n") }

func planText(t *testing.T, app *entities.App) string {
	t.Helper()

	planner := NewPlanner("lxc", "helpdesk")
	lines := []string{}
	for _, step := range planner.Deploy(app, time.Unix(0, 0)).Steps {
		lines = append(lines, step.Shell())
	}
	return shells(lines)
}

// A deployment that breaks the application is recoverable. One that breaks the
// database inside the container is not, unless there is a point to go back to.
func TestTheSnapshotIsTakenBeforeAnythingElse(t *testing.T) {
	planner := NewPlanner("lxc", "helpdesk")
	steps := planner.Deploy(nodeApp(), time.Unix(0, 0)).Steps

	if len(steps) == 0 {
		t.Fatal("empty plan")
	}
	if !strings.Contains(steps[0].Shell(), "snapshot") {
		t.Fatalf("the first step is %q", steps[0].Shell())
	}
}

// Every step either acts on the container from outside, or runs inside it.
// Nothing may touch the host by accident — that is somebody else's machine,
// and the whole point of a container is that we stay in it.
func TestTheCommandsRunInsideTheContainer(t *testing.T) {
	planner := NewPlanner("lxc", "helpdesk")

	for _, step := range planner.Deploy(nodeApp(), time.Unix(0, 0)).Steps {
		if len(step.Argv) < 3 || step.Argv[0] != "lxc" {
			t.Errorf("%q does not go through the runtime: %v", step.Describe, step.Argv)
			continue
		}

		switch step.Argv[1] {
		// The only things allowed from outside are the ones that can only be
		// done from outside.
		case "snapshot", "restore":
		case "exec":
			if step.Argv[2] != "helpdesk" {
				t.Errorf("%q runs in %q", step.Describe, step.Argv[2])
			}
		default:
			t.Errorf("%q uses %q, which is neither", step.Describe, step.Argv[1])
		}
	}
}

// One command that works whether or not the code is already there. Two plans,
// one for the first deployment and one for the rest, is two things to keep
// correct.
func TestFetchingWorksTheFirstTimeAndEveryTimeAfter(t *testing.T) {
	text := planText(t, nodeApp())

	if !strings.Contains(text, "git clone") {
		t.Error("no first deployment")
	}
	if !strings.Contains(text, "reset --hard FETCH_HEAD") {
		t.Error("no way to update an existing checkout")
	}
}

func TestTheStepsHappenInAnOrderThatCanWork(t *testing.T) {
	lines := strings.Split(planText(t, nodeApp()), "\n")

	at := func(fragment string) int {
		for i, line := range lines {
			if strings.Contains(line, fragment) {
				return i
			}
		}
		return -1
	}

	order := []string{"snapshot", "apt-get install", "git clone", "npm ci", "npm run build", "systemctl enable"}
	previous := -1

	for _, fragment := range order {
		found := at(fragment)
		if found < 0 {
			t.Fatalf("%q never happens", fragment)
		}
		if found < previous {
			t.Errorf("%q happens too early", fragment)
		}
		previous = found
	}
}

// git is needed to fetch anything at all, whatever the runtime says.
func TestGitIsAlwaysInstalled(t *testing.T) {
	app := nodeApp()
	app.Packages = nil

	if !strings.Contains(planText(t, app), "install -y -qq git") {
		t.Error("git is not installed")
	}
}

// systemd restarts the application, not us watching it.
func TestTheServiceRestartsOnItsOwn(t *testing.T) {
	text := planText(t, nodeApp())

	if !strings.Contains(text, "Restart=on-failure") {
		t.Error("a crash would leave it down")
	}
	if !strings.Contains(text, "WantedBy=multi-user.target") {
		t.Error("it would not come back after a reboot")
	}
	if !strings.Contains(text, "EnvironmentFile=-") {
		t.Error("no place for environment variables, and a missing file would stop it starting")
	}
}

func TestAProjectWithNoBuildStepGetsNoBuildCommand(t *testing.T) {
	app := nodeApp()
	app.Build = nil

	if strings.Contains(planText(t, app), "npm run build") {
		t.Error("a build was invented")
	}
}

func TestSnapshotNamesSortByWhenTheyWereTaken(t *testing.T) {
	first := SnapshotName(time.Date(2026, 9, 10, 8, 0, 0, 0, time.UTC))
	second := SnapshotName(time.Date(2026, 9, 10, 9, 0, 0, 0, time.UTC))

	if !(first < second) {
		t.Errorf("%s does not sort before %s", first, second)
	}
	if !strings.HasPrefix(first, "croft-deploy-") {
		t.Errorf("%s does not say what made it", first)
	}
}

// Restoring brings back whatever the application had written, which is the
// part an image cannot do.
func TestRollbackRestoresTheSnapshot(t *testing.T) {
	planner := NewPlanner("lxc", "helpdesk")
	steps := planner.Rollback("croft-deploy-20260910-080000").Steps

	if len(steps) != 1 || !strings.Contains(steps[0].Shell(), "restore helpdesk croft-deploy-") {
		t.Fatalf("got %v", steps)
	}
}
