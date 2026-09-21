// Package container turns a database into the commands that put it inside a
// container and hand its credentials to the applications beside it.
package container

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/Hyzokaaa/opencroft/internal/database/domain/entities"
	"github.com/Hyzokaaa/opencroft/internal/database/domain/enums"
	"github.com/Hyzokaaa/opencroft/internal/shared/plan"
)

// Ready is how long the engine is given to accept a connection before
// provisioning is called failed. Installing takes minutes; coming up takes
// seconds, and a minute of silence means something is wrong.
const Ready = 60

type Planner struct {
	bin       string
	container string
}

func NewPlanner(bin, container string) *Planner {
	return &Planner{bin: bin, container: container}
}

func (p *Planner) exec(describe, command string) plan.Step {
	return plan.Command(describe, p.bin, "exec", p.container, "--", "sh", "-lc", command)
}

// ── What the distribution calls things ────────────────────────────────────────
//
// This is the only file that knows. Supporting a second package manager means
// three more switches here and nothing anywhere else.

func pkg(engine enums.Engine) string {
	switch engine {
	case enums.EnginePostgres:
		return "postgresql"
	case enums.EngineMySQL:
		return "mariadb-server"
	default:
		return "redis-server"
	}
}

// Unit is the systemd unit the engine runs as. It is exported because the
// agent asks whether it is running, and that question belongs to whoever knows
// what the distribution called it.
func Unit(engine enums.Engine) string {
	switch engine {
	case enums.EnginePostgres:
		return "postgresql"
	case enums.EngineMySQL:
		return "mariadb"
	default:
		return "redis-server"
	}
}

// ready is the question asked until the engine answers it. Nothing reports
// readiness: a unit is active the moment the process forks, which is well
// before it is listening.
func ready(engine enums.Engine) string {
	switch engine {
	case enums.EnginePostgres:
		return "pg_isready -q"
	case enums.EngineMySQL:
		return "mariadb-admin ping --silent"
	default:
		return "redis-cli ping >/dev/null 2>&1"
	}
}

// ── The password ──────────────────────────────────────────────────────────────

// generate is the line that makes this design work.
//
// The plan shows the exact commands that run, so a password computed
// beforehand would travel over the socket, through the panel, into the browser
// and into every job event — job.Event records step.Shell() verbatim. Showing
// a placeholder instead would break the one rule the plan exists to keep.
//
// So it is not computed beforehand. The step generates the password itself,
// inside the container, uses it, and writes it where it belongs. The plan is
// honest to the character and carries no secret, because at the moment it is
// read the secret does not exist yet.
//
// Alphanumeric by construction, so there is nothing to escape in the SQL that
// follows it.
const generate = "PW=$(head -c 24 /dev/urandom | base64 | tr -dc A-Za-z0-9 | head -c 32)"

// regenerate rebuilds the single file every unit reads, out of the one file
// per database on disk.
//
// This is what keeps the two modules apart. A unit carries one fixed
// EnvironmentFile line, written once when the service was deployed, so
// provisioning a database never has to rewrite a unit or even know that
// services exist.
const regenerate = "{ cat " + entities.Dir + "/*.env 2>/dev/null || true; } > " + entities.EnvPath +
	"\nchmod 600 " + entities.EnvPath

// ── Provisioning ──────────────────────────────────────────────────────────────

// Provision installs the engine and creates one database and one user for the
// application beside it.
//
// The snapshot is first and is not optional, for the same reason it is in a
// deployment: everything after it writes to disk, and here the disk is the
// whole point.
func (p *Planner) Provision(d *entities.Database, at time.Time, index, services []string) plan.Plan {
	steps := []plan.Step{
		plan.Command("Take a snapshot to come back to",
			p.bin, "snapshot", p.container, d.ProvisionName(at)),

		p.exec("Install "+pkg(d.Engine),
			"apt-get update -qq && DEBIAN_FRONTEND=noninteractive apt-get install -y -qq "+pkg(d.Engine)),

		p.exec("Start "+Unit(d.Engine)+" and enable it at boot",
			"systemctl enable --now "+Unit(d.Engine)),

		p.exec(fmt.Sprintf("Wait until %s accepts connections", d.Engine), waitFor(d.Engine)),

		p.exec(p.describeCreate(d), p.create(d)),
	}

	for _, a := range d.Annotations() {
		if a[1] == "" {
			continue
		}
		steps = append(steps, plan.Command(
			"Record "+a[0]+" on the container",
			p.bin, "config", "set", p.container,
			entities.KeyPrefix+d.Name+"."+a[0], a[1]))
	}

	steps = append(steps, plan.Command(
		"List "+strings.Join(index, ", ")+" in the index",
		p.bin, "config", "set", p.container, entities.IndexKey, strings.Join(index, " ")))

	// A unit written by croft 0.20 or later already reads the file the previous
	// step regenerated, and a restart is all it takes. One written before that
	// does not, and would come back up without its credentials and without
	// saying why — so the line is put there if it is missing.
	//
	// Checked rather than assumed, because a container deployed months ago is
	// exactly the one somebody adds a database to.
	for _, service := range services {
		steps = append(steps, plan.Optional(
			"Restart croft-"+service+" so it reads the new credentials",
			p.bin, "exec", p.container, "--", "sh", "-lc", p.reread(service)))
	}

	return plan.New(steps...)
}

// reread makes a service pick the credentials up, whatever version of croft
// wrote its unit.
//
// The EnvironmentFile line is a fixed path so that provisioning never has to
// regenerate a unit — but a unit that predates the path has no line to fix, so
// it gets one. Idempotent: the grep means running this twice changes nothing.
func (p *Planner) reread(service string) string {
	unit := "/etc/systemd/system/croft-" + service + ".service"
	line := "EnvironmentFile=-" + entities.EnvPath

	return "unit=" + unit + "\n" +
		"if [ -f \"$unit\" ] && ! grep -q '" + line + "' \"$unit\"; then\n" +
		"  sed -i '/^\\[Service\\]/a " + line + "' \"$unit\"\n" +
		"  systemctl daemon-reload\n" +
		"fi\n" +
		"systemctl restart croft-" + service
}

func waitFor(engine enums.Engine) string {
	return fmt.Sprintf(
		"for attempt in $(seq 1 %d); do %s && exit 0; sleep 1; done; "+
			"echo '%s did not accept a connection within %d seconds'; exit 1",
		Ready, ready(engine), Unit(engine), Ready)
}

func (p *Planner) describeCreate(d *entities.Database) string {
	if !d.Engine.Credentialed() {
		return fmt.Sprintf("Write %s, reachable on the loopback only", d.EnvFile())
	}
	return fmt.Sprintf(
		"Create %s and the user %s, with a password generated inside the container",
		d.DB, d.User)
}

// create is one step on purpose: the password has to be made, used and written
// without ever existing anywhere a shell variable does not reach.
func (p *Planner) create(d *entities.Database) string {
	head := "set -e\ninstall -d -m 0700 " + entities.Dir + "\n"
	port := strconv.Itoa(d.Port)

	switch d.Engine {
	case enums.EnginePostgres:
		return head + generate + "\n" +
			"su -s /bin/sh postgres -c 'psql -v ON_ERROR_STOP=1' <<SQL\n" +
			"CREATE USER " + d.User + " PASSWORD '$PW';\n" +
			"CREATE DATABASE " + d.DB + " OWNER " + d.User + ";\n" +
			"SQL\n" +
			p.write(d, []string{
				"DATABASE_URL=postgres://" + d.User + ":$PW@127.0.0.1:" + port + "/" + d.DB,
				"PGHOST=127.0.0.1",
				"PGPORT=" + port,
				"PGUSER=" + d.User,
				"PGPASSWORD=$PW",
				"PGDATABASE=" + d.DB,
			})

	case enums.EngineMySQL:
		// No backquotes around the identifiers: inside a double-quoted shell
		// string they are command substitution, and the names are validated to
		// a shape that never needs quoting in the first place.
		return head + generate + "\n" +
			"mariadb -e \"CREATE DATABASE IF NOT EXISTS " + d.DB + "; " +
			"CREATE USER '" + d.User + "'@'localhost' IDENTIFIED BY '$PW'; " +
			"GRANT ALL PRIVILEGES ON " + d.DB + ".* TO '" + d.User + "'@'localhost'; " +
			"FLUSH PRIVILEGES;\"\n" +
			p.write(d, []string{
				"DATABASE_URL=mysql://" + d.User + ":$PW@127.0.0.1:" + port + "/" + d.DB,
				"MYSQL_HOST=127.0.0.1",
				"MYSQL_PORT=" + port,
				"MYSQL_USER=" + d.User,
				"MYSQL_PASSWORD=$PW",
				"MYSQL_DATABASE=" + d.DB,
			})

	default:
		return head + p.write(d, []string{
			"REDIS_URL=redis://127.0.0.1:" + port + "/0",
		})
	}
}

// write puts the credentials in this database's own file — the one a
// deployment never touches — and then rebuilds the file the units read.
func (p *Planner) write(d *entities.Database, lines []string) string {
	return "umask 077\n" +
		"cat > " + d.EnvFile() + " <<ENV\n" +
		strings.Join(lines, "\n") + "\n" +
		"ENV\n" +
		regenerate
}

// ── Removing one ──────────────────────────────────────────────────────────────

// Destroy drops the data.
//
// The snapshot is first and carries a kind that pruning never reaches, because
// everything below this line is irreversible and it is the data itself. The
// annotations go last: a failure halfway then leaves a database the panel
// still knows about, which can be looked at, rather than one nobody remembers
// owning.
func (p *Planner) Destroy(d *entities.Database, at time.Time, keys, remaining, services []string) plan.Plan {
	steps := []plan.Step{
		plan.Command("Take a snapshot, in case this was a mistake",
			p.bin, "snapshot", p.container, d.FarewellName(at)),
	}

	if d.Engine.Credentialed() {
		steps = append(steps, p.exec(
			"Drop "+d.DB+" and the user "+d.User+" — this is the data itself", p.drop(d)))
	}

	steps = append(steps,
		p.exec("Remove "+d.EnvFile()+" and rebuild "+entities.EnvPath,
			"set -e\nrm -f "+d.EnvFile()+"\n"+regenerate))

	for _, service := range services {
		steps = append(steps, plan.Optional(
			"Restart croft-"+service+", which no longer has these credentials",
			p.bin, "exec", p.container, "--", "systemctl", "restart", "croft-"+service))
	}

	for _, key := range keys {
		steps = append(steps, plan.Optional("Forget "+key,
			p.bin, "config", "unset", p.container, key))
	}

	if len(remaining) > 0 {
		steps = append(steps, plan.Command(
			"Leave "+strings.Join(remaining, ", ")+" in the index",
			p.bin, "config", "set", p.container, entities.IndexKey, strings.Join(remaining, " ")))
	} else {
		steps = append(steps, plan.Optional("Empty the index",
			p.bin, "config", "unset", p.container, entities.IndexKey))
	}

	return plan.New(steps...)
}

func (p *Planner) drop(d *entities.Database) string {
	switch d.Engine {
	case enums.EnginePostgres:
		return "set -e\n" +
			"su -s /bin/sh postgres -c 'psql -v ON_ERROR_STOP=1' <<SQL\n" +
			"DROP DATABASE IF EXISTS " + d.DB + ";\n" +
			"DROP USER IF EXISTS " + d.User + ";\n" +
			"SQL"
	default:
		return "set -e\n" +
			"mariadb -e \"DROP DATABASE IF EXISTS " + d.DB + "; " +
			"DROP USER IF EXISTS '" + d.User + "'@'localhost';\""
	}
}
