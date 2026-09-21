package server

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/Hyzokaaa/opencroft/internal/agent"
	"github.com/Hyzokaaa/opencroft/internal/shared/plan"
)

// Provisioner is the database side of the agent.
//
// A database lives inside the container that uses it, which is what makes a
// snapshot of that container a snapshot of the application and its data at the
// same instant. The password is generated on the far side of the socket and
// never comes back: this process could not leak it if it tried.
type Provisioner interface {
	FindAll(ctx context.Context, container string) (agent.DatabasesResponse, error)
	ProvisionPlan(ctx context.Context, container string, want agent.DatabaseDTO) (plan.Plan, error)
	Provision(ctx context.Context, container string, want agent.DatabaseDTO, report func(int, string)) error
	DestroyPlan(ctx context.Context, container, database string) (plan.Plan, string, error)
	Destroy(ctx context.Context, container, database string, report func(int, string)) error
}

func (d Deps) provisioner(w http.ResponseWriter) (Provisioner, bool) {
	provisioner, ok := d.Databases.(Provisioner)
	if !ok || d.Databases == nil {
		writeError(w, http.StatusServiceUnavailable,
			errors.New("this host cannot provision databases"))
		return nil, false
	}
	return provisioner, true
}

func (d Deps) listDatabases(w http.ResponseWriter, r *http.Request) {
	provisioner, ok := d.provisioner(w)
	if !ok {
		return
	}

	found, err := provisioner.FindAll(r.Context(), r.PathValue("name"))
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, found)
}

// provisionDatabase installs an engine inside the container and creates one
// database for the application beside it.
//
// The summary says the part that matters and is easy to miss: from here on,
// restoring a snapshot of this container restores the data too. That is the
// advantage and it is also the thing to know before rolling back.
func (d Deps) provisionDatabase(w http.ResponseWriter, r *http.Request) {
	if !d.writable(w) {
		return
	}
	provisioner, ok := d.provisioner(w)
	if !ok {
		return
	}

	name := r.PathValue("name")

	var body agent.DatabaseDTO
	if err := readBody(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	p, err := provisioner.ProvisionPlan(r.Context(), name, body)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	if wantsPlan(r) {
		writeJSON(w, http.StatusOK, map[string]any{
			"summary": fmt.Sprintf(
				"Install %s inside %s and create %s for it. The password is generated "+
					"in the container and never leaves it. From now on a snapshot of %s "+
					"holds the data as well as the code — which is what makes going back "+
					"work, and what makes it worth reading the warning when you do.",
				body.Engine, name, database(body), name),
			"plan": p,
		})
		return
	}

	started := d.Jobs.Start("database", name+"/"+body.Name, p,
		func(ctx context.Context, report func(int, string)) error {
			if d.Simulated {
				return d.rehearse(p, report)
			}
			return provisioner.Provision(ctx, name, body, report)
		})

	writeJSON(w, http.StatusAccepted, map[string]string{"jobId": started.Id, "name": body.Name})
}

func database(body agent.DatabaseDTO) string {
	if body.DB != "" {
		return body.DB
	}
	return body.Name
}

// destroyDatabase drops the data. It is the one operation in the module whose
// only way back is the snapshot it takes first, and the summary says so.
func (d Deps) destroyDatabase(w http.ResponseWriter, r *http.Request) {
	if !d.writable(w) {
		return
	}
	provisioner, ok := d.provisioner(w)
	if !ok {
		return
	}

	name, target := r.PathValue("name"), r.PathValue("database")

	p, warning, err := provisioner.DestroyPlan(r.Context(), name, target)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	if wantsPlan(r) {
		summary := fmt.Sprintf("Remove %s from %s.", target, name)
		if warning != "" {
			summary += " " + warning
		}
		writeJSON(w, http.StatusOK, map[string]any{"summary": summary, "plan": p})
		return
	}

	started := d.Jobs.Start("database-remove", name+"/"+target, p,
		func(ctx context.Context, report func(int, string)) error {
			if d.Simulated {
				return d.rehearse(p, report)
			}
			return provisioner.Destroy(ctx, name, target, report)
		})

	writeJSON(w, http.StatusAccepted, map[string]string{"jobId": started.Id, "name": target})
}
