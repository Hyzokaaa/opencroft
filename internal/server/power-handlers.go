package server

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/Hyzokaaa/opencroft/internal/shared/plan"
)

// powerWriter is offered by a repository that can narrate starting and
// stopping — the agent client does, by forwarding what the privileged side
// reports.
type powerWriter interface {
	StartWithProgress(ctx context.Context, name string, report func(int, string)) error
	StopWithProgress(ctx context.Context, name string, report func(int, string)) error
}

func (d Deps) startInstance(w http.ResponseWriter, r *http.Request) { d.power(w, r, false) }
func (d Deps) stopInstance(w http.ResponseWriter, r *http.Request)  { d.power(w, r, true) }

func (d Deps) power(w http.ResponseWriter, r *http.Request, stop bool) {
	if d.ReadOnly {
		writeError(w, http.StatusForbidden, errors.New("this instance is read-only"))
		return
	}

	name := r.PathValue("name")
	existing, err := d.Instances.FindByName(r.Context(), name)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	if existing == nil {
		writeError(w, http.StatusNotFound, errors.New("no container by that name"))
		return
	}

	var p plan.Plan
	if stop {
		p = d.Instances.StopPlan(name)
	} else {
		p = d.Instances.StartPlan(name)
	}

	if wantsPlan(r) {
		summary := fmt.Sprintf("Start %s.", name)
		if stop {
			// Worth saying: the domain keeps pointing here and will answer 502.
			summary = fmt.Sprintf("Stop %s. Anything served from it goes down until it is started again.", name)
		}
		writeJSON(w, http.StatusOK, map[string]any{"summary": summary, "plan": p})
		return
	}

	kind := "start"
	if stop {
		kind = "stop"
	}

	started := d.Jobs.Start(kind, name, p, func(ctx context.Context, report func(int, string)) error {
		if d.Simulated {
			if err := d.rehearse(p, report); err != nil {
				return err
			}
			if stop {
				return d.Instances.Stop(ctx, name)
			}
			return d.Instances.Start(ctx, name)
		}

		if narrator, ok := d.Instances.(powerWriter); ok {
			if stop {
				return narrator.StopWithProgress(ctx, name, report)
			}
			return narrator.StartWithProgress(ctx, name, report)
		}

		for i, step := range p.Steps {
			report(i+1, step.Describe)
		}
		if stop {
			return d.Instances.Stop(ctx, name)
		}
		return d.Instances.Start(ctx, name)
	})

	writeJSON(w, http.StatusAccepted, map[string]string{"jobId": started.Id, "name": name})
}
