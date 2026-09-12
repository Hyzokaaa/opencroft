package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	instanceCommands "github.com/Hyzokaaa/opencroft/internal/instance/application/commands"
	instanceEntities "github.com/Hyzokaaa/opencroft/internal/instance/domain/entities"
	instanceServices "github.com/Hyzokaaa/opencroft/internal/instance/domain/services"
	routeEnums "github.com/Hyzokaaa/opencroft/internal/route/domain/enums"
	routeServices "github.com/Hyzokaaa/opencroft/internal/route/domain/services"
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
	if err := readBody(r, &body); err != nil {
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

	// nginx buffers proxied responses by default, which holds every event back
	// until the work finishes and leaves the panel showing 0/N throughout. This
	// header turns that off for this response alone, so it works behind a vhost
	// we wrote and behind one somebody else did.
	w.Header().Set("X-Accel-Buffering", "no")
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

// ── Domains ───────────────────────────────────────────────────────────────────

type addRouteRequest struct {
	Domain string `json:"domain"`
	Target string `json:"target"`
	Port   int    `json:"port"`
}

func (d Deps) addRoute(w http.ResponseWriter, r *http.Request) {
	if d.ReadOnly {
		writeError(w, http.StatusForbidden, errors.New("this instance is read-only"))
		return
	}

	var body addRouteRequest
	if err := readBody(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	route, p, err := d.AddRoute.Prepare(r.Context(), routeServices.AddRouteProps{
		Domain: body.Domain,
		Target: body.Target,
		Port:   body.Port,
	})
	if err != nil {
		writeError(w, routeStatusFor(err), err)
		return
	}

	if wantsPlan(r) {
		writeJSON(w, http.StatusOK, map[string]any{
			"summary": fmt.Sprintf("Serve %s from %s:%d over http", route.Domain, route.Target, route.Port),
			"plan":    p,
		})
		return
	}

	started := d.Jobs.Start("route", route.Domain, p, func(ctx context.Context, report func(int, string)) error {
		if d.Simulated {
			if err := d.rehearse(p, report); err != nil {
				return err
			}
			return d.Routes.Write(ctx, route)
		}

		// Writing a vhost is quick; there is no progress worth streaming, so
		// the steps are announced as they are handed over.
		for i, step := range p.Steps {
			report(i+1, step.Describe)
		}
		return d.Routes.Write(ctx, route)
	})

	writeJSON(w, http.StatusAccepted, map[string]string{"jobId": started.Id, "domain": route.Domain})
}

func (d Deps) removeRoute(w http.ResponseWriter, r *http.Request) {
	if d.ReadOnly {
		writeError(w, http.StatusForbidden, errors.New("this instance is read-only"))
		return
	}

	domain := r.PathValue("domain")
	existing, err := d.Routes.FindByDomain(r.Context(), domain)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	if existing == nil {
		writeError(w, http.StatusNotFound, errors.New("no route for that domain"))
		return
	}

	// A vhost somebody else wrote is somebody else's business. Say so plainly
	// rather than offering a button that will fail.
	if existing.State == routeEnums.StateUnmanaged {
		writeError(w, http.StatusForbidden,
			errors.New("that vhost was not created here, so croft will not remove it: "+existing.File))
		return
	}

	p := d.Routes.RemovePlan(domain)

	if wantsPlan(r) {
		summary := fmt.Sprintf("Stop serving %s. The container stays.", domain)
		if existing.State == routeEnums.StateAdopted {
			summary = fmt.Sprintf("Stop serving %s — you edited this file by hand. The container stays.", domain)
		}
		writeJSON(w, http.StatusOK, map[string]any{"summary": summary, "plan": p})
		return
	}

	started := d.Jobs.Start("route-remove", domain, p, func(ctx context.Context, report func(int, string)) error {
		if d.Simulated {
			if err := d.rehearse(p, report); err != nil {
				return err
			}
			return d.Routes.Remove(ctx, domain)
		}

		for i, step := range p.Steps {
			report(i+1, step.Describe)
		}
		return d.Routes.Remove(ctx, domain)
	})

	writeJSON(w, http.StatusAccepted, map[string]string{"jobId": started.Id, "domain": domain})
}

func routeStatusFor(err error) int {
	switch {
	case errors.Is(err, routeServices.ErrDomainTaken):
		return http.StatusConflict
	case errors.Is(err, routeServices.ErrTargetUnknown):
		return http.StatusNotFound
	case errors.Is(err, routeServices.ErrDomainRequired),
		errors.Is(err, routeServices.ErrDomainInvalid),
		errors.Is(err, routeServices.ErrTargetNoAddress):
		return http.StatusBadRequest
	default:
		return http.StatusInternalServerError
	}
}

// cancelJob stops work that is under way.
//
// It is honest about what that means: a plan abandoned halfway has applied
// some of its steps and not the rest. The snapshot taken at step one is still
// there, and going back to it is a separate decision — made by a person, not
// by us on their behalf.
func (d Deps) cancelJob(w http.ResponseWriter, r *http.Request) {
	if d.ReadOnly {
		writeError(w, http.StatusForbidden, errors.New("this instance is read-only"))
		return
	}

	if !d.Jobs.Cancel(r.PathValue("id")) {
		writeError(w, http.StatusNotFound, errors.New("no such job, or it had already finished"))
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// readBody decodes what arrived, turning the two failures that actually happen
// into something a person can act on.
//
// "EOF" on screen is the daemon reporting its own plumbing: it means the
// request carried no body at all, which says nothing about what went wrong or
// what to do about it.
func readBody(r *http.Request, into any) error {
	err := json.NewDecoder(r.Body).Decode(into)

	switch {
	case errors.Is(err, io.EOF):
		return errors.New("that request arrived with nothing in it, so there is nothing to act on")
	case err != nil:
		return fmt.Errorf("the request body is not what was expected: %w", err)
	}
	return nil
}
