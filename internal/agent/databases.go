package agent

import (
	"context"
	"errors"
	"net/http"
	"time"

	databaseEntities "github.com/Hyzokaaa/opencroft/internal/database/domain/entities"
	databaseServices "github.com/Hyzokaaa/opencroft/internal/database/domain/services"
	databaseContainer "github.com/Hyzokaaa/opencroft/internal/database/infrastructure/container"
	"github.com/Hyzokaaa/opencroft/internal/shared/host"
	"github.com/Hyzokaaa/opencroft/internal/shared/plan"
)

// A database lives inside the container that uses it.
//
// That is the whole reason this module exists: because the data is in there,
// `lxc snapshot` captures an application and its database at the same instant.
// Two containers would be two snapshots that are not consistent with each
// other, which is the problem every PaaS built on images has.
//
// The password is the one thing that never crosses this socket. It is
// generated inside the container by the step that uses it, written to a file
// only root can read, and never recorded as an annotation — because the panel
// reads annotations and serves them to a browser.

// ── What crosses the socket ───────────────────────────────────────────────────

type DatabaseDTO struct {
	Name     string `json:"name"`
	Engine   string `json:"engine"`
	DB       string `json:"db"`
	User     string `json:"user"`
	Port     int    `json:"port"`
	Location string `json:"location,omitempty"`

	// State is read from the container, not remembered. The machine is the
	// source of truth about whether the engine is running.
	State string `json:"state,omitempty"`
}

type DatabasesResponse struct {
	Databases []DatabaseDTO `json:"databases"`
}

func toDatabaseDTO(d *databaseEntities.Database) DatabaseDTO {
	return DatabaseDTO{
		Name: d.Name, Engine: string(d.Engine), DB: d.DB, User: d.User,
		Port: d.Port, Location: d.Location,
	}
}

// ── Reading ───────────────────────────────────────────────────────────────────

// listDatabases answers the whole card in two calls: one for the annotations,
// one for every engine's state at once.
func (s *Server) listDatabases(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if err := validName(name); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	ctx := r.Context()
	config, err := s.instances.Annotations(ctx, name)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	found := databaseServices.All(config)
	out := make([]DatabaseDTO, 0, len(found))
	units := make([]string, 0, len(found))

	for _, database := range found {
		out = append(out, toDatabaseDTO(database))
		units = append(units, databaseContainer.Unit(database.Engine))
	}

	// An engine living in another container is not this container's to report
	// on, so it is asked about where it actually runs.
	for i, state := range s.states(ctx, name, units) {
		if found[i].Local() {
			out[i].State = state
		}
	}

	writeJSON(w, http.StatusOK, DatabasesResponse{Databases: out})
}

// ── Provisioning ──────────────────────────────────────────────────────────────

// acceptDatabase turns the request into something the agent is willing to act
// on. Everything is checked again here: the API validates too, but the agent
// cannot assume the API is the one calling.
func (s *Server) acceptDatabase(r *http.Request) (string, *databaseEntities.Database, []string, error) {
	name := r.PathValue("name")
	if err := validName(name); err != nil {
		return "", nil, nil, err
	}

	var dto DatabaseDTO
	if err := decode(r, &dto); err != nil {
		return "", nil, nil, err
	}

	config, err := s.instances.Annotations(r.Context(), name)
	if err != nil {
		return "", nil, nil, err
	}

	database, index, err := databaseServices.NewProvisionDatabase().Execute(
		databaseServices.ProvisionProps{
			Name: dto.Name, Engine: dto.Engine, DB: dto.DB,
			User: dto.User, Location: dto.Location,
		}, config)
	if err != nil {
		return "", nil, nil, err
	}

	if !database.Local() {
		return "", nil, nil, errors.New(
			"a database in another container is not built yet — leave it here, " +
				"where a snapshot of this container brings the data back with it")
	}

	return name, database, index, nil
}

func (s *Server) provisionPlan(
	ctx context.Context, container string, database *databaseEntities.Database, index []string,
) plan.Plan {
	planner := databaseContainer.NewPlanner(s.bin, container)
	return planner.Provision(database, time.Now(), index, s.deployed(ctx, container))
}

// deployed is the services this container runs, which are what has to be
// restarted once the credentials exist.
func (s *Server) deployed(ctx context.Context, container string) []string {
	config, err := s.instances.Annotations(ctx, container)
	if err != nil {
		return nil
	}
	return stored(config)
}

func (s *Server) planProvision(w http.ResponseWriter, r *http.Request) {
	container, database, index, err := s.acceptDatabase(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	writeJSON(w, http.StatusOK, PlanResponse{
		Plan: s.provisionPlan(r.Context(), container, database, index),
	})
}

// provision walks the very plan that planProvision returned.
func (s *Server) provision(w http.ResponseWriter, r *http.Request) {
	container, database, index, err := s.acceptDatabase(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	ctx := r.Context()
	steps := s.provisionPlan(ctx, container, database, index).Steps

	s.streamOn(w, r, container, func(report func(int, string)) error {
		for i, step := range steps {
			report(i+1, step.Describe)

			if err := host.RunStep(ctx, s.host, step); err != nil {
				if step.Optional {
					continue
				}
				return explain(step, err)
			}
		}
		return nil
	})
}

// ── Removing one ──────────────────────────────────────────────────────────────

func (s *Server) destroyDatabasePlan(
	ctx context.Context, container, name string,
) (plan.Plan, string, error) {
	if err := validName(container); err != nil {
		return plan.Plan{}, "", err
	}

	config, err := s.instances.Annotations(ctx, container)
	if err != nil {
		return plan.Plan{}, "", err
	}

	database, removal, err := databaseServices.NewDestroyDatabase().Execute(config, name)
	if err != nil {
		return plan.Plan{}, "", err
	}

	services := stored(config)
	planner := databaseContainer.NewPlanner(s.bin, container)

	return planner.Destroy(database, time.Now(), removal.Keys, removal.Remaining, services),
		databaseServices.Consequences(database, services), nil
}

func (s *Server) planDestroyDatabase(w http.ResponseWriter, r *http.Request) {
	p, warning, err := s.destroyDatabasePlan(
		r.Context(), r.PathValue("name"), r.PathValue("database"))
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, RollbackResponse{Plan: p, Warning: warning})
}

func (s *Server) destroyDatabase(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	p, _, err := s.destroyDatabasePlan(ctx, r.PathValue("name"), r.PathValue("database"))
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	s.streamOn(w, r, r.PathValue("name"), func(report func(int, string)) error {
		for i, step := range p.Steps {
			report(i+1, step.Describe)

			if err := host.RunStep(ctx, s.host, step); err != nil && !step.Optional {
				return explain(step, err)
			}
		}
		return nil
	})
}
