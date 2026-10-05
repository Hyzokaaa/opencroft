package agent

import (
	"context"
	"net/http"

	databaseEntities "github.com/Hyzokaaa/opencroft/internal/database/domain/entities"
	databaseServices "github.com/Hyzokaaa/opencroft/internal/database/domain/services"
	databaseContainer "github.com/Hyzokaaa/opencroft/internal/database/infrastructure/container"
	"github.com/Hyzokaaa/opencroft/internal/shared/host"
)

// A database somebody set up before croft — the one an install script left
// beside the application — is found by asking the engines that run in the
// container, and taken on by writing down what they said. Nothing in the
// container changes, so the application keeps reaching it exactly as before;
// what changes is that croft now knows it is there: it is listed, a rollback
// warns that it goes back too, and another container of the project can be
// connected to it.

type FoundDatabaseDTO struct {
	Engine string `json:"engine"`
	DB     string `json:"db"`
	Owner  string `json:"owner,omitempty"`
	Port   int    `json:"port"`
}

type AdoptDatabaseDTO struct {
	Engine string `json:"engine"`
	DB     string `json:"db"`
}

// found asks the container's engines which databases they hold. An engine
// that is not installed, or not running, simply answers nothing.
func (s *Server) found(ctx context.Context, container string) ([]databaseServices.Found, error) {
	argv := databaseContainer.LookCommand(s.bin, container)
	out, err := s.host.Run(ctx, argv[0], argv[1:]...)
	if err != nil {
		return nil, err
	}
	return databaseServices.ParseFound(out.Stdout), nil
}

func (s *Server) listFoundDatabases(w http.ResponseWriter, r *http.Request) {
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
	found, err := s.found(ctx, name)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	out := []FoundDatabaseDTO{}
	for _, f := range databaseServices.Unrecorded(config, found) {
		out = append(out, FoundDatabaseDTO{Engine: string(f.Engine), DB: f.DB, Owner: f.Owner, Port: f.Port})
	}
	writeJSON(w, http.StatusOK, out)
}

// acceptAdoption finds the database asked for among what the engines report
// now. What the request says about it is only which one: its owner and port
// are read from the engine, never taken from the request.
func (s *Server) acceptAdoption(r *http.Request) (string, *databaseEntities.Database, []string, error) {
	name := r.PathValue("name")
	if err := validName(name); err != nil {
		return "", nil, nil, err
	}
	var dto AdoptDatabaseDTO
	if err := decode(r, &dto); err != nil {
		return "", nil, nil, err
	}

	ctx := r.Context()
	config, err := s.instances.Annotations(ctx, name)
	if err != nil {
		return "", nil, nil, err
	}
	found, err := s.found(ctx, name)
	if err != nil {
		return "", nil, nil, err
	}
	for _, f := range found {
		if string(f.Engine) == dto.Engine && f.DB == dto.DB {
			database, index, err := databaseServices.AdoptDatabase(config, f)
			return name, database, index, err
		}
	}
	return "", nil, nil, databaseServices.ErrNotFoundHere
}

func (s *Server) planAdoptDatabase(w http.ResponseWriter, r *http.Request) {
	name, database, index, err := s.acceptAdoption(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, PlanResponse{Plan: databaseContainer.NewPlanner(s.bin, name).Adopt(database, index)})
}

func (s *Server) adoptDatabase(w http.ResponseWriter, r *http.Request) {
	name, database, index, err := s.acceptAdoption(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	ctx := r.Context()
	steps := databaseContainer.NewPlanner(s.bin, name).Adopt(database, index).Steps
	s.streamOn(w, r, name, func(report func(int, string)) error {
		for i, step := range steps {
			report(i+1, step.Describe)
			if err := host.RunStep(ctx, s.host, step); err != nil {
				return explain(step, err)
			}
		}
		return nil
	})
}

// releaseWarning is what letting an adopted database go means: nothing to the
// data, and only that croft stops knowing about it.
func releaseWarning(d *databaseEntities.Database) string {
	return "croft forgets " + d.DB + ". The database, its data and whatever uses it stay exactly as they are."
}
