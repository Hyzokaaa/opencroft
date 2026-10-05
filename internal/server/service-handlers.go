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
	DestroyPlan(ctx context.Context, container, service string) (plan.Plan, string, error)
	Destroy(ctx context.Context, container, service string, report func(int, string)) error
	Environment(ctx context.Context, container, service string) (agent.EnvironmentDTO, error)
	EnvironmentPlan(ctx context.Context, container, service string, change agent.EnvChangeDTO) (plan.Plan, error)
	ChangeEnvironment(ctx context.Context, container, service string, change agent.EnvChangeDTO, report func(int, string)) error
	RedeployPlan(ctx context.Context, container, service string) (plan.Plan, error)
	Redeploy(ctx context.Context, container, service string, report func(int, string)) error
	PowerPlan(ctx context.Context, container, service string, action agent.PowerAction) (plan.Plan, error)
	Power(ctx context.Context, container, service string, action agent.PowerAction, report func(int, string)) error
	UnitPowerPlan(ctx context.Context, container, unit string, action agent.PowerAction) (plan.Plan, error)
	UnitPower(ctx context.Context, container, unit string, action agent.PowerAction, report func(int, string)) error
	UnitLogs(ctx context.Context, container, unit string, lines int) (string, error)
	AdoptionOf(ctx context.Context, container string, kind agent.Adoptable, key string) (agent.AdoptionDTO, error)
	AdoptPlan(ctx context.Context, container string, kind agent.Adoptable, key string, answer agent.AdoptDTO) (plan.Plan, error)
	Adopt(ctx context.Context, container string, kind agent.Adoptable, key string, answer agent.AdoptDTO, report func(int, string)) error
}

func (d Deps) deployer(w http.ResponseWriter) (Deployer, bool) {
	deployer, ok := d.Services.(Deployer)
	if !ok || d.Services == nil {
		writeError(w, http.StatusServiceUnavailable,
			errors.New("this host cannot deploy services"))
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
			"summary": summarise(name, body, adoptionOf(r.Context(), deployer, name, body.Name),
				d.crowding(r.Context(), name, body.Name)),
			"plan": p,
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

// showEnvironment is a service's environment file as it is now, with the hash
// a change has to be made against.
func (d Deps) showEnvironment(w http.ResponseWriter, r *http.Request) {
	deployer, ok := d.deployer(w)
	if !ok {
		return
	}
	found, err := deployer.Environment(r.Context(), r.PathValue("name"), r.PathValue("service"))
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, found)
}

// changeEnvironment rewrites only what was asked in a service's environment
// file, and makes it take effect. It is refused if the file changed since it
// was opened, so an edit made over ssh is never overwritten.
func (d Deps) changeEnvironment(w http.ResponseWriter, r *http.Request) {
	if !d.writable(w) {
		return
	}
	deployer, ok := d.deployer(w)
	if !ok {
		return
	}

	name, service := r.PathValue("name"), r.PathValue("service")
	var change agent.EnvChangeDTO
	if err := readBody(r, &change); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	p, err := deployer.EnvironmentPlan(r.Context(), name, service, change)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	if wantsPlan(r) {
		writeJSON(w, http.StatusOK, map[string]any{
			"summary": fmt.Sprintf(
				"Change the environment of %s: only what you changed is rewritten, the rest of the file "+
					"stays as it is. Then it takes effect. A snapshot is taken first.", service),
			"plan": p,
		})
		return
	}

	started := d.Jobs.Start("environment", name+"/"+service, p,
		func(ctx context.Context, report func(int, string)) error {
			if d.Simulated {
				return d.rehearse(p, report)
			}
			return deployer.ChangeEnvironment(ctx, name, service, change, report)
		})

	writeJSON(w, http.StatusAccepted, map[string]string{"jobId": started.Id, "name": service})
}

// redeployService deploys again what the container already records, with
// nothing asked: the same repository, branch, commands and environment, at the
// tip of the branch. The plan is still shown — only the form is skipped.
func (d Deps) redeployService(w http.ResponseWriter, r *http.Request) {
	if !d.writable(w) {
		return
	}
	deployer, ok := d.deployer(w)
	if !ok {
		return
	}

	name, service := r.PathValue("name"), r.PathValue("service")
	p, err := deployer.RedeployPlan(r.Context(), name, service)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	if wantsPlan(r) {
		summary := fmt.Sprintf("Redeploy %s in %s as it is configured.", service, name)
		if found, err := deployer.FindAll(r.Context(), name); err == nil {
			for _, s := range found.Services {
				if s.Name == service {
					summary = fmt.Sprintf(
						"Redeploy %s from the tip of %s, with the commands and environment it already has. "+
							"A snapshot is taken first, so this can be undone.", service, s.Branch)
				}
			}
		}
		writeJSON(w, http.StatusOK, map[string]any{"summary": summary, "plan": p})
		return
	}

	started := d.Jobs.Start("deploy", name+"/"+service, p,
		func(ctx context.Context, report func(int, string)) error {
			if d.Simulated {
				return d.rehearse(p, report)
			}
			return deployer.Redeploy(ctx, name, service, report)
		})

	writeJSON(w, http.StatusAccepted, map[string]string{"jobId": started.Id, "name": service})
}

func summarise(container string, service agent.ServiceDTO, adopted *agent.AdoptedDTO, crowding string) string {
	sentence := fmt.Sprintf("Deploy %s to %s as %s", service.Repo, container, service.Name)
	if service.Branch != "" {
		sentence += " from " + service.Branch
	}
	switch {
	case adopted != nil && adopted.Site != "":
		sentence += ", publishing the build to " + adopted.Site + " — nothing is restarted, the web server serves the new files"
	case adopted != nil:
		sentence += ", restarting its own unit " + adopted.Unit + " — which, like its environment, stays as it is"
	default:
		sentence += ", starting it with `" + service.Start + "`"
	}

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

// destroyService removes one service from a container.
//
// Everything below the snapshot it takes first is irreversible: the unit, the
// code, and the environment file with whatever secrets are in it. The summary
// says what else stops being true — a domain that pointed at its port, and the
// deployment snapshots that stay behind because they are the way back.
func (d Deps) destroyService(w http.ResponseWriter, r *http.Request) {
	if !d.writable(w) {
		return
	}
	deployer, ok := d.deployer(w)
	if !ok {
		return
	}

	name, service := r.PathValue("name"), r.PathValue("service")

	p, warning, err := deployer.DestroyPlan(r.Context(), name, service)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	// An adopted service is let go, not removed: croft did not put it there.
	kind := "destroy"
	summary := fmt.Sprintf("Remove %s from %s, and everything it wrote beside its code.", service, name)
	if adoptionOf(r.Context(), deployer, name, service) != nil {
		kind = "release"
		summary = fmt.Sprintf("Let go of %s: croft forgets what it recorded about it.", service)
	}

	if wantsPlan(r) {
		if warning != "" {
			summary += " " + warning
		}
		writeJSON(w, http.StatusOK, map[string]any{"summary": summary, "plan": p})
		return
	}

	started := d.Jobs.Start(kind, name+"/"+service, p,
		func(ctx context.Context, report func(int, string)) error {
			if d.Simulated {
				return d.rehearse(p, report)
			}
			return deployer.Destroy(ctx, name, service, report)
		})

	writeJSON(w, http.StatusAccepted, map[string]string{"jobId": started.Id, "name": service})
}

// adoptionOf says whether a service is one croft took on rather than deployed,
// as the agent reads it off the container.
func adoptionOf(ctx context.Context, deployer Deployer, container, service string) *agent.AdoptedDTO {
	found, err := deployer.FindAll(ctx, container)
	if err != nil {
		return nil
	}
	for _, s := range found.Services {
		if s.Name == service {
			return s.Adopted
		}
	}
	return nil
}

// subject is the unit or site a request is about.
func subject(r *http.Request, kind agent.Adoptable) string {
	if kind == agent.AdoptSite {
		return r.PathValue("site")
	}
	return r.PathValue("unit")
}

// showAdoption is what croft would record about something it found, read
// before anything is agreed to. It changes nothing.
func (d Deps) showAdoption(kind agent.Adoptable) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		deployer, ok := d.deployer(w)
		if !ok {
			return
		}
		found, err := deployer.AdoptionOf(r.Context(), r.PathValue("name"), kind, subject(r, kind))
		if err != nil {
			writeError(w, http.StatusNotFound, err)
			return
		}
		writeJSON(w, http.StatusOK, found)
	}
}

// adopt takes on a unit or a site croft found. Its plan is annotations and
// nothing else: the container is untouched until the next deployment, which
// comes with a plan and a snapshot of its own.
func (d Deps) adopt(kind agent.Adoptable) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !d.writable(w) {
			return
		}
		deployer, ok := d.deployer(w)
		if !ok {
			return
		}

		name, key := r.PathValue("name"), subject(r, kind)
		var answer agent.AdoptDTO
		if err := readBody(r, &answer); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}

		p, err := deployer.AdoptPlan(r.Context(), name, kind, key, answer)
		if err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}

		if wantsPlan(r) {
			writeJSON(w, http.StatusOK, map[string]any{"summary": taken(kind, key, answer.Name, name), "plan": p})
			return
		}

		started := d.Jobs.Start("adopt", name+"/"+answer.Name, p,
			func(ctx context.Context, report func(int, string)) error {
				if d.Simulated {
					return d.rehearse(p, report)
				}
				return deployer.Adopt(ctx, name, kind, key, answer, report)
			})

		writeJSON(w, http.StatusAccepted, map[string]string{"jobId": started.Id, "name": answer.Name})
	}
}

func taken(kind agent.Adoptable, key, service, container string) string {
	if kind == agent.AdoptSite {
		return fmt.Sprintf(
			"Take on the site at %s as %s. Nothing inside %s changes now: croft records what it found, "+
				"and from the next deployment on it fetches, builds and publishes it — leaving the web "+
				"server's configuration and the build's environment file exactly as they are.",
			key, service, container)
	}
	return fmt.Sprintf(
		"Take on %s as %s. Nothing inside %s changes now: croft records what it found, and "+
			"from the next deployment on it fetches, builds and restarts %s — leaving the unit "+
			"and its environment file exactly as they are.", key, service, container, key)
}

// powerService restarts, stops or starts a service croft deployed; powerUnit
// does the same to one it only found. Nothing is fetched or built either way —
// the process is bounced, which is what a hang or a change made by hand needs.
func (d Deps) powerService(action agent.PowerAction) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		name, service := r.PathValue("name"), r.PathValue("service")
		d.bounce(w, r, action, name, service,
			func(ctx context.Context, deployer Deployer) (plan.Plan, error) {
				return deployer.PowerPlan(ctx, name, service, action)
			},
			func(ctx context.Context, deployer Deployer, report func(int, string)) error {
				return deployer.Power(ctx, name, service, action, report)
			})
	}
}

func (d Deps) powerUnit(action agent.PowerAction) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		name, unit := r.PathValue("name"), r.PathValue("unit")
		d.bounce(w, r, action, name, unit,
			func(ctx context.Context, deployer Deployer) (plan.Plan, error) {
				return deployer.UnitPowerPlan(ctx, name, unit, action)
			},
			func(ctx context.Context, deployer Deployer, report func(int, string)) error {
				return deployer.UnitPower(ctx, name, unit, action, report)
			})
	}
}

func (d Deps) bounce(
	w http.ResponseWriter, r *http.Request, action agent.PowerAction, container, subject string,
	planOf func(context.Context, Deployer) (plan.Plan, error),
	run func(context.Context, Deployer, func(int, string)) error,
) {
	if !d.writable(w) {
		return
	}
	deployer, ok := d.deployer(w)
	if !ok {
		return
	}

	p, err := planOf(r.Context(), deployer)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	if wantsPlan(r) {
		writeJSON(w, http.StatusOK, map[string]any{"summary": bounced(action, subject, container), "plan": p})
		return
	}

	started := d.Jobs.Start(string(action), container+"/"+subject, p,
		func(ctx context.Context, report func(int, string)) error {
			if d.Simulated {
				return d.rehearse(p, report)
			}
			return run(ctx, deployer, report)
		})

	writeJSON(w, http.StatusAccepted, map[string]string{"jobId": started.Id, "name": subject})
}

func bounced(action agent.PowerAction, subject, container string) string {
	switch action {
	case agent.PowerStop:
		return fmt.Sprintf("Stop %s in %s. Anything it serves goes down until it is started again.",
			subject, container)
	case agent.PowerStart:
		return fmt.Sprintf("Start %s in %s.", subject, container)
	default:
		return fmt.Sprintf("Restart %s in %s. Nothing is fetched or built — the process is stopped and started again.",
			subject, container)
	}
}

func (d Deps) showUnitLogs(w http.ResponseWriter, r *http.Request) {
	deployer, ok := d.deployer(w)
	if !ok {
		return
	}

	lines := 200
	if asked, err := strconv.Atoi(r.URL.Query().Get("lines")); err == nil && asked > 0 {
		lines = asked
	}

	text, err := deployer.UnitLogs(r.Context(), r.PathValue("name"), r.PathValue("unit"), lines)
	if err != nil && strings.TrimSpace(text) == "" {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"lines": text})
}

// Configurer saves a service's properties without deploying it; they take
// effect at its next deployment, and the listing says so meanwhile.
type Configurer interface {
	ConfigurePlan(ctx context.Context, container string, want agent.ServiceDTO) (plan.Plan, error)
	Configure(ctx context.Context, container string, want agent.ServiceDTO, report func(int, string)) error
}

func (d Deps) configureService(w http.ResponseWriter, r *http.Request) {
	if !d.writable(w) {
		return
	}
	configurer, ok := d.Services.(Configurer)
	if !ok {
		writeError(w, http.StatusServiceUnavailable, errors.New("this host cannot save properties"))
		return
	}

	name, service := r.PathValue("name"), r.PathValue("service")
	var body agent.ServiceDTO
	if err := readBody(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	body.Name = service

	p, err := configurer.ConfigurePlan(r.Context(), name, body)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	if wantsPlan(r) {
		writeJSON(w, http.StatusOK, map[string]any{
			"summary": fmt.Sprintf("Save the properties of %s. Nothing that runs changes: they take effect "+
				"the next time %s is deployed, and until then it shows as having changes not deployed.", service, service),
			"plan": p,
		})
		return
	}

	started := d.Jobs.Start("configure", name+"/"+service, p,
		func(ctx context.Context, report func(int, string)) error {
			if d.Simulated {
				return d.rehearse(p, report)
			}
			return configurer.Configure(ctx, name, body, report)
		})
	writeJSON(w, http.StatusAccepted, map[string]string{"jobId": started.Id, "name": service})
}

// SnapshotRemover deletes a snapshot somebody chose to let go of.
type SnapshotRemover interface {
	SnapshotRemovalPlan(ctx context.Context, container, snapshot string) (plan.Plan, string, error)
	RemoveSnapshot(ctx context.Context, container, snapshot string, report func(int, string)) error
}

func (d Deps) removeSnapshot(w http.ResponseWriter, r *http.Request) {
	if !d.writable(w) {
		return
	}
	remover, ok := d.Services.(SnapshotRemover)
	if !ok {
		writeError(w, http.StatusServiceUnavailable, errors.New("this host cannot remove snapshots"))
		return
	}
	name, snapshot := r.PathValue("name"), r.PathValue("snapshot")
	p, warning, err := remover.SnapshotRemovalPlan(r.Context(), name, snapshot)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	if wantsPlan(r) {
		writeJSON(w, http.StatusOK, map[string]any{
			"summary": fmt.Sprintf("Delete the snapshot %s of %s, for good: nothing else keeps a copy. %s", snapshot, name, warning),
			"plan":    p,
		})
		return
	}

	started := d.Jobs.Start("snapshot-remove", name+"/"+snapshot, p,
		func(ctx context.Context, report func(int, string)) error {
			if d.Simulated {
				return d.rehearse(p, report)
			}
			return remover.RemoveSnapshot(ctx, name, snapshot, report)
		})
	writeJSON(w, http.StatusAccepted, map[string]string{"jobId": started.Id, "name": snapshot})
}
