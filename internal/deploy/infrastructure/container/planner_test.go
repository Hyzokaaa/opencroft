package container

import (
	"strings"
	"testing"
	"time"

	"github.com/Hyzokaaa/opencroft/internal/deploy/domain/entities"
	"github.com/Hyzokaaa/opencroft/internal/shared/plan"
)

func nodeService() *entities.Service {
	return entities.NewService(entities.ServiceProps{
		Name:     "backend",
		Source:   entities.Source{Repo: "https://github.com/user/app.git", Branch: "main"},
		Install:  []string{"npm ci"},
		Build:    []string{"npm run build"},
		Start:    "npm start",
		Runtime:  "node",
		Port:     3000,
		Packages: []string{"nodejs", "npm"},
	})
}

func planner() *Planner { return NewPlanner("lxc", "helpdesk", "10.0.0.200") }

func deploy(t *testing.T, service *entities.Service) plan.Plan {
	t.Helper()
	return planner().Deploy(Deployment{Service: service, At: time.Unix(0, 0)})
}

func text(p plan.Plan) string {
	lines := []string{}
	for _, step := range p.Steps {
		lines = append(lines, step.Shell())
	}
	return strings.Join(lines, "\n")
}

// A deployment that breaks the service is recoverable. One that breaks the
// database inside the container is not, unless there is a point to go back to.
func TestTheSnapshotIsTakenBeforeAnythingElse(t *testing.T) {
	steps := deploy(t, nodeService()).Steps

	if len(steps) == 0 {
		t.Fatal("empty plan")
	}
	if !strings.Contains(steps[0].Shell(), "snapshot") {
		t.Fatalf("the first step is %q", steps[0].Shell())
	}
}

// Every step either acts on the container from outside, or runs inside it, or
// is the readiness check — which has to be outside, and says so.
func TestTheCommandsGoWhereTheyShould(t *testing.T) {
	for _, step := range deploy(t, withHealth(nodeService())).Steps {
		switch step.Argv[0] {
		case "sh":
			if !strings.Contains(step.Describe, "answers") {
				t.Errorf("%q runs a shell on the host for no stated reason", step.Describe)
			}
		case "lxc":
			if step.Argv[1] == "exec" && step.Argv[2] != "helpdesk" {
				t.Errorf("%q runs in %q", step.Describe, step.Argv[2])
			}
		default:
			t.Errorf("%q starts with %q", step.Describe, step.Argv[0])
		}
	}
}

// One command that works whether or not the code is already there. Two plans,
// one for the first deployment and one for the rest, is two things to keep
// correct.
func TestFetchingWorksTheFirstTimeAndEveryTimeAfter(t *testing.T) {
	body := text(deploy(t, nodeService()))

	if !strings.Contains(body, "git clone") {
		t.Error("no first deployment")
	}
	if !strings.Contains(body, "reset --hard FETCH_HEAD") {
		t.Error("no way to update an existing checkout")
	}
}

// One commit is enough to deploy and not enough to go back. Going back to the
// previous commit is the rollback that leaves the rest of the container alone,
// so the history has to come down with it.
func TestEnoughHistoryComesDownToGoBack(t *testing.T) {
	if Depth < 2 {
		t.Fatalf("a depth of %d cannot reach a previous commit", Depth)
	}
	if !strings.Contains(text(deploy(t, nodeService())), "--depth 10") {
		t.Error("the checkout is shallower than the rollback needs")
	}
}

func TestAPinnedCommitIsWhatGetsCheckedOut(t *testing.T) {
	service := nodeService()
	service.Source.Commit = "abc1234"

	body := text(deploy(t, service))
	if !strings.Contains(body, "reset --hard abc1234") {
		t.Errorf("the pinned commit is not checked out:\n%s", body)
	}
}

func TestTheStepsHappenInAnOrderThatCanWork(t *testing.T) {
	service := withEnv(withHealth(nodeService()))
	lines := strings.Split(text(deploy(t, service)), "\n")

	at := func(fragment string) int {
		for i, line := range lines {
			if strings.Contains(line, fragment) {
				return i
			}
		}
		return -1
	}

	// The environment file is written after the checkout: cloning into a
	// directory that already has files in it fails.
	order := []string{
		"snapshot", "apt-get install", "git clone", ".env",
		"npm ci", "npm run build", "systemctl enable", "curl -fsS",
	}
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

// git fetches the code and curl is what asks whether it came up. Neither is
// optional, whatever the runtime needs.
func TestTheToolsTheDeploymentItselfNeedsAreAlwaysInstalled(t *testing.T) {
	service := nodeService()
	service.Packages = nil

	body := text(deploy(t, service))
	for _, tool := range []string{"git", "curl"} {
		if !strings.Contains(body, " "+tool) {
			t.Errorf("%s is not installed:\n%s", tool, body)
		}
	}
}

// systemd restarts the service, not us watching it.
func TestTheUnitRestartsOnItsOwn(t *testing.T) {
	body := text(deploy(t, nodeService()))

	if !strings.Contains(body, "Restart=on-failure") {
		t.Error("a crash would leave it down")
	}
	if !strings.Contains(body, "WantedBy=multi-user.target") {
		t.Error("it would not come back after a reboot")
	}
	if !strings.Contains(body, "EnvironmentFile=-") {
		t.Error("a missing environment file would stop it starting")
	}
}

// A container can run several services, so nothing about one may be named
// after the container. Two services sharing a unit would silently replace each
// other.
func TestEachServiceGetsItsOwnUnitAndPath(t *testing.T) {
	backend := nodeService()
	client := entities.NewService(entities.ServiceProps{
		Name:   "client",
		Source: entities.Source{Repo: "https://github.com/user/web.git"},
		Start:  "nginx -g 'daemon off;'",
	})

	if backend.Unit() == client.Unit() {
		t.Errorf("both are %q", backend.Unit())
	}
	if backend.Path == client.Path {
		t.Errorf("both live in %q", backend.Path)
	}
	if !strings.Contains(text(deploy(t, backend)), "croft-backend") {
		t.Error("the unit is not named after the service")
	}
}

func TestTheEnvironmentIsWrittenBesideTheCodeAndKeptPrivate(t *testing.T) {
	body := text(deploy(t, withEnv(nodeService())))

	if !strings.Contains(body, "/srv/backend/.env") {
		t.Error("the environment file is not beside the code")
	}
	if !strings.Contains(body, "JWT_SECRET=s3cret") {
		t.Error("the variables are not written")
	}
	if !strings.Contains(body, "chmod 600") {
		t.Error("the file holding the secrets is left world-readable")
	}
}

// The file is shown in the plan before it is written, so it must not churn
// between two identical deployments.
func TestTheEnvironmentFileIsStable(t *testing.T) {
	first := withEnv(nodeService()).EnvContent()

	for i := 0; i < 20; i++ {
		if again := withEnv(nodeService()).EnvContent(); again != first {
			t.Fatalf("the file changed between renderings:\n%q\n%q", first, again)
		}
	}
}

func TestNoEnvironmentMeansNoStepAtAll(t *testing.T) {
	if strings.Contains(text(deploy(t, nodeService())), ".env <<") {
		t.Error("an empty environment file would be written for no reason")
	}
}

// Nothing reports readiness, so it is asked repeatedly — from the host,
// because that is where nginx will ask from. A service bound to 127.0.0.1
// inside the container passes an inside check and serves nobody.
func TestReadinessIsAskedFromOutsideTheContainer(t *testing.T) {
	body := text(deploy(t, withHealth(nodeService())))

	if !strings.Contains(body, "http://10.0.0.200:3000/health") {
		t.Errorf("the check does not go through the container's address:\n%s", body)
	}
	if !strings.Contains(body, "is-active") {
		t.Error("a unit that died would be waited for anyway")
	}
	if !strings.Contains(body, "exit 1") {
		t.Error("a deployment that never answers would be called a success")
	}
}

func TestABodyConditionOnlyAppearsWhenAsked(t *testing.T) {
	service := withHealth(nodeService())
	service.Health.Contains = "ok"

	if !strings.Contains(text(deploy(t, service)), "*ok*") {
		t.Error("the body condition is missing")
	}
}

// Without somewhere to ask, the deployment finishes when the unit is up, which
// is a weaker promise — but not a false one.
func TestNoHealthPathMeansNoWaiting(t *testing.T) {
	if strings.Contains(text(deploy(t, nodeService())), "curl -fsS") {
		t.Error("something is being waited for that was never configured")
	}
}

// Pruning is the one place croft removes something on its own, so it happens
// in the open, one step at a time.
func TestPruningOnlyTouchesOurOwnSnapshots(t *testing.T) {
	existing := []string{
		"before-upgrade",
		entities.SnapshotName("backend", when(1)),
		entities.SnapshotName("backend", when(2)),
		entities.SnapshotName("backend", when(3)),
		entities.SnapshotName("backend", when(4)),
		entities.SnapshotName("client", when(1)),
	}

	p := planner().Deploy(Deployment{
		Service: nodeService(), At: time.Unix(0, 0), Existing: existing,
	})

	removed := []string{}
	for _, step := range p.Steps {
		if len(step.Argv) > 1 && step.Argv[1] == "delete" {
			removed = append(removed, step.Argv[2])
		}
	}

	if len(removed) == 0 {
		t.Fatal("nothing was pruned, so snapshots accumulate forever")
	}
	for _, snapshot := range removed {
		if strings.Contains(snapshot, "before-upgrade") {
			t.Errorf("would remove %q, which we did not take", snapshot)
		}
		if strings.Contains(snapshot, "client") {
			t.Errorf("would remove %q, which belongs to another service", snapshot)
		}
	}
}

// A deployment that fails must leave every point of return it started with.
func TestNothingIsPrunedBeforeTheDeploymentHasWorked(t *testing.T) {
	existing := []string{}
	for i := 1; i <= 6; i++ {
		existing = append(existing, entities.SnapshotName("backend", when(i)))
	}

	steps := planner().Deploy(Deployment{
		Service: withHealth(nodeService()), At: time.Unix(0, 0), Existing: existing,
	}).Steps

	checked := false
	for i, step := range steps {
		if strings.Contains(step.Describe, "answers") {
			checked = true
		}
		if len(step.Argv) > 1 && step.Argv[1] == "delete" && !checked {
			t.Fatalf("step %d removes a snapshot before the deployment was known to work", i+1)
		}
	}
}

func withHealth(service *entities.Service) *entities.Service {
	service.Health = entities.Health{Path: "/health"}
	return service
}

func withEnv(service *entities.Service) *entities.Service {
	service.Env = map[string]string{"JWT_SECRET": "s3cret", "DB_HOST": "postgres", "PORT": "3000"}
	return service
}

func when(hour int) time.Time {
	return time.Date(2026, 9, 11, hour, 0, 0, 0, time.UTC)
}

// A command that never returns — a server typed into the wrong box — would
// otherwise hold the deployment until something far away gave up, with nothing
// said. The limit runs inside the container, so it kills the process too
// rather than leaving it orphaned.
func TestTheStepsThatMustFinishAreGivenALimit(t *testing.T) {
	for _, step := range deploy(t, nodeService()).Steps {
		mustFinish := step.Describe == "Install dependencies" || step.Describe == "Build"

		bounded := len(step.Argv) > 3 && step.Argv[3] == "--" && step.Argv[4] == "timeout"
		if mustFinish && !bounded {
			t.Errorf("%q could run for ever: %v", step.Describe, step.Argv)
		}
		if !mustFinish && bounded {
			t.Errorf("%q is limited and should not be: %v", step.Describe, step.Argv)
		}
	}
}

// The one that keeps running is the unit, and systemd is what watches it.
// A limit there would stop the service itself.
func TestTheServiceItselfIsNotLimited(t *testing.T) {
	if strings.Contains(text(deploy(t, nodeService())), "timeout 900 sh -lc systemctl") {
		t.Error("starting the service is on a clock")
	}
}

// Everything a removal does below the snapshot is irreversible, so the
// snapshot has to come first — and it must not be a deploy snapshot, because
// pruning would eventually take away the only way back to something somebody
// chose to delete.
func TestRemovingAServiceKeepsAWayBackForever(t *testing.T) {
	service := nodeService()
	steps := planner().Destroy(service, []string{"user.croft.service.backend.repo"}, nil, when(1)).Steps

	if len(steps) == 0 || !strings.Contains(steps[0].Shell(), "snapshot") {
		t.Fatalf("the first step is %v", steps)
	}

	taken := steps[0].Argv[len(steps[0].Argv)-1]
	if !entities.Ours(taken) {
		t.Errorf("%s would not be recognised as ours", taken)
	}
	if entities.OfService(taken, service.Name) {
		t.Errorf("%s is prunable, so the only way back would age out", taken)
	}
}

// The environment file lives beside the code, so removing the checkout takes
// the secrets with it. That is correct, and it is why the plan has to say so.
func TestRemovingAServiceSaysItTakesTheEnvironmentWithIt(t *testing.T) {
	service := nodeService()
	p := planner().Destroy(service, nil, nil, when(1))

	if !strings.Contains(text(p), "rm -rf /srv/backend") {
		t.Errorf("the checkout is left behind:\n%s", text(p))
	}

	said := false
	for _, step := range p.Steps {
		if strings.Contains(step.Describe, ".env") {
			said = true
		}
	}
	if !said {
		t.Error("nothing warns that the environment file goes too")
	}
}

// A failure halfway should leave a service the panel still knows about, which
// can be looked at and tried again — not a directory nobody remembers owning.
func TestWhatCroftRecordedIsForgottenLast(t *testing.T) {
	keys := []string{"user.croft.service.backend.repo", "user.croft.service.backend.start"}
	steps := planner().Destroy(nodeService(), keys, nil, when(1)).Steps

	removal, forgetting := -1, -1
	for i, step := range steps {
		if strings.Contains(step.Shell(), "rm -rf") {
			removal = i
		}
		if strings.Contains(step.Shell(), "config unset") && forgetting < 0 {
			forgetting = i
		}
	}

	if removal < 0 || forgetting < 0 || forgetting < removal {
		t.Errorf("forgetting happens at %d and removal at %d", forgetting, removal)
	}
}

// The index has to agree with what is on the container, so what is left is
// written rather than the removed one being subtracted somewhere else.
func TestTheIndexIsLeftSayingWhatRemains(t *testing.T) {
	body := text(planner().Destroy(nodeService(), nil, []string{"client"}, when(1)))

	if !strings.Contains(body, "config set helpdesk "+entities.IndexKey+" client") {
		t.Errorf("the index would not match the container:\n%s", body)
	}
}
