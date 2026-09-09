package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	instanceCommands "github.com/Hyzokaaa/opencroft/internal/instance/application/commands"
	instanceEntities "github.com/Hyzokaaa/opencroft/internal/instance/domain/entities"
	instanceServices "github.com/Hyzokaaa/opencroft/internal/instance/domain/services"
	"github.com/Hyzokaaa/opencroft/internal/shared/plan"
)

// Every write answers two questions with the same input: what would you do,
// and then do it. `?plan=1` asks the first. The plan the user approves is the
// list the job walks — they cannot disagree, because they are one list.
func wantsPlan(r *http.Request) bool {
	return r.URL.Query().Get("plan") == "1"
}

func (d Deps) createInstance(w http.ResponseWriter, r *http.Request) {
	if d.ReadOnly {
		writeError(w, http.StatusForbidden, errors.New("this instance is read-only"))
		return
	}

	var body instanceCommands.CreateInstanceRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	instance, p, err := d.CreateInstance.Prepare(r.Context(), instanceServices.CreateInstanceProps{
		Name:     body.Name,
		Image:    body.Image,
		Port:     body.Port,
		CPULimit: body.CPULimit,
		MemLimit: body.MemLimit,
	})
	if err != nil {
		writeError(w, statusFor(err), err)
		return
	}

	if wantsPlan(r) {
		writeJSON(w, http.StatusOK, map[string]any{
			"summary": fmt.Sprintf("Create %s at %s", instance.Name, instance.Address),
			"plan":    p,
		})
		return
	}

	started := d.Jobs.Start("create", instance.Name, p, func(ctx context.Context, report func(int, string)) error {
		if d.Simulated {
			// The plan is theatre here, so the container is recorded
			// separately for the panel to show it.
			if err := d.rehearse(p, report); err != nil {
				return err
			}
			return d.Instances.Create(ctx, instance)
		}

		if narrator, ok := d.Instances.(progressWriter); ok {
			return narrator.CreateWithProgress(ctx, instance, report)
		}
		return d.Instances.Create(ctx, instance)
	})

	writeJSON(w, http.StatusAccepted, map[string]string{"jobId": started.Id, "name": instance.Name})
}

func (d Deps) destroyInstance(w http.ResponseWriter, r *http.Request) {
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
		writeError(w, http.StatusNotFound, instanceServices.ErrNotFound)
		return
	}

	p := d.Instances.DeletePlan(name)

	if wantsPlan(r) {
		summary := fmt.Sprintf("Destroy %s. This cannot be undone.", name)
		if !existing.Managed {
			summary = fmt.Sprintf("Destroy %s — which OpenCroft did not create. This cannot be undone.", name)
		}
		writeJSON(w, http.StatusOK, map[string]any{"summary": summary, "plan": p})
		return
	}

	started := d.Jobs.Start("destroy", name, p, func(ctx context.Context, report func(int, string)) error {
		if d.Simulated {
			if err := d.rehearse(p, report); err != nil {
				return err
			}
			return d.Instances.Delete(ctx, name)
		}

		if narrator, ok := d.Instances.(progressWriter); ok {
			return narrator.DeleteWithProgress(ctx, name, report)
		}
		return d.Instances.Delete(ctx, name)
	})

	writeJSON(w, http.StatusAccepted, map[string]string{"jobId": started.Id, "name": name})
}

// progressWriter is what a repository offers when it can narrate its work —
// the agent client does, by forwarding what the privileged side reports.
type progressWriter interface {
	CreateWithProgress(ctx context.Context, instance *instanceEntities.Instance, report func(int, string)) error
	DeleteWithProgress(ctx context.Context, name string, report func(int, string)) error
}

// rehearse walks the plan without doing anything, for demo mode.
func (d Deps) rehearse(p plan.Plan, report func(int, string)) error {
	for i, step := range p.Steps {
		report(i+1, step.Describe)
		// Long enough to see the plan advance, short enough not to annoy.
		time.Sleep(400 * time.Millisecond)
	}
	return nil
}

func (d Deps) showJob(w http.ResponseWriter, r *http.Request) {
	found, ok := d.Jobs.Find(r.PathValue("id"))
	if !ok {
		writeError(w, http.StatusNotFound, errors.New("no such job"))
		return
	}
	writeJSON(w, http.StatusOK, found)
}

// streamJob pushes progress over server-sent events. The work belongs to the
// daemon, so closing this stream does not stop anything.
func (d Deps) streamJob(w http.ResponseWriter, r *http.Request) {
	events, unsubscribe, ok := d.Jobs.Subscribe(r.PathValue("id"))
	if !ok {
		writeError(w, http.StatusNotFound, errors.New("no such job"))
		return
	}
	defer unsubscribe()

	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, errors.New("streaming is not supported here"))
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	for {
		select {
		case <-r.Context().Done():
			return
		case event, open := <-events:
			if !open {
				fmt.Fprint(w, "event: end\ndata: {}\n\n")
				flusher.Flush()
				return
			}
			payload, _ := json.Marshal(event)
			fmt.Fprintf(w, "data: %s\n\n", payload)
			flusher.Flush()
		}
	}
}
