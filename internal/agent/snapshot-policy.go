package agent

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/Hyzokaaa/opencroft/internal/shared/host"
	"github.com/Hyzokaaa/opencroft/internal/shared/plan"
)

// A service can be deployed without a snapshot first. On a `dir` pool every
// snapshot is a full copy of the container, and writing one out has frozen a
// whole host; for a test environment, losing the rollback costs less.
//
// It is not one of the configurable properties: it changes nothing that runs,
// so saving it must not mark the service as having changes not deployed. It
// takes effect at the next write that would have taken one.

const snapshotKey = "snapshot"

// snapshotOff is the only value written. The key's absence means snapshots are
// taken, so every service deployed before this existed keeps taking them.
const snapshotOff = "off"

type SnapshotPolicyDTO struct {
	Enabled bool `json:"enabled"`
}

func (s *Server) noSnapshot(ctx context.Context, container, service string) bool {
	return s.get(ctx, container, full(service, snapshotKey)) == snapshotOff
}

func (s *Server) snapshotPolicyPlan(r *http.Request) (string, plan.Plan, error) {
	name, service := r.PathValue("name"), r.PathValue("service")
	if err := validName(name); err != nil {
		return "", plan.Plan{}, err
	}
	if err := validService(service); err != nil {
		return "", plan.Plan{}, err
	}

	var body SnapshotPolicyDTO
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		return "", plan.Plan{}, errors.New("the request did not say whether to take snapshots")
	}

	config, err := s.instances.Annotations(r.Context(), name)
	if err != nil {
		return "", plan.Plan{}, err
	}
	if !contains(stored(config), service) {
		return "", plan.Plan{}, errors.New("there is no service " + service + " in " + name)
	}

	off := config[full(service, snapshotKey)] == snapshotOff
	switch {
	case body.Enabled && off:
		return name, plan.New(plan.Command("Take a snapshot before every deployment of "+service+" again",
			s.bin, "config", "unset", name, full(service, snapshotKey))), nil
	case !body.Enabled && !off:
		return name, plan.New(plan.Command("Deploy "+service+" without taking a snapshot first",
			s.bin, "config", "set", name, full(service, snapshotKey), snapshotOff)), nil
	}
	return "", plan.Plan{}, errors.New("nothing would change")
}

func (s *Server) planSnapshotPolicy(w http.ResponseWriter, r *http.Request) {
	_, p, err := s.snapshotPolicyPlan(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, PlanResponse{Plan: p})
}

func (s *Server) setSnapshotPolicy(w http.ResponseWriter, r *http.Request) {
	name, p, err := s.snapshotPolicyPlan(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	ctx := r.Context()
	s.streamOn(w, r, name, func(report func(int, string)) error {
		for i, step := range p.Steps {
			report(i+1, step.Describe)
			if err := host.RunStep(ctx, s.host, step); err != nil {
				return errors.New(strings.TrimSpace(step.Describe + ": " + err.Error()))
			}
		}
		return nil
	})
}
