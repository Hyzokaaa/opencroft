package server

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/Hyzokaaa/opencroft/internal/agent"
	"github.com/Hyzokaaa/opencroft/internal/shared/plan"
)

// Projector is the agent's project side: declarations are files in /etc and
// membership is a label written with lxc, both root's to write.
type Projector interface {
	List(ctx context.Context) ([]agent.ProjectDTO, error)
	DeclarePlan(ctx context.Context, name, description string) (plan.Plan, error)
	Declare(ctx context.Context, name, description string, report func(int, string)) error
	RemovePlan(ctx context.Context, name string) (plan.Plan, error)
	Remove(ctx context.Context, name string, report func(int, string)) error
	AssignPlan(ctx context.Context, instance, project string) (plan.Plan, error)
	Assign(ctx context.Context, instance, project string, report func(int, string)) error
}

func (d Deps) projector(w http.ResponseWriter) (Projector, bool) {
	projector, ok := d.Projects.(Projector)
	if !ok || d.Projects == nil {
		writeError(w, http.StatusServiceUnavailable, errors.New("this host cannot keep projects"))
		return nil, false
	}
	return projector, true
}

func (d Deps) listProjects(w http.ResponseWriter, r *http.Request) {
	projector, ok := d.projector(w)
	if !ok {
		return
	}
	found, err := projector.List(r.Context())
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	writeJSON(w, http.StatusOK, found)
}

// projectJob shows a project change's plan or runs it, like every write.
func (d Deps) projectJob(w http.ResponseWriter, r *http.Request, subject, summary string,
	p plan.Plan, run func(ctx context.Context, report func(int, string)) error) {
	if wantsPlan(r) {
		writeJSON(w, http.StatusOK, map[string]any{"summary": summary, "plan": p})
		return
	}
	started := d.Jobs.Start("project", subject, p, func(ctx context.Context, report func(int, string)) error {
		if d.Simulated {
			return d.rehearse(p, report)
		}
		return run(ctx, report)
	})
	writeJSON(w, http.StatusAccepted, map[string]string{"jobId": started.Id, "project": subject})
}

func (d Deps) declareProject(w http.ResponseWriter, r *http.Request) {
	if !d.writable(w) {
		return
	}
	projector, ok := d.projector(w)
	if !ok {
		return
	}
	var body agent.ProjectDTO
	if err := readBody(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	p, err := projector.DeclarePlan(r.Context(), body.Name, body.Description)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	d.projectJob(w, r, body.Name,
		fmt.Sprintf("Declare the project %s. It is a file on this host, so it outlives the panel; "+
			"which containers are in it stays a label on each of them.", body.Name),
		p, func(ctx context.Context, report func(int, string)) error {
			return projector.Declare(ctx, body.Name, body.Description, report)
		})
}

func (d Deps) removeProject(w http.ResponseWriter, r *http.Request) {
	if !d.writable(w) {
		return
	}
	projector, ok := d.projector(w)
	if !ok {
		return
	}
	name := r.PathValue("project")

	p, err := projector.RemovePlan(r.Context(), name)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	d.projectJob(w, r, name,
		fmt.Sprintf("Forget the project %s. No container is in it, so nothing else changes.", name),
		p, func(ctx context.Context, report func(int, string)) error {
			return projector.Remove(ctx, name, report)
		})
}

func (d Deps) assignProject(w http.ResponseWriter, r *http.Request) {
	if !d.writable(w) {
		return
	}
	projector, ok := d.projector(w)
	if !ok {
		return
	}
	instance := r.PathValue("name")
	var body agent.AssignmentDTO
	if err := readBody(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	p, err := projector.AssignPlan(r.Context(), instance, body.Project)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	summary := fmt.Sprintf("Put %s in %s. Only its label changes: nothing restarts, and "+
		"its services, domains and snapshots stay as they are.", instance, body.Project)
	if body.Project == "" {
		summary = fmt.Sprintf("Take %s out of its project. Only its label changes; nothing restarts.", instance)
	}
	d.projectJob(w, r, instance, summary, p, func(ctx context.Context, report func(int, string)) error {
		return projector.Assign(ctx, instance, body.Project, report)
	})
}
