package container

import (
	"strings"

	"github.com/Hyzokaaa/opencroft/internal/database/domain/entities"
	"github.com/Hyzokaaa/opencroft/internal/shared/plan"
)

// Look is the one command that asks a container which databases it runs.
// Each engine that is installed and active lists its own, one per line, as
// engine|database|owner|port; system databases are left out. It reads and
// changes nothing, and an engine that is not there prints nothing.
const Look = `if command -v psql >/dev/null 2>&1 && systemctl is-active --quiet postgresql; then
  port=$(su -s /bin/sh postgres -c "psql -tAc 'SHOW port'" 2>/dev/null)
  su -s /bin/sh postgres -c "psql -tA -F'|' -c \"SELECT 'postgres', datname, pg_get_userbyid(datdba), '$port' FROM pg_database WHERE NOT datistemplate AND datname <> 'postgres'\"" 2>/dev/null
fi
if command -v mariadb >/dev/null 2>&1 && systemctl is-active --quiet mariadb; then
  port=$(mariadb -N -e 'SELECT @@port' 2>/dev/null)
  mariadb -N -e "SELECT 'mysql', schema_name, '', '$port' FROM information_schema.schemata WHERE schema_name NOT IN ('mysql','information_schema','performance_schema','sys')" 2>/dev/null | tr '\t' '|'
fi
if command -v redis-cli >/dev/null 2>&1 && systemctl is-active --quiet redis-server; then
  echo "redis|redis||$(redis-cli CONFIG GET port 2>/dev/null | tail -1)"
fi
true`

// LookCommand is Look as the runtime runs it inside a container.
func LookCommand(bin, container string) []string {
	return []string{bin, "exec", container, "--", "sh", "-c", Look}
}

// Adopt takes note of a database that is already running: annotations only.
// Nothing is installed, created or restarted, no credentials are written —
// the application keeps reaching it the way it always did — so there is no
// snapshot to take first: nothing in the container changes.
func (p *Planner) Adopt(d *entities.Database, index []string) plan.Plan {
	steps := []plan.Step{}
	for _, a := range d.Annotations() {
		if a[1] == "" {
			continue
		}
		steps = append(steps, plan.Command("Record "+a[0]+" on the container",
			p.bin, "config", "set", p.container, entities.KeyPrefix+d.Name+"."+a[0], a[1]))
	}
	steps = append(steps, plan.Command("List "+strings.Join(index, ", ")+" in the index",
		p.bin, "config", "set", p.container, entities.IndexKey, strings.Join(index, " ")))
	return plan.New(steps...)
}

// Release forgets an adopted database. The data, the engine and whoever uses
// it are left exactly as they were found: croft never made them.
func (p *Planner) Release(keys, remaining []string) plan.Plan {
	steps := []plan.Step{}
	for _, key := range keys {
		steps = append(steps, plan.Optional("Forget "+key, p.bin, "config", "unset", p.container, key))
	}
	if len(remaining) > 0 {
		steps = append(steps, plan.Command("Leave "+strings.Join(remaining, ", ")+" in the index",
			p.bin, "config", "set", p.container, entities.IndexKey, strings.Join(remaining, " ")))
	} else {
		steps = append(steps, plan.Optional("Empty the index", p.bin, "config", "unset", p.container, entities.IndexKey))
	}
	return plan.New(steps...)
}
