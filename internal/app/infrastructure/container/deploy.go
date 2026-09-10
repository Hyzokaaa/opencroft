// Package container turns an application into the commands that put it on a
// container and keep it running.
package container

import (
	"fmt"
	"strings"
	"time"

	"github.com/Hyzokaaa/opencroft/internal/app/domain/entities"
	"github.com/Hyzokaaa/opencroft/internal/shared/plan"
)

// UnitName is the service inside the container. One per container: an App is
// what the container runs, not one of several things it might run.
const UnitName = "croft-app"

type Planner struct {
	bin       string
	container string
}

func NewPlanner(bin, container string) *Planner {
	return &Planner{bin: bin, container: container}
}

// exec wraps a shell command so it runs inside the container. `sh -lc` gives
// it a login shell, which is what puts freshly installed tools on the path.
func (p *Planner) exec(describe, command string) plan.Step {
	return plan.Command(describe, p.bin, "exec", p.container, "--", "sh", "-lc", command)
}

// Deploy is the whole thing, in the order it has to happen.
//
// The snapshot is first and is not optional. A deployment that breaks the
// application is recoverable; one that breaks the database inside the
// container is not, unless there is a point to go back to. Taking it costs
// seconds and is the one advantage a system container has over an image.
func (p *Planner) Deploy(app *entities.App, at time.Time) plan.Plan {
	steps := []plan.Step{
		plan.Command("Take a snapshot to come back to",
			p.bin, "snapshot", p.container, SnapshotName(at)),
	}

	packages := append([]string{"git"}, app.Packages...)
	steps = append(steps,
		p.exec("Install "+strings.Join(packages, ", "),
			"apt-get update -qq && DEBIAN_FRONTEND=noninteractive apt-get install -y -qq "+
				strings.Join(packages, " ")))

	steps = append(steps, p.fetch(app))

	for _, command := range app.Install {
		steps = append(steps, p.exec("Install dependencies", p.inPath(app, command)))
	}
	for _, command := range app.Build {
		steps = append(steps, p.exec("Build", p.inPath(app, command)))
	}

	steps = append(steps,
		p.exec("Write the service that keeps it running", p.unit(app)),
		p.exec("Start it", "systemctl daemon-reload && systemctl enable --now "+UnitName+
			" && systemctl restart "+UnitName),
	)

	return plan.New(steps...)
}

// fetch is one command that works whether or not the code is already there.
// Two plans — one for the first deployment and one for the rest — is two
// things to keep correct.
func (p *Planner) fetch(app *entities.App) plan.Step {
	branch := app.Source.Branch
	if branch == "" {
		branch = "main"
	}

	command := fmt.Sprintf(
		"if [ -d %s/.git ]; then "+
			"git -C %s fetch --depth 1 origin %s && git -C %s reset --hard FETCH_HEAD; "+
			"else git clone --depth 1 --branch %s %s %s; fi",
		app.Path, app.Path, branch, app.Path, branch, app.Source.Repo, app.Path)

	return p.exec("Fetch "+app.Source.Repo+" at "+branch, command)
}

func (p *Planner) inPath(app *entities.App, command string) string {
	return "cd " + app.Path + " && " + command
}

// unit is written by the container's own init, so the application is restarted
// by systemd rather than by us watching it.
func (p *Planner) unit(app *entities.App) string {
	unit := fmt.Sprintf(`[Unit]
Description=%s, deployed by croft
After=network-online.target

[Service]
WorkingDirectory=%s
ExecStart=/bin/sh -lc '%s'
Restart=on-failure
RestartSec=3
EnvironmentFile=-%s/.env

[Install]
WantedBy=multi-user.target
`, p.container, app.Path, app.Start, app.Path)

	return "cat > /etc/systemd/system/" + UnitName + ".service <<'CROFT_UNIT'\n" + unit + "CROFT_UNIT"
}

// SnapshotName is sortable and says what made it, so a person listing
// snapshots can tell ours from theirs.
func SnapshotName(at time.Time) string {
	return "croft-deploy-" + at.UTC().Format("20060102-150405")
}

// Rollback puts the container back to a snapshot. Restoring includes whatever
// the application had written to disk, which is the part an image cannot do.
func (p *Planner) Rollback(snapshot string) plan.Plan {
	return plan.New(
		plan.Command("Restore "+snapshot+", including anything written since",
			p.bin, "restore", p.container, snapshot),
	)
}

// Logs is what a failed deployment leaves behind, and the first thing anybody
// asks for.
func (p *Planner) Logs(lines int) plan.Plan {
	return plan.New(p.exec(fmt.Sprintf("Read the last %d lines", lines),
		fmt.Sprintf("journalctl -u %s -n %d --no-pager", UnitName, lines)))
}
