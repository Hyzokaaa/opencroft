package agent

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	deployEntities "github.com/Hyzokaaa/opencroft/internal/deploy/domain/entities"
	deployServices "github.com/Hyzokaaa/opencroft/internal/deploy/domain/services"
	"github.com/Hyzokaaa/opencroft/internal/shared/host"
	"github.com/Hyzokaaa/opencroft/internal/shared/plan"
)

// A service's environment lives in its file — the one its unit reads, or its
// build — for services croft deployed and ones it adopted alike. The panel
// reads that file and writes it back, and keeps no copy of its own.
//
// Two people can have it open at once: one in the panel, one over ssh. So what
// is read carries a hash of the file, and a change is only written if the file
// still holds exactly that — checked again at the last moment, once it is this
// change's turn on the container. Somebody else's edit is never overwritten.

var errChanged = errors.New(
	"the environment file changed since it was opened — reload it and make the change again")

type EnvVarDTO struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

type EnvironmentDTO struct {
	File   string      `json:"file"`
	Exists bool        `json:"exists"`
	Vars   []EnvVarDTO `json:"vars"`
	Hash   string      `json:"hash"`
	// Applies is how a change takes effect: "restart" for a process, which
	// reads its environment at start, or "rebuild" for a site, whose build
	// read it.
	Applies string `json:"applies"`
}

// EnvChangeDTO is the whole environment as it should be, and the hash of the
// file it was edited from.
type EnvChangeDTO struct {
	Hash string            `json:"hash"`
	Vars map[string]string `json:"vars"`
}

// environmentOf is a stored service and the file its environment is in.
func (s *Server) environmentOf(r *http.Request) (string, *deployEntities.Service, string, error) {
	name, service, err := s.stored(r)
	if err != nil {
		return "", nil, "", err
	}

	file := service.Environment()
	if file == "" {
		return "", nil, "", errors.New(service.Name + " runs the way its unit " + service.Unit() +
			" says, and the unit reads no environment file — give it an EnvironmentFile= to manage one here")
	}
	// An adopted unit's file comes from what systemd said, and it is about
	// to reach a command line.
	if !pathPattern.MatchString(file) {
		return "", nil, "", errors.New(file + " is not a path croft can manage")
	}
	return name, service, file, nil
}

func (s *Server) readEnvironment(ctx context.Context, name, file string) (string, bool, error) {
	if !s.exists(ctx, name, file) {
		return "", false, nil
	}
	out, err := s.host.Run(ctx, s.bin, "exec", name, "--", "cat", file)
	if err != nil {
		return "", true, err
	}
	return out.Stdout, true, nil
}

// environmentPlan is the change as it stands now: the file as it is, checked
// against the hash it was edited from, with only what was asked made different.
func (s *Server) environmentPlan(
	ctx context.Context, name string, service *deployEntities.Service, file, hash string, want map[string]string,
) (plan.Plan, error) {
	content, _, err := s.readEnvironment(ctx, name, file)
	if err != nil {
		return plan.Plan{}, err
	}
	if deployServices.EnvHash(content) != hash {
		return plan.Plan{}, errChanged
	}

	next := deployServices.EditEnv(content, want)
	if strings.TrimRight(next, "\n") == strings.TrimRight(content, "\n") {
		return plan.Plan{}, errors.New("nothing in the environment changed")
	}

	planner, err := s.planner(ctx, name)
	if err != nil {
		return plan.Plan{}, err
	}
	return planner.ChangeEnvironment(service, file, next, len(want), time.Now()), nil
}

func (s *Server) acceptEnvironmentChange(r *http.Request) (string, *deployEntities.Service, string, EnvChangeDTO, map[string]string, error) {
	name, service, file, err := s.environmentOf(r)
	if err != nil {
		return "", nil, "", EnvChangeDTO{}, nil, err
	}
	var change EnvChangeDTO
	if err := json.NewDecoder(r.Body).Decode(&change); err != nil {
		return "", nil, "", EnvChangeDTO{}, nil, err
	}
	want, err := environment(change.Vars)
	if err != nil {
		return "", nil, "", EnvChangeDTO{}, nil, err
	}
	return name, service, file, change, want, nil
}

// ── Handlers ──────────────────────────────────────────────────────────────────

func (s *Server) showEnvironment(w http.ResponseWriter, r *http.Request) {
	name, service, file, err := s.environmentOf(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	content, exists, err := s.readEnvironment(r.Context(), name, file)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	vars := []EnvVarDTO{}
	for _, v := range deployServices.ReadEnv(content) {
		vars = append(vars, EnvVarDTO{Key: v.Key, Value: v.Value})
	}
	applies := "restart"
	if service.IsSite() {
		applies = "rebuild"
	}
	writeJSON(w, http.StatusOK, EnvironmentDTO{
		File: file, Exists: exists, Vars: vars, Hash: deployServices.EnvHash(content), Applies: applies,
	})
}

func (s *Server) planEnvironment(w http.ResponseWriter, r *http.Request) {
	name, service, file, change, want, err := s.acceptEnvironmentChange(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	p, err := s.environmentPlan(r.Context(), name, service, file, change.Hash, want)
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, errChanged) {
			status = http.StatusConflict
		}
		writeError(w, status, err)
		return
	}
	writeJSON(w, http.StatusOK, PlanResponse{Plan: p})
}

func (s *Server) changeEnvironment(w http.ResponseWriter, r *http.Request) {
	name, service, file, change, want, err := s.acceptEnvironmentChange(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	ctx := r.Context()
	s.streamOn(w, r, name, func(report func(int, string)) error {
		p, err := s.environmentPlan(ctx, name, service, file, change.Hash, want)
		if err != nil {
			return err
		}
		for i, step := range p.Steps {
			report(i+1, step.Describe)
			if err := host.RunStep(ctx, s.host, step); err != nil {
				return explain(step, err)
			}
		}
		return nil
	})
}
