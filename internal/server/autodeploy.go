package server

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/Hyzokaaa/opencroft/internal/autodeploy"
	"github.com/Hyzokaaa/opencroft/internal/shared/job"
)

// A service can ask to be deployed again whenever its branch moves. The
// repository tells croft the moment somebody pushes, through a webhook; and
// croft asks the repository every few minutes as well, for when nobody set
// the webhook up or one went missing. Both only ever lead to the redeploy a
// person could have started — its plan, its snapshot, its line in Activity —
// and never to anything a request could choose.

// AutoDeployEvery is how often croft asks, when nothing tells it.
const AutoDeployEvery = 3 * time.Minute

// Tipper says which commit a service's branch ends at now.
type Tipper interface {
	Tip(ctx context.Context, container, service string) (string, error)
}

// AutoDeploys is what the handlers and the watcher share.
type AutoDeploys struct {
	Repo autodeploy.Repository

	mu sync.Mutex
	// busy is a service being checked or deployed; again, one a webhook
	// asked about meanwhile, to be checked once more when it is done.
	busy, again map[string]bool
	last        map[string]AutoDeployCheck
}

func NewAutoDeploys(repo autodeploy.Repository) *AutoDeploys {
	return &AutoDeploys{Repo: repo, busy: map[string]bool{}, again: map[string]bool{},
		last: map[string]AutoDeployCheck{}}
}

// AutoDeployCheck is what the last look at the repository found.
type AutoDeployCheck struct {
	At     time.Time `json:"at"`
	By     string    `json:"by"`
	Tip    string    `json:"tip,omitempty"`
	Result string    `json:"result"`
	Error  string    `json:"error,omitempty"`
}

type autoDeployResponse struct {
	Enabled bool             `json:"enabled"`
	Hook    string           `json:"hook,omitempty"`
	Secret  string           `json:"secret,omitempty"`
	Failed  string           `json:"failed,omitempty"`
	Last    *AutoDeployCheck `json:"last,omitempty"`
	Every   int              `json:"everySeconds"`
}

func hookPath(container, service string) string {
	return "/hooks/deploy/" + container + "/" + service
}

func (d Deps) autoDeploys(w http.ResponseWriter) (*AutoDeploys, bool) {
	if d.AutoDeploys == nil {
		writeError(w, http.StatusNotImplemented, errors.New("this panel cannot deploy on its own"))
		return nil, false
	}
	return d.AutoDeploys, true
}

func (d Deps) describeAutoDeploy(ctx context.Context, a *AutoDeploys, container, service string) (autoDeployResponse, error) {
	found, err := a.Repo.Find(ctx, container, service)
	if err != nil || found == nil {
		return autoDeployResponse{Every: int(AutoDeployEvery.Seconds())}, err
	}
	response := autoDeployResponse{
		Enabled: true, Hook: hookPath(container, service), Secret: found.Secret, Failed: found.Failed,
		Every: int(AutoDeployEvery.Seconds()),
	}
	a.mu.Lock()
	if last, ok := a.last[container+"/"+service]; ok {
		response.Last = &last
	}
	a.mu.Unlock()
	return response, nil
}

func (d Deps) showAutoDeploy(w http.ResponseWriter, r *http.Request) {
	a, ok := d.autoDeploys(w)
	if !ok {
		return
	}
	response, err := d.describeAutoDeploy(r.Context(), a, r.PathValue("name"), r.PathValue("service"))
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, response)
}

type autoDeployRequest struct {
	Enabled bool `json:"enabled"`
}

// changeAutoDeploy turns it on or off. Turning it on makes a new secret;
// off forgets it, so turning it on again is also how a leaked one is
// replaced. Nothing on the host changes, so there is no plan to show.
func (d Deps) changeAutoDeploy(w http.ResponseWriter, r *http.Request) {
	if !d.writable(w) {
		return
	}
	a, ok := d.autoDeploys(w)
	if !ok {
		return
	}
	var body autoDeployRequest
	if err := readBody(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	ctx := r.Context()
	container, service := r.PathValue("name"), r.PathValue("service")

	if body.Enabled {
		if err := d.knownService(ctx, container, service); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		found, err := a.Repo.Find(ctx, container, service)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		if found == nil {
			secret, err := autodeploy.NewSecret()
			if err != nil {
				writeError(w, http.StatusInternalServerError, err)
				return
			}
			if err := a.Repo.Save(ctx, autodeploy.Setting{Container: container, Service: service, Secret: secret}); err != nil {
				writeError(w, http.StatusInternalServerError, err)
				return
			}
		}
	} else if err := a.Repo.Remove(ctx, container, service); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	response, err := d.describeAutoDeploy(ctx, a, container, service)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, response)
}

func (d Deps) knownService(ctx context.Context, container, service string) error {
	deployer, ok := d.Services.(Deployer)
	if d.Services == nil || !ok {
		return errors.New("this host cannot deploy services")
	}
	found, err := deployer.FindAll(ctx, container)
	if err != nil {
		return err
	}
	for _, s := range found.Services {
		if s.Name == service {
			return nil
		}
	}
	return fmt.Errorf("there is no service %s in %s", service, container)
}

// hook is what a repository calls when somebody pushes. It is the one route
// open without a session, so it accepts only the service's secret — as
// GitLab sends it, or as GitHub signs the body with it — and all it can do is
// make croft look at the branch it already deploys from. Nothing in the body
// is believed: which commit to deploy is asked of the repository itself.
func (d Deps) hook(w http.ResponseWriter, r *http.Request) {
	refused := func() {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "Not accepted."})
	}
	if d.ReadOnly || d.AutoDeploys == nil {
		refused()
		return
	}
	container, service := r.PathValue("name"), r.PathValue("service")
	found, err := d.AutoDeploys.Repo.Find(r.Context(), container, service)
	if err != nil || found == nil {
		refused()
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		refused()
		return
	}
	if !signed(r, body, found.Secret) {
		refused()
		return
	}

	go d.checkBranch(context.Background(), container, service, "webhook")
	writeJSON(w, http.StatusAccepted, map[string]string{"status": "checking"})
}

// signed is GitLab's token or GitHub's signature, compared in constant time.
func signed(r *http.Request, body []byte, secret string) bool {
	if token := r.Header.Get("X-Gitlab-Token"); token != "" {
		return subtle.ConstantTimeCompare([]byte(token), []byte(secret)) == 1
	}
	if signature, ok := strings.CutPrefix(r.Header.Get("X-Hub-Signature-256"), "sha256="); ok {
		mac := hmac.New(sha256.New, []byte(secret))
		mac.Write(body)
		want := hex.EncodeToString(mac.Sum(nil))
		return subtle.ConstantTimeCompare([]byte(signature), []byte(want)) == 1
	}
	return false
}

// WatchBranches asks every service that deploys itself whether its branch
// moved, every AutoDeployEvery, until ctx ends.
func (d Deps) WatchBranches(ctx context.Context) {
	if d.AutoDeploys == nil || d.Simulated {
		return
	}
	ticker := time.NewTicker(AutoDeployEvery)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
		settings, err := d.AutoDeploys.Repo.FindAll(ctx)
		if err != nil {
			log.Printf("autodeploy: %v", err)
			continue
		}
		for _, s := range settings {
			d.checkBranch(ctx, s.Container, s.Service, "check")
		}
	}
}

// checkBranch deploys the service again if its branch ends somewhere other
// than what runs — unless that commit already failed, or the service is
// being deployed right now.
func (d Deps) checkBranch(ctx context.Context, container, service, by string) {
	a := d.AutoDeploys
	k := container + "/" + service

	a.mu.Lock()
	if a.busy[k] {
		// Looked at again once the current look, or deployment, is over.
		a.again[k] = true
		a.mu.Unlock()
		return
	}
	a.busy[k] = true
	a.mu.Unlock()

	deploying := false
	done := func() {
		a.mu.Lock()
		delete(a.busy, k)
		again := a.again[k]
		delete(a.again, k)
		a.mu.Unlock()
		if again {
			go d.checkBranch(context.Background(), container, service, "webhook")
		}
	}
	defer func() {
		if !deploying {
			done()
		}
	}()

	note := func(check AutoDeployCheck) {
		check.At, check.By = time.Now().UTC(), by
		a.mu.Lock()
		a.last[k] = check
		a.mu.Unlock()
		if check.Error != "" {
			log.Printf("autodeploy %s: %s", k, check.Error)
		}
	}

	setting, err := a.Repo.Find(ctx, container, service)
	if err != nil || setting == nil {
		return
	}
	deployer, ok := d.Services.(Deployer)
	tipper, canTip := d.Services.(Tipper)
	if d.Services == nil || !ok || !canTip {
		return
	}

	// A deployment somebody started is left to finish; the next look sees
	// what it left.
	if d.running(k) {
		note(AutoDeployCheck{Result: "a deployment is already running"})
		return
	}

	found, err := deployer.FindAll(ctx, container)
	if err != nil {
		note(AutoDeployCheck{Result: "could not read the service", Error: err.Error()})
		return
	}
	deployed, known := "", false
	for _, s := range found.Services {
		if s.Name == service {
			deployed, known = s.Commit, true
		}
	}
	if !known {
		note(AutoDeployCheck{Result: "the service is not there any more"})
		return
	}

	tip, err := tipper.Tip(ctx, container, service)
	if err != nil {
		note(AutoDeployCheck{Result: "could not ask the repository", Error: err.Error()})
		return
	}
	switch {
	case deployed != "" && strings.HasPrefix(tip, deployed):
		note(AutoDeployCheck{Tip: tip, Result: "up to date"})
		return
	case tip == setting.Failed:
		note(AutoDeployCheck{Tip: tip, Result: "this commit failed to deploy; waiting for the next one"})
		return
	}

	p, err := deployer.RedeployPlan(ctx, container, service)
	if err != nil {
		note(AutoDeployCheck{Tip: tip, Result: "could not plan the deployment", Error: err.Error()})
		return
	}
	note(AutoDeployCheck{Tip: tip, Result: "deploying " + short(tip)})
	deploying = true
	d.Jobs.Start("auto-deploy", k, p, func(ctx context.Context, report func(int, string)) error {
		defer done()
		err := deployer.Redeploy(ctx, container, service, report)

		// Remembered only while it is still asked for.
		if current, _ := a.Repo.Find(context.Background(), container, service); current != nil {
			current.Failed = ""
			if err != nil {
				current.Failed = tip
			}
			_ = a.Repo.Save(context.Background(), *current)
		}
		if err != nil {
			note(AutoDeployCheck{Tip: tip, Result: "the deployment of " + short(tip) + " failed", Error: err.Error()})
		} else {
			note(AutoDeployCheck{Tip: tip, Result: "deployed " + short(tip)})
		}
		return err
	})
}

// running is whether a job for the service is under way.
func (d Deps) running(subject string) bool {
	recent, err := d.Jobs.Recent(30)
	if err != nil {
		return false
	}
	for _, j := range recent {
		if j.Subject == subject && j.Status == job.StatusRunning {
			return true
		}
	}
	return false
}

func short(commit string) string {
	if len(commit) > 7 {
		return commit[:7]
	}
	return commit
}
