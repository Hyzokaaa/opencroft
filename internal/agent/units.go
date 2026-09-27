package agent

import (
	"context"
	"errors"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/Hyzokaaa/opencroft/internal/deploy/infrastructure/container"
	"github.com/Hyzokaaa/opencroft/internal/route/infrastructure/nginx"
	"github.com/Hyzokaaa/opencroft/internal/shared/host"
	"github.com/Hyzokaaa/opencroft/internal/shared/plan"
)

// A container is the source of truth for what it runs, the same as it is for
// domains and for its own existence. This is what lets croft say something
// about a unit it never deployed — found on disk, worked with, but not
// (yet) adopted.
//
// The signal is where the unit file lives: /etc/systemd/system is where a
// person, or an install script run by hand, writes one — the same place
// croft writes its own. The distribution's own services live under /lib or
// /usr/lib — except the ones a package aliases into /etc/systemd/system as a
// symlink back to /lib (Debian does this for syslog, dbus activation units,
// open-vm-tools and others), which is why a symlink is excluded even though
// it lives in the right place: it was not written there, only pointed
// there. Reserved names (croft-*) are excluded too — those already have a
// home in the services list, and this is only for what does not.

const croftUnitPrefix = "croft-"

var unitPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9@:_.-]{0,127}$`)

func validUnit(name string) error {
	if !unitPattern.MatchString(name) {
		return errors.New("that is not a systemd unit name")
	}
	if strings.HasPrefix(name, croftUnitPrefix) {
		return errors.New(name + " is already a croft service")
	}
	return nil
}

type UnitDTO struct {
	Name  string `json:"name"`
	State string `json:"state"`
}

// PowerAction is the closed set of things croft does to a running unit
// without touching its code. The panel names one; the agent decides the
// command. Both halves register their routes from this list, so neither can
// offer an action the other does not serve.
type PowerAction string

const (
	PowerRestart PowerAction = "restart"
	PowerStop    PowerAction = "stop"
	PowerStart   PowerAction = "start"
)

var PowerActions = []PowerAction{PowerRestart, PowerStop, PowerStart}

var powerPlans = map[PowerAction]func(*container.Planner, string) plan.Plan{
	PowerRestart: (*container.Planner).RestartUnit,
	PowerStop:    (*container.Planner).StopUnit,
	PowerStart:   (*container.Planner).StartUnit,
}

// siteMarker separates the two answers the discovery script gives.
const siteMarker = "#### croft: web server"

// discover names what the container runs that croft did not put there: units
// with a file of their own, and directories its own web server serves. Both
// come from one command, so reading a container costs the same however much
// is in it.
func (s *Server) discover(ctx context.Context, name string) ([]string, []nginx.StaticSite) {
	// The if, rather than a chain of &&, is deliberate: a for loop's exit
	// status is whichever command ran last, and with && that would have been
	// the test on the final file the glob happened to match — turning
	// "the last unit alphabetically was a symlink" into "report nothing at
	// all". An if with no else always exits 0, so the result depends only on
	// what was printed, never on where a skipped entry landed in the sort.
	// Likewise a container without nginx, or with a broken configuration,
	// answers with no sites rather than with a failure.
	out, err := s.host.Run(ctx, s.bin, "exec", name, "--", "sh", "-lc",
		`for f in /etc/systemd/system/*.service; do if [ -f "$f" ] && [ ! -L "$f" ]; then basename "$f" .service; fi; done; `+
			`echo '`+siteMarker+`'; if command -v nginx >/dev/null 2>&1; then nginx -T 2>/dev/null || true; fi`)
	if err != nil {
		return nil, nil
	}

	listed, dump, _ := strings.Cut(out.Stdout, siteMarker)

	units := []string{}
	for _, unit := range strings.Fields(listed) {
		if validUnit(unit) == nil {
			units = append(units, unit)
		}
	}
	sort.Strings(units)

	sites := []nginx.StaticSite{}
	for _, site := range nginx.StaticSites(dump) {
		if pathPattern.MatchString(site.Root) {
			sites = append(sites, site)
		}
	}
	return units, sites
}

func (s *Server) discoverUnits(ctx context.Context, name string) []string {
	units, _ := s.discover(ctx, name)
	return units
}

// ── Power, for any unit ───────────────────────────────────────────────────────

func (s *Server) unitPlan(ctx context.Context, name, unit string, action PowerAction) (plan.Plan, error) {
	build, ok := powerPlans[action]
	if !ok {
		return plan.Plan{}, errors.New("that is not something croft does to a unit")
	}
	planner, err := s.planner(ctx, name)
	if err != nil {
		return plan.Plan{}, err
	}
	return build(planner, unit), nil
}

func (s *Server) writeUnitPlan(w http.ResponseWriter, r *http.Request, name, unit string, action PowerAction) {
	p, err := s.unitPlan(r.Context(), name, unit, action)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, PlanResponse{Plan: p})
}

func (s *Server) runUnitPlan(w http.ResponseWriter, r *http.Request, name, unit string, action PowerAction) {
	ctx := r.Context()
	p, err := s.unitPlan(ctx, name, unit, action)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	s.stream(w, func(report func(int, string)) error {
		for i, step := range p.Steps {
			report(i+1, step.Describe)
			if err := host.RunStep(ctx, s.host, step); err != nil {
				return explain(step, err)
			}
		}
		return nil
	})
}

// ── Handlers ──────────────────────────────────────────────────────────────────

func (s *Server) acceptUnit(r *http.Request) (string, string, error) {
	name := r.PathValue("name")
	if err := validName(name); err != nil {
		return "", "", err
	}
	unit := r.PathValue("unit")
	if err := validUnit(unit); err != nil {
		return "", "", err
	}
	return name, unit, nil
}

func (s *Server) planUnitPower(w http.ResponseWriter, r *http.Request, action PowerAction) {
	name, unit, err := s.acceptUnit(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	s.writeUnitPlan(w, r, name, unit, action)
}

func (s *Server) unitPower(w http.ResponseWriter, r *http.Request, action PowerAction) {
	name, unit, err := s.acceptUnit(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	s.runUnitPlan(w, r, name, unit, action)
}

func (s *Server) unitLogs(w http.ResponseWriter, r *http.Request) {
	name, unit, err := s.acceptUnit(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	lines := 200
	if asked, err := strconv.Atoi(r.URL.Query().Get("lines")); err == nil && asked > 0 && asked <= 2000 {
		lines = asked
	}

	planner, err := s.planner(r.Context(), name)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	step := planner.LogsUnit(unit, lines).Steps[0]
	out, _ := s.host.Run(r.Context(), step.Argv[0], step.Argv[1:]...)
	writeJSON(w, http.StatusOK, LogsResponse{Lines: out.Stdout})
}
