package agent

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"regexp"
	"strings"

	deployEntities "github.com/Hyzokaaa/opencroft/internal/deploy/domain/entities"
	"github.com/Hyzokaaa/opencroft/internal/shared/host"
	"github.com/Hyzokaaa/opencroft/internal/shared/plan"
)

// Adopting is how croft takes on a service it found running rather than
// deployed. Everything it records is read off the container — the unit says
// where the code lives and who runs it, the checkout says where it came from —
// and only how to build it is asked, because nothing on the machine says so.
// The person's own work is left as it was: the unit and the environment file
// are theirs, and stay theirs.

var userPattern = regexp.MustCompile(`^[a-z_][a-z0-9_-]{0,31}$`)

// AdoptionDTO is what croft can tell about a unit before taking it on: the
// facts it will record, and a proposal for what it cannot know.
type AdoptionDTO struct {
	Unit    string `json:"unit"`
	Path    string `json:"path"`
	RunAs   string `json:"runAs"`
	EnvFile string `json:"envFile"`
	// Runs is the unit's command, for recognising it — croft never changes it.
	Runs string `json:"runs"`

	Repo   string `json:"repo"`
	Branch string `json:"branch"`
	Commit string `json:"commit"`
	// Changed are tracked files edited in place. A deployment resets the
	// checkout to the branch, so these are what it would discard — said
	// before, not discovered after.
	Changed []string `json:"changed"`

	Runtime string   `json:"runtime"`
	Why     string   `json:"why"`
	Install []string `json:"install"`
	Build   []string `json:"build"`
	Port    int      `json:"port"`

	// Problem is why this unit cannot be taken on, when it cannot.
	Problem string `json:"problem,omitempty"`
}

// AdoptDTO is the part a person answers: a name, and how to build it.
type AdoptDTO struct {
	Name     string    `json:"name"`
	Install  []string  `json:"install"`
	Build    []string  `json:"build"`
	Packages []string  `json:"packages"`
	Port     int       `json:"port"`
	Health   HealthDTO `json:"health"`
}

// inspectUnit reads what adopting a unit would record. It changes nothing.
func (s *Server) inspectUnit(ctx context.Context, name, unit string) (AdoptionDTO, error) {
	config, err := s.instances.Annotations(ctx, name)
	if err != nil {
		return AdoptionDTO{}, err
	}
	taken := map[string]bool{}
	for _, service := range stored(config) {
		if adoption := adoptionFrom(config, service); adoption != nil {
			taken[adoption.Unit] = true
		}
	}
	if taken[unit] || !contains(s.discoverUnits(ctx, name), unit) {
		return AdoptionDTO{}, errors.New("there is no unit called " + unit + " on " + name + " for croft to take on")
	}

	found := AdoptionDTO{Unit: unit}
	shown := s.unitProperties(ctx, name, unit)
	found.Path = shown["WorkingDirectory"]
	found.RunAs = shown["User"]
	found.EnvFile = firstFile(shown["EnvironmentFiles"])
	found.Runs = argv(shown["ExecStart"])

	if !pathPattern.MatchString(found.Path) {
		found.Problem = "the unit does not say where it runs (no WorkingDirectory=), so croft cannot tell which checkout is its code"
		return found, nil
	}
	if found.RunAs != "" && !userPattern.MatchString(found.RunAs) {
		found.Problem = "the unit runs as " + found.RunAs + ", which is not a user name croft can hand files to"
		return found, nil
	}

	git := func(args ...string) string {
		argv := append([]string{"exec", name, "--", "git", "-c", "safe.directory=" + found.Path, "-C", found.Path}, args...)
		out, err := s.host.Run(ctx, s.bin, argv...)
		if err != nil {
			return ""
		}
		return strings.TrimSpace(out.Stdout)
	}
	found.Repo = git("remote", "get-url", "origin")
	found.Branch = git("rev-parse", "--abbrev-ref", "HEAD")
	found.Commit = git("rev-parse", "HEAD")
	found.Changed = strings.Fields(git("diff", "--name-only", "HEAD"))

	switch {
	case found.Repo == "":
		found.Problem = found.Path + " is not a git checkout with an origin, so there is nothing to fetch a new version from"
	case !repoPattern.MatchString(found.Repo):
		found.Problem = "its origin is " + found.Repo + ", which is not a public https repository — deploy keys are not built yet"
	case !branchPattern.MatchString(found.Branch) || found.Branch == "HEAD":
		found.Problem = "the checkout is not on a branch, so there is no branch to follow"
	}

	_, detection := s.look(ctx, name, &deployEntities.Service{Path: found.Path})
	detection = detection.ForRunning()
	found.Runtime, found.Why = detection.Runtime, detection.Why
	found.Install, found.Build, found.Port = detection.Install, detection.Build, detection.Port

	return found, nil
}

// unitProperties asks systemd for the handful of things adoption records, in
// the form systemd itself resolved — not a parse of the unit file.
func (s *Server) unitProperties(ctx context.Context, name, unit string) map[string]string {
	out, _ := s.host.Run(ctx, s.bin, "exec", name, "--", "systemctl", "show", unit, "--no-pager",
		"-p", "WorkingDirectory", "-p", "User", "-p", "EnvironmentFiles", "-p", "ExecStart")

	properties := map[string]string{}
	for _, line := range strings.Split(out.Stdout, "\n") {
		if key, value, ok := strings.Cut(line, "="); ok {
			if _, seen := properties[key]; !seen {
				properties[key] = strings.TrimSpace(value)
			}
		}
	}
	return properties
}

// firstFile turns "/srv/app/.env (ignore_errors=no)" into the path.
func firstFile(files string) string {
	path, _, _ := strings.Cut(files, " ")
	return path
}

// argv pulls the command out of systemd's "{ path=… ; argv[]=… ; … }".
func argv(execStart string) string {
	_, rest, found := strings.Cut(execStart, "argv[]=")
	if !found {
		return ""
	}
	command, _, _ := strings.Cut(rest, " ;")
	return strings.TrimSpace(command)
}

// adoption turns what was found and what was answered into the service croft
// will record. Every field is checked the way a deployment's is, because from
// here on it is one.
func (s *Server) adoption(ctx context.Context, name, unit string, answer AdoptDTO) (*deployEntities.Service, []string, error) {
	found, err := s.inspectUnit(ctx, name, unit)
	if err != nil {
		return nil, nil, err
	}
	if found.Problem != "" {
		return nil, nil, errors.New(found.Problem)
	}

	config, err := s.instances.Annotations(ctx, name)
	if err != nil {
		return nil, nil, err
	}
	index := stored(config)
	if contains(index, answer.Name) {
		return nil, nil, errors.New(name + " already has a service called " + answer.Name)
	}

	service, err := ServiceDTO{
		Name: answer.Name, Repo: found.Repo, Branch: found.Branch, Commit: found.Commit,
		Path: found.Path, Runtime: found.Runtime, Port: answer.Port,
		Install: answer.Install, Build: answer.Build, Packages: answer.Packages,
		Health: answer.Health,
	}.entity()
	if err != nil {
		return nil, nil, err
	}
	service.Adopted = &deployEntities.Adoption{Unit: unit, RunAs: found.RunAs, EnvFile: found.EnvFile}

	return service, append(index, answer.Name), nil
}

func (s *Server) acceptAdoption(r *http.Request) (string, string, AdoptDTO, error) {
	name, unit, err := s.acceptUnit(r)
	if err != nil {
		return "", "", AdoptDTO{}, err
	}
	var answer AdoptDTO
	if err := json.NewDecoder(r.Body).Decode(&answer); err != nil {
		return "", "", AdoptDTO{}, err
	}
	return name, unit, answer, nil
}

func (s *Server) adoptionPlan(ctx context.Context, name, unit string, answer AdoptDTO) (plan.Plan, error) {
	service, index, err := s.adoption(ctx, name, unit, answer)
	if err != nil {
		return plan.Plan{}, err
	}
	planner, err := s.planner(ctx, name)
	if err != nil {
		return plan.Plan{}, err
	}
	return planner.Adopt(service, index), nil
}

// ── Handlers ──────────────────────────────────────────────────────────────────

func (s *Server) showAdoption(w http.ResponseWriter, r *http.Request) {
	name, unit, err := s.acceptUnit(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	found, err := s.inspectUnit(r.Context(), name, unit)
	if err != nil {
		writeError(w, http.StatusNotFound, err)
		return
	}
	writeJSON(w, http.StatusOK, found)
}

func (s *Server) planAdopt(w http.ResponseWriter, r *http.Request) {
	name, unit, answer, err := s.acceptAdoption(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	p, err := s.adoptionPlan(r.Context(), name, unit, answer)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, PlanResponse{Plan: p})
}

func (s *Server) adopt(w http.ResponseWriter, r *http.Request) {
	name, unit, answer, err := s.acceptAdoption(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	ctx := r.Context()
	p, err := s.adoptionPlan(ctx, name, unit, answer)
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
