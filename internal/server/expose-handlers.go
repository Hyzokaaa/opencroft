package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/Hyzokaaa/opencroft/internal/shared/plan"
)

// Exposer is the privileged side once more: a certificate and a vhost.
type Exposer interface {
	ExposePlan(ctx context.Context, domain string, port int) (plan.Plan, error)
	Expose(ctx context.Context, domain string, port int, report func(int, string)) error
}

type exposeBody struct {
	Domain string `json:"domain"`
	Port   int    `json:"port"`
}

// Exposing the panel from the panel is worth doing carefully: if it goes
// wrong, the thing you are using to do it is what breaks. The vhost is removed
// again when nginx rejects it, so the worst case is that nothing changed.
func (d Deps) exposePanel(w http.ResponseWriter, r *http.Request) {
	if d.ReadOnly {
		writeError(w, http.StatusForbidden, errors.New("this instance is read-only"))
		return
	}

	exposer, ok := d.Expose.(Exposer)
	if !ok || d.Expose == nil {
		writeError(w, http.StatusServiceUnavailable,
			errors.New("this host cannot obtain certificates from the panel"))
		return
	}

	var body exposeBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if body.Port == 0 {
		body.Port = d.PanelPort
	}

	p, err := exposer.ExposePlan(r.Context(), body.Domain, body.Port)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	if wantsPlan(r) {
		writeJSON(w, http.StatusOK, map[string]any{
			"summary": fmt.Sprintf(
				"Serve this panel at https://%s. It keeps listening on localhost — nginx is what the internet reaches.",
				body.Domain),
			"plan": p,
		})
		return
	}

	started := d.Jobs.Start("expose", body.Domain, p, func(ctx context.Context, report func(int, string)) error {
		if d.Simulated {
			return d.rehearse(p, report)
		}
		return exposer.Expose(ctx, body.Domain, body.Port, report)
	})

	writeJSON(w, http.StatusAccepted, map[string]string{"jobId": started.Id, "domain": body.Domain})
}
