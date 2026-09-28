// Package container turns a service into the commands that put it on a
// container and keep it running.
package container

import (
	"fmt"
	"sort"
	"strconv"
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

// Patience is how long a step that has to finish is given. Installing a large
// project takes minutes; a command that was never going to return takes for
// ever, and telling the two apart is the whole point of having a limit.
const Patience = 900

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

// bounded is for the steps that have to finish: installing and building.
//
// A command that never returns — a server put in the wrong box — would
// otherwise hold the whole deployment until something far away gives up, with
// nothing said. `timeout` is visible in the plan and, because it runs inside
// the container, it also kills the process rather than leaving it orphaned
// when we stop waiting.
func (p *Planner) bounded(describe, command string) plan.Step {
	return plan.Command(describe, p.bin, "exec", p.container, "--",
		"timeout", strconv.Itoa(Patience), "sh", "-lc", command)
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

	// An adopted service's environment is the unit's own file, and croft
	// never writes it — whatever a request carries.
	if len(service.Env) > 0 && service.Adopted == nil {
		steps = append(steps, p.writeEnv(service))
	}

	// Numbered when there is more than one, because two steps called "Install
	// dependencies" tell you nothing about which of them you are watching.
	for i, command := range service.Install {
		steps = append(steps, p.bounded(
			counted("Install dependencies", i, len(service.Install)), p.inPath(service, command)))
	}
	for i, command := range service.Build {
		steps = append(steps, p.bounded(
			counted("Build", i, len(service.Build)), p.inPath(service, command)))
	}

	if adopted := service.Adopted; adopted != nil {
		// Whoever owned the checkout before still does. The build left what
		// it wrote owned by root.
		if adopted.RunAs != "" && adopted.RunAs != "root" {
			steps = append(steps, p.exec(
				"Give "+service.Path+" back to "+adopted.RunAs+", who owned it",
				"chown -R "+adopted.RunAs+": "+service.Path))
		}
		if service.IsSite() {
			steps = append(steps, p.publish(service))
		} else {
			// Its unit was written by somebody else and stays as they wrote it.
			steps = append(steps, p.exec("Restart "+service.Unit(), "systemctl restart "+service.Unit()))
		}
	} else {
		steps = append(steps,
			p.exec("Write the unit that keeps it running", p.unit(service)),
			p.exec("Start "+service.Unit(), "systemctl daemon-reload && systemctl enable --now "+
				service.Unit()+" && systemctl restart "+service.Unit()),
		)
	}

	if service.Health.Wanted() && !service.IsSite() {
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

	// An adopted checkout was there before croft, and is fetched as it is:
	// never cloned again, never made shallow, and fetched from the recorded
	// repository rather than whatever origin says. It usually belongs to the
	// user the unit runs as, and git refuses to touch a repository owned by
	// someone else unless told this one is expected — told here, for this
	// command only, rather than in anybody's configuration.
	if service.Adopted != nil {
		git := "git -c safe.directory=" + service.Path + " -C " + service.Path
		return p.exec("Fetch "+service.Source.Repo+" at "+at+" into the existing checkout",
			fmt.Sprintf("%s fetch %s %s && %s reset --hard %s",
				git, service.Source.Repo, branch, git, target))
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

// publish puts a site's new build where the web server reads it.
//
// A build with no index.html is refused before anything is touched: a site
// that published an empty directory would answer every request with an error,
// and the check costs nothing. The new files are copied beside the old ones,
// given the old directory's owner and mode, and swapped in by two renames —
// so the site serves the whole old build or the whole new one, never a half
// copy, and the gap between the renames is the only moment it serves neither.
// Nothing is restarted: the server reads files from disk on each request.
func (p *Planner) publish(service *entities.Service) plan.Step {
	built := service.Path + "/" + service.Adopted.Output
	site := service.Adopted.Site

	command := strings.Join([]string{
		"test -f " + built + "/index.html",
		"rm -rf " + site + ".croft-new " + site + ".croft-old",
		"cp -r " + built + " " + site + ".croft-new",
		"chown -R --reference=" + site + " " + site + ".croft-new",
		"chmod --reference=" + site + " " + site + ".croft-new",
		"mv " + site + " " + site + ".croft-old",
		"mv " + site + ".croft-new " + site,
		"rm -rf " + site + ".croft-old",
	}, " && ")

	return p.exec("Publish "+built+" to "+site, command)
}

// writeEnv puts the configuration beside the code. Written after the checkout,
// because cloning into a directory that already has files in it fails, and
// readable only by its owner because it is where the secrets are.
func (p *Planner) writeEnv(service *entities.Service) plan.Step {
	step := p.exec(
		fmt.Sprintf("Write %s (%d variables)", service.EnvFile(), len(service.Env)),
		"cat > "+service.EnvFile()+" <<'CROFT_ENV'\n"+service.EnvContent()+"CROFT_ENV\n"+
			"chmod 600 "+service.EnvFile())
	step.Secret = true
	return step
}

func (p *Planner) inPath(service *entities.Service, command string) string {
	return "cd " + service.Path + " && " + command
}

// unit is written for the container's own init, so the service is restarted by
// systemd rather than by us watching it.
//
// It reads two environment files, and the order is the point. /etc/croft/db.env
// is regenerated by the database module whenever a database is provisioned or
// dropped; the path is fixed so that adding a database never means rewriting a
// unit, and the leading dash means a container without one is not an error.
// The file the person edits comes second, because systemd lets the last one
// win — a value somebody typed always beats one we generated.
func (p *Planner) unit(service *entities.Service) string {
	unit := fmt.Sprintf(`[Unit]
Description=%s, deployed by croft
After=network-online.target

[Service]
WorkingDirectory=%s
ExecStart=/bin/sh -lc '%s'
Restart=on-failure
RestartSec=3
EnvironmentFile=-/etc/croft/db.env
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
//
// This is the only shell croft runs on the host rather than inside a
// container, which makes it the only place where a field from a request could
// reach root. The URL is therefore single-quoted, and the health path is
// refused if it carries a quote that could close it. Quoting is the fix;
// rejecting the quote is the belt to its braces.
//
// curl is asked with -f only when the expected status is a success. -f makes
// curl exit non-zero on 4xx and 5xx, which would short-circuit the && and
// leave a check for an expected 401 or 404 timing out against a service that
// was answering correctly all along.
func (p *Planner) check(service *entities.Service) plan.Step {
	url := fmt.Sprintf("http://%s:%d%s", p.address, service.Port, service.Health.Path)

	fail := "f"
	if service.Health.Code() >= 400 {
		fail = ""
	}

	command := fmt.Sprintf(
		"for attempt in $(seq 1 %d); do "+
			"%s exec %s -- systemctl is-active --quiet %s || "+
			"{ echo '%s stopped before it answered'; exit 1; }; "+
			"answer=$(curl -%ssS --max-time 2 -w '%%{http_code}' '%s' 2>/dev/null) && "+
			"case \"$answer\" in %s) %s;; esac; "+
			"sleep 1; done; "+
			"echo 'nothing answered %s within %d seconds'; exit 1",
		Budget,
		p.bin, p.container, service.Unit(),
		service.Unit(),
		fail, url,
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

// RestartUnit, StopUnit and StartUnit touch nothing the unit runs — nothing
// is fetched, installed or built. They exist for what a full Deploy is not: a
// process that hung, or a change made by hand that only needs systemd to
// notice. Taking a raw unit name rather than a Service is what lets them work
// on something croft did not deploy — found on the container, not written by
// us — the same way.
func (p *Planner) RestartUnit(unit string) plan.Plan {
	return plan.New(p.exec("Restart "+unit, "systemctl restart "+unit))
}

func (p *Planner) StopUnit(unit string) plan.Plan {
	return plan.New(p.exec("Stop "+unit, "systemctl stop "+unit))
}

func (p *Planner) StartUnit(unit string) plan.Plan {
	return plan.New(p.exec("Start "+unit, "systemctl start "+unit))
}

// LogsUnit is what a failed deployment leaves behind, and the first thing
// anybody asks for.
func (p *Planner) LogsUnit(unit string, lines int) plan.Plan {
	return plan.New(p.exec(fmt.Sprintf("Read the last %d lines", lines),
		fmt.Sprintf("journalctl -u %s -n %d --no-pager", unit, lines)))
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

func counted(describe string, i, total int) string {
	if total < 2 {
		return describe
	}
	return fmt.Sprintf("%s (%d of %d)", describe, i+1, total)
}

// Destroy takes a service off the container.
//
// The snapshot is first and is not a DeployKind, so nothing will ever prune
// it: everything below this line is irreversible otherwise, and it includes
// the environment file and whatever the service wrote beside its code.
//
// The annotations go last. A failure halfway then leaves a service the panel
// still knows about — which can be looked at and tried again — rather than a
// directory nobody remembers owning.
func (p *Planner) Destroy(service *entities.Service, keys []string, remaining []string, at time.Time) plan.Plan {
	steps := []plan.Step{
		plan.Command("Take a snapshot, in case this was a mistake",
			p.bin, "snapshot", p.container, entities.FarewellName(service.Name, at)),

		// Optional: a unit that was never written, or already gone, is not a
		// reason to stop.
		plan.Optional("Stop and disable "+service.Unit(),
			p.bin, "exec", p.container, "--", "systemctl", "disable", "--now", service.Unit()),
	}

	steps = append(steps,
		p.exec("Remove the unit", "rm -f /etc/systemd/system/"+service.Unit()+
			".service && systemctl daemon-reload"),
		p.exec("Remove "+service.Path+", including its .env and anything written beside it",
			"rm -rf "+service.Path),
	)

	return plan.New(append(steps, p.forget(keys, remaining)...)...)
}

// Adopt writes down what croft now knows about a service it found running.
// Nothing runs inside the container: taking something on is recording what it
// is, and the first change to it is the next deployment — with its own plan
// and its own snapshot.
func (p *Planner) Adopt(service *entities.Service, index []string) plan.Plan {
	record := service.Record()

	keys := make([]string, 0, len(record))
	for key := range record {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	steps := []plan.Step{}
	for _, key := range keys {
		steps = append(steps, plan.Command("Record "+key,
			p.bin, "config", "set", p.container, entities.ServiceKey(service.Name, key), record[key]))
	}
	steps = append(steps, plan.Command("Add "+service.Name+" to the index",
		p.bin, "config", "set", p.container, entities.IndexKey, strings.Join(index, " ")))

	return plan.New(steps...)
}

// Release is how an adopted service is let go. Only croft's own notes are
// forgotten: the unit, the code and the environment stay where they are,
// running as they were, exactly as before croft took them on.
func (p *Planner) Release(keys []string, remaining []string) plan.Plan {
	return plan.New(p.forget(keys, remaining)...)
}

// forget removes what croft recorded about a service. It goes last in any
// plan that ends a service, so a failure halfway leaves one the panel still
// knows about — which can be looked at and tried again — rather than a
// directory nobody remembers owning.
func (p *Planner) forget(keys []string, remaining []string) []plan.Step {
	steps := []plan.Step{}
	for _, key := range keys {
		steps = append(steps, plan.Optional("Forget "+key,
			p.bin, "config", "unset", p.container, key))
	}

	// Written rather than unset when something is left, so the index never
	// disagrees with what is actually on the container.
	if len(remaining) > 0 {
		steps = append(steps, plan.Command("Leave "+strings.Join(remaining, ", ")+" in the index",
			p.bin, "config", "set", p.container, entities.IndexKey, strings.Join(remaining, " ")))
	} else {
		steps = append(steps, plan.Optional("Empty the index",
			p.bin, "config", "unset", p.container, entities.IndexKey))
	}
	return steps
}
