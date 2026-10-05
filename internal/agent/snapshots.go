package agent

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	databaseEntities "github.com/Hyzokaaa/opencroft/internal/database/domain/entities"
	deployEntities "github.com/Hyzokaaa/opencroft/internal/deploy/domain/entities"
	"github.com/Hyzokaaa/opencroft/internal/shared/host"
	"github.com/Hyzokaaa/opencroft/internal/shared/plan"
	"github.com/Hyzokaaa/opencroft/internal/shared/snapshot"
)

// Removing a snapshot by hand. Pruning only ever reaches the deployment
// snapshots croft takes and keeps a few of; everything else — taken before a
// removal, before a database, or by a person — stays until somebody decides
// otherwise. This is that decision, with what it costs said before it is made.

// snapshotCost is what stops being possible once the snapshot is gone, worked
// out from its name and from what the container records.
func snapshotCost(config map[string]string, services []string, name string) string {
	said := []string{}
	for _, service := range services {
		if config[full(service, "healthy")] == name {
			said = append(said, "it is the last version of "+service+" known to work, so going back to one stops being possible until the next deployment that passes its check")
		}
	}
	switch {
	case strings.HasPrefix(name, deployEntities.DestroyKind):
		said = append(said, "it was taken just before a service was removed, and it is the only way back to it")
	case strings.HasPrefix(name, databaseEntities.FarewellKind):
		said = append(said, "it was taken just before a database was dropped, and it is the only way back to that data")
	case strings.HasPrefix(name, databaseEntities.ProvisionKind):
		said = append(said, "it was taken before a database was added: the container as it was without it")
	case !snapshot.Ours(name):
		said = append(said, "croft did not take it — somebody did, by hand, and may be counting on it")
	}
	if len(said) == 0 {
		return "Nothing else depends on it."
	}
	return "Once it is gone " + strings.Join(said, "; ") + "."
}

func (s *Server) snapshotRemoval(ctx context.Context, name, wanted string) (plan.Plan, string, error) {
	if err := validName(name); err != nil {
		return plan.Plan{}, "", err
	}
	if wanted == "" || strings.ContainsAny(wanted, " \t\n;|&$`/") {
		return plan.Plan{}, "", errors.New("that is not a snapshot name")
	}
	if !containsSnapshot(s.snapshots(ctx, name), wanted) {
		return plan.Plan{}, "", errors.New("there is no snapshot called " + wanted + " on " + name)
	}

	config, err := s.instances.Annotations(ctx, name)
	if err != nil {
		return plan.Plan{}, "", err
	}
	services := stored(config)

	steps := []plan.Step{plan.Command("Delete the snapshot "+wanted+" of "+name, s.bin, "delete", name+"/"+wanted)}
	// What pointed at it stops pointing at something that is not there.
	for _, service := range services {
		if config[full(service, "healthy")] == wanted {
			steps = append(steps, plan.Command(fmt.Sprintf("Forget that %s was the last version of %s known to work", wanted, service),
				s.bin, "config", "unset", name, full(service, "healthy")))
		}
	}
	return plan.New(steps...), snapshotCost(config, services, wanted), nil
}

func (s *Server) planRemoveSnapshot(w http.ResponseWriter, r *http.Request) {
	p, warning, err := s.snapshotRemoval(r.Context(), r.PathValue("name"), r.PathValue("snapshot"))
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, RollbackResponse{Plan: p, Warning: warning})
}

func (s *Server) removeSnapshot(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	name := r.PathValue("name")
	p, _, err := s.snapshotRemoval(ctx, name, r.PathValue("snapshot"))
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	s.streamOn(w, r, name, func(report func(int, string)) error {
		for i, step := range p.Steps {
			report(i+1, step.Describe)
			if err := host.RunStep(ctx, s.host, step); err != nil {
				return fmt.Errorf("%s: %w", step.Describe, err)
			}
		}
		return nil
	})
}
