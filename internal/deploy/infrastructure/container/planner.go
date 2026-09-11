// Package container turns a service into the commands that put it on a
// container and keep it running.
package container

import (
	"fmt"
	"strings"
	"time"

	"github.com/Hyzokaaa/opencroft/internal/deploy/domain/entities"
	"github.com/Hyzokaaa/opencroft/internal/shared/plan"
)

// Depth is how much history comes with the checkout.
//
// One commit is enough to deploy and not enough to go back, and going back to
// the previous commit is the rollback that does not take the whole container
// with it. Ten is arbitrary — ten deployments of room, for a slightly slower
// clone.
const Depth = 10

// Keep is how many of our snapshots survive per service. The one recorded as
// healthy is kept regardless.
const Keep = 3

// Budget is how long readiness is waited for before a deployment is called
// failed.
const Budget = 30

type Planner struct {
	bin       string
	container string
	address   string
}

// NewPlanner needs the address because a deployment is only finished when
// something answers from outside the container — which is where nginx will be
// asking from.
func NewPlanner(bin, container, address string) *Planner {
	return &Planner{bin: bin, container: container, address: address}
}

// Deployment is everything the plan depends on. The snapshots already on the
// container are part of it because pruning is a visible step, not a background
// tidy-up.
type Deployment struct {
	Service  *entities.Service
	At       time.Time
	Existing []string
	Healthy  string
}

// exec wraps a shell command so it runs inside the container. `sh -lc` gives
// it a login shell, which is what puts freshly installed tools on the path.
func (p *Planner) exec(describe, command string) plan.Step {
	return plan.Command(describe, p.bin, "exec", p.container, "--", "sh", "-lc", command)
}

// Deploy is the whole thing, in the order it has to happen.
//
// The snapshot is first and is not optional. A deployment that breaks the
// service is recoverable; one that breaks the database inside the container is
// not, unless there is a point to go back to. Taking it costs seconds and is
// the one advantage a system container has over an image.
func (p *Planner) Deploy(d Deployment) plan.Plan {
	service := d.Service

	steps := []plan.Step{
		plan.Command("Take a snapshot to come back to",
			p.bin, "snapshot", p.container, entities.SnapshotName(service.Name, d.At)),
	}

	packages := append([]string{"git", "curl"}, service.Packages...)
	steps = append(steps,
		p.exec("Install "+strings.Join(packages, ", "),
			"apt-get update -qq && DEBIAN_FRONTEND=noninteractive apt-get install -y -qq "+
				strings.Join(packages, " ")))

	steps = append(steps, p.fetch(service))

	if len(service.Env) > 0 {
		steps = append(steps, p.writeEnv(service))
	}

	for _, command := range service.Install {
		steps = append(steps, p.exec("Install dependencies", p.inPath(service, command)))
	}
	for _, command := range service.Build {
		steps = append(steps, p.exec("Build", p.inPath(service, command)))
	}

	steps = append(steps,
		p.exec("Write the unit that keeps it running", p.unit(service)),
		p.exec("Start "+service.Unit(), "systemctl daemon-reload && systemctl enable --now "+
			service.Unit()+" && systemctl restart "+service.Unit()),
	)

	if service.Health.Wanted() {
		steps = append(steps, p.check(service))
	}

	// Last, so that a deployment that fails leaves every point of return it
	// had when it started.
	steps = append(steps, p.prune(d)...)

	return plan.New(steps...)
}

// fetch is one command that works whether or not the code is already there.
// Two plans — one for the first deployment and one for the rest — is two
// things to keep correct.
func (p *Planner) fetch(service *entities.Service) plan.Step {
	branch := service.Source.Branch
	if branch == "" {
		branch = "main"
	}

	// A pinned commit is how the previous version comes back without
	// restoring a snapshot, so it has to survive the same command.
	target, at := "FETCH_HEAD", branch
	if service.Source.Commit != "" {
		target, at = service.Source.Commit, service.Source.Commit
	}

	command := fmt.Sprintf(
		"if [ -d %s/.git ]; then "+
			"git -C %s fetch --depth %d origin %s && git -C %s reset --hard %s; "+
			"else git clone --depth %d --branch %s %s %s; fi",
		service.Path,
		service.Path, Depth, branch, service.Path, target,
		Depth, branch, service.Source.Repo, service.Path)

	return p.exec("Fetch "+service.Source.Repo+" at "+at, command)
}

// writeEnv puts the configuration beside the code. Written after the checkout,
// because cloning into a directory that already has files in it fails, and
// readable only by its owner because it is where the secrets are.
func (p *Planner) writeEnv(service *entities.Service) plan.Step {
	return p.exec(
		fmt.Sprintf("Write %s (%d variables)", service.EnvFile(), len(service.Env)),
		"cat > "+service.EnvFile()+" <<'CROFT_ENV'\n"+service.EnvContent()+"CROFT_ENV\n"+
			"chmod 600 "+service.EnvFile())
}

func (p *Planner) inPath(service *entities.Service, command string) string {
	return "cd " + service.Path + " && " + command
}

// unit is written for the container's own init, so the service is restarted by
// systemd rather than by us watching it.
func (p *Planner) unit(service *entities.Service) string {
	unit := fmt.Sprintf(`[Unit]
Description=%s, deployed by croft
After=network-online.target

[Service]
WorkingDirectory=%s
ExecStart=/bin/sh -lc '%s'
Restart=on-failure
RestartSec=3
EnvironmentFile=-%s

[Install]
WantedBy=multi-user.target
`, service.Name, service.Path, service.Start, service.EnvFile())

	return "cat > /etc/systemd/system/" + service.Unit() + ".service <<'CROFT_UNIT'\n" +
		unit + "CROFT_UNIT"
}

// check asks, repeatedly, whether the service is actually up.
//
// Nothing reports readiness: systemd calls a unit running the moment the
// process forks, long before anything listens. So it is a question asked until
// it is answered or the budget runs out — which is what everything claiming
// otherwise does underneath.
//
// It is asked from the host, not from inside the container, because that is
// where nginx will ask from. A service bound to 127.0.0.1 inside passes an
// inside check and serves nobody, and that is the mistake worth catching.
//
// The loop gives up early if the unit died, so a deployment that failed to
// start says so in seconds rather than after the whole budget.
func (p *Planner) check(service *entities.Service) plan.Step {
	url := fmt.Sprintf("http://%s:%d%s", p.address, service.Port, service.Health.Path)

	command := fmt.Sprintf(
		"for attempt in $(seq 1 %d); do "+
			"%s exec %s -- systemctl is-active --quiet %s || "+
			"{ echo '%s stopped before it answered'; exit 1; }; "+
			"answer=$(curl -fsS --max-time 2 -w '%%{http_code}' %s 2>/dev/null) && "+
			"case \"$answer\" in %s) %s;; esac; "+
			"sleep 1; done; "+
			"echo 'nothing answered %s within %d seconds'; exit 1",
		Budget,
		p.bin, p.container, service.Unit(),
		service.Unit(),
		url,
		"*"+fmt.Sprint(service.Health.Code()), body(service.Health.Contains),
		url, Budget)

	return plan.Command(p.describeCheck(service, url), "sh", "-c", command)
}

func (p *Planner) describeCheck(service *entities.Service, url string) string {
	describe := fmt.Sprintf("Wait until %s answers %d", url, service.Health.Code())
	if service.Health.Contains != "" {
		describe += " carrying " + service.Health.Contains
	}
	return describe
}

// body is the condition on what came back. An empty one is absent from the
// command rather than a check that always passes.
func body(want string) string {
	if want == "" {
		return "exit 0"
	}
	return "case \"$answer\" in *" + want + "*) exit 0;; esac"
}

// prune removes our older snapshots for this service, and only ours. Each
// removal is its own step so that it is read before it is approved — this is
// the one place croft deletes something on its own.
func (p *Planner) prune(d Deployment) []plan.Step {
	steps := []plan.Step{}

	for _, snapshot := range entities.Prunable(d.Existing, d.Service.Name, Keep, d.Healthy) {
		steps = append(steps, plan.Optional(
			fmt.Sprintf("Remove %s, keeping the last %d", snapshot, Keep),
			p.bin, "delete", p.container+"/"+snapshot))
	}
	return steps
}

// Rollback puts the container back to a snapshot. Restoring includes whatever
// the services had written to disk, which is the part an image cannot do — and
// the part that reaches every other service in the container.
func (p *Planner) Rollback(snapshot string) plan.Plan {
	return plan.New(
		plan.Command("Restore "+snapshot+", including anything written since",
			p.bin, "restore", p.container, snapshot),
	)
}

// Logs is what a failed deployment leaves behind, and the first thing anybody
// asks for.
func (p *Planner) Logs(service *entities.Service, lines int) plan.Plan {
	return plan.New(p.exec(fmt.Sprintf("Read the last %d lines", lines),
		fmt.Sprintf("journalctl -u %s -n %d --no-pager", service.Unit(), lines)))
}
