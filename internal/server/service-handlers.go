package server

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/Hyzokaaa/opencroft/internal/agent"
	"github.com/Hyzokaaa/opencroft/internal/shared/plan"
)

// Deployer is the privileged side of a deployment. Two plans, because you
// cannot know how to build code you have not seen: the first fetches it, the
// second says exactly what building and running it will do. Both are read
// before either runs.
type Deployer interface {
	FindAll(ctx context.Context, container string) (agent.ServicesResponse, error)
	InspectPlan(ctx context.Context, container string, want agent.ServiceDTO) (plan.Plan, error)
	Inspect(ctx context.Context, container string, want agent.ServiceDTO) (agent.DetectionDTO, error)
	DeployPlan(ctx context.Context, container string, want agent.ServiceDTO) (plan.Plan, error)
	Deploy(ctx context.Context, container string, want agent.ServiceDTO, report func(int, string)) error
	RollbackPlan(ctx context.Context, container, snapshot string) (plan.Plan, string, error)
	Rollback(ctx context.Context, container, snapshot string, report func(int, string)) error
	Logs(ctx context.Context, container, service string, lines int) (string, error)
}

func (d Deps) deployer(w http.ResponseWriter) (Deployer, bool) {
	deployer, ok := d.Services.(Deployer)
	if !ok || d.Services == nil {
		writeError(w, http.StatusServiceUnavailable,
			errors.New("this host cannot deploy projects"))
		return nil, false
	}
	return deployer, true
}

func (d Deps) writable(w http.ResponseWriter) bool {
	if d.ReadOnly {
		writeError(w, http.StatusForbidden, errors.New("this instance is read-only"))
		return false
	}
	return true
}

func (d Deps) listServices(w http.ResponseWriter, r *http.Request) {
	deployer, ok := d.deployer(w)
	if !ok {
		return
	}

	found, err := deployer.FindAll(r.Context(), r.PathValue("name"))
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, found)
}

func wanted(r *http.Request) (agent.ServiceDTO, error) {
	var body agent.ServiceDTO
	err := readBody(r, &body)
	return body, err
}

// inspectService is the first half of the flow: "I am going to look at the
// repository", with the git commands in plain sight, and then what was found.
// It builds nothing and starts nothing, and the snapshot it takes first means
// even that much is reversible.
func (d Deps) inspectService(w http.ResponseWriter, r *http.Request) {
	if !d.writable(w) {
		return
	}
	deployer, ok := d.deployer(w)
	if !ok {
		return
	}

	name := r.PathValue("name")
	body, err := wanted(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	p, err := deployer.InspectPlan(r.Context(), name, body)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	if wantsPlan(r) {
		writeJSON(w, http.StatusOK, map[string]any{
			"summary": fmt.Sprintf(
				"Look at %s inside %s. Nothing is built and nothing is started.", body.Repo, name),
			"plan": p,
		})
		return
	}

	// Waited for rather than watched: what comes back is the detection, and
	// there is nothing useful to narrate while three steps happen.
	detection, err := deployer.Inspect(r.Context(), name, body)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, detection)
}

// deployService is the second half. Everything in the body is what the person
// saw and could edit — detection proposed it, nothing decided it.
func (d Deps) deployService(w http.ResponseWriter, r *http.Request) {
	if !d.writable(w) {
		return
	}
	deployer, ok := d.deployer(w)
	if !ok {
		return
	}

	name := r.PathValue("name")
	body, err := wanted(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	p, err := deployer.DeployPlan(r.Context(), name, body)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	if wantsPlan(r) {
		writeJSON(w, http.StatusOK, map[string]any{
			"summary": summarise(name, body, d.crowding(r.Context(), name, body.Name)),
			"plan":    p,
		})
		return
	}

	started := d.Jobs.Start("deploy", name+"/"+body.Name, p,
		func(ctx context.Context, report func(int, string)) error {
			if d.Simulated {
				return d.rehearse(p, report)
			}
			return deployer.Deploy(ctx, name, body, report)
		})

	writeJSON(w, http.StatusAccepted, map[string]string{"jobId": started.Id, "name": body.Name})
}

func summarise(container string, service agent.ServiceDTO, crowding string) string {
	sentence := fmt.Sprintf("Deploy %s to %s as %s", service.Repo, container, service.Name)
	if service.Branch != "" {
		sentence += " from " + service.Branch
	}
	sentence += ", starting it with `" + service.Start + "`"

	if service.Port > 0 {
		sentence += fmt.Sprintf(" on port %d", service.Port)
	}
	sentence += ". A snapshot is taken first, so this can be undone."

	return strings.TrimSpace(sentence + " " + crowding)
}

// crowding is the warning at the moment it becomes true: adding a second
// service to a container is when the shared rollback starts to cost something,
// which is long before anybody tries to roll one back.
func (d Deps) crowding(ctx context.Context, container, adding string) string {
	deployer, ok := d.Services.(Deployer)
	if !ok {
		return ""
	}

	found, err := deployer.FindAll(ctx, container)
	if err != nil {
		return ""
	}

	others := []string{}
	for _, service := range found.Services {
		if service.Name != adding {
			others = append(others, service.Name)
		}
	}
	if len(others) == 0 {
		return ""
	}

	return fmt.Sprintf(
		"%s already runs %s. Snapshots are taken of containers, not of services, "+
			"so restoring one takes the other%s back too — one service per container keeps that simple.",
		container, strings.Join(others, ", "), plural(len(others)))
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}

// rollbackService restores a snapshot, which takes the whole container back.
// The warning comes from the agent, which knows what else is in there.
func (d Deps) rollbackService(w http.ResponseWriter, r *http.Request) {
	if !d.writable(w) {
		return
	}
	deployer, ok := d.deployer(w)
	if !ok {
		return
	}

	name := r.PathValue("name")

	var body struct {
		Snapshot string `json:"snapshot"`
	}
	if err := readBody(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	p, warning, err := deployer.RollbackPlan(r.Context(), name, body.Snapshot)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	if wantsPlan(r) {
		summary := fmt.Sprintf("Restore %s onto %s.", body.Snapshot, name)
		if warning != "" {
			summary += " " + warning
		}
		writeJSON(w, http.StatusOK, map[string]any{"summary": summary, "plan": p})
		return
	}

	started := d.Jobs.Start("rollback", name, p,
		func(ctx context.Context, report func(int, string)) error {
			if d.Simulated {
				return d.rehearse(p, report)
			}
			return deployer.Rollback(ctx, name, body.Snapshot, report)
		})

	writeJSON(w, http.StatusAccepted, map[string]string{"jobId": started.Id, "name": name})
}

// showServiceLogs is the first thing anybody asks for when a deployment does
// not come up. One request, rather than an ssh session.
func (d Deps) showServiceLogs(w http.ResponseWriter, r *http.Request) {
	deployer, ok := d.deployer(w)
	if !ok {
		return
	}

	lines := 200
	if asked, err := strconv.Atoi(r.URL.Query().Get("lines")); err == nil && asked > 0 {
		lines = asked
	}

	text, err := deployer.Logs(r.Context(), r.PathValue("name"), r.PathValue("service"), lines)
	if err != nil && strings.TrimSpace(text) == "" {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"lines": text})
}
