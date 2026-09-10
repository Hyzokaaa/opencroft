package server

import (
	"context"
	"encoding/json"
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
	InspectPlan(ctx context.Context, name string, want agent.Inspect) (plan.Plan, error)
	Inspect(ctx context.Context, name string, want agent.Inspect) (agent.DetectionDTO, error)
	DeployPlan(ctx context.Context, name string, want agent.Deploy) (plan.Plan, error)
	Deploy(ctx context.Context, name string, want agent.Deploy, report func(int, string)) error
	Find(ctx context.Context, name string) (agent.AppDTO, error)
	Logs(ctx context.Context, name string, lines int) (string, error)
}

func (d Deps) deployer(w http.ResponseWriter) (Deployer, bool) {
	deployer, ok := d.Apps.(Deployer)
	if !ok || d.Apps == nil {
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

// inspectProject is the first half of the flow the panel shows: "I am going to
// look at the repository", with the git commands in plain sight, and then what
// was found. It builds nothing and starts nothing, and the snapshot it takes
// first means even that much is reversible.
func (d Deps) inspectProject(w http.ResponseWriter, r *http.Request) {
	if !d.writable(w) {
		return
	}
	deployer, ok := d.deployer(w)
	if !ok {
		return
	}

	name := r.PathValue("name")

	var body agent.Inspect
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
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

	// Run rather than queued: what comes back is the detection, and there is
	// nothing useful to show while three steps happen.
	detection, err := deployer.Inspect(r.Context(), name, body)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, detection)
}

// deployProject is the second half. Everything in the body is what the person
// saw and could edit — detection proposed it, nothing decided it.
func (d Deps) deployProject(w http.ResponseWriter, r *http.Request) {
	if !d.writable(w) {
		return
	}
	deployer, ok := d.deployer(w)
	if !ok {
		return
	}

	name := r.PathValue("name")

	var body agent.Deploy
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
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
			"summary": summarise(name, body),
			"plan":    p,
		})
		return
	}

	started := d.Jobs.Start("deploy", name, p, func(ctx context.Context, report func(int, string)) error {
		if d.Simulated {
			return d.rehearse(p, report)
		}
		return deployer.Deploy(ctx, name, body, report)
	})

	writeJSON(w, http.StatusAccepted, map[string]string{"jobId": started.Id, "name": name})
}

func summarise(name string, want agent.Deploy) string {
	sentence := fmt.Sprintf("Deploy %s to %s", want.Repo, name)
	if want.Branch != "" {
		sentence += " from " + want.Branch
	}
	sentence += ", starting it with `" + want.Start + "`"

	if want.Port > 0 {
		sentence += fmt.Sprintf(" on port %d", want.Port)
	}
	return sentence + ". A snapshot is taken first, so this can be undone."
}

func (d Deps) showApp(w http.ResponseWriter, r *http.Request) {
	deployer, ok := d.deployer(w)
	if !ok {
		return
	}

	app, err := deployer.Find(r.Context(), r.PathValue("name"))
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, app)
}

// showAppLogs is the first thing anybody asks for when a deployment does not
// come up. One request, rather than an ssh session.
func (d Deps) showAppLogs(w http.ResponseWriter, r *http.Request) {
	deployer, ok := d.deployer(w)
	if !ok {
		return
	}

	lines := 200
	if asked, err := strconv.Atoi(r.URL.Query().Get("lines")); err == nil && asked > 0 {
		lines = asked
	}

	text, err := deployer.Logs(r.Context(), r.PathValue("name"), lines)
	if err != nil && strings.TrimSpace(text) == "" {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"lines": text})
}
