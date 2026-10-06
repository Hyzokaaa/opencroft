package server

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/Hyzokaaa/opencroft/internal/shared/plan"
)

// Aliaser gives a route more names — customers' own domains answering as the
// route does — each with its own certificate.
type Aliaser interface {
	AliasPlan(ctx context.Context, domain, alias string) (plan.Plan, error)
	AddAlias(ctx context.Context, domain, alias string, report func(int, string)) error
	AliasRemovalPlan(ctx context.Context, domain, alias string) (plan.Plan, error)
	RemoveAlias(ctx context.Context, domain, alias string, report func(int, string)) error
}

type aliasRequest struct {
	Alias string `json:"alias"`
}

func (d Deps) aliaser(w http.ResponseWriter) (Aliaser, bool) {
	aliaser, ok := d.Routes.(Aliaser)
	if !ok {
		writeError(w, http.StatusServiceUnavailable, errors.New("this host cannot give a domain more names"))
		return nil, false
	}
	return aliaser, true
}

func (d Deps) addAlias(w http.ResponseWriter, r *http.Request) {
	if !d.writable(w) {
		return
	}
	aliaser, ok := d.aliaser(w)
	if !ok {
		return
	}
	var body aliasRequest
	if err := readBody(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	domain := r.PathValue("domain")
	p, err := aliaser.AliasPlan(r.Context(), domain, body.Alias)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	if wantsPlan(r) {
		writeJSON(w, http.StatusOK, map[string]any{
			"summary": fmt.Sprintf("Make %s answer exactly as %s does — the same container, paths and "+
				"websockets — with a certificate of its own. Its DNS has to point to this server first: "+
				"until it does, it answers over http and its certificate can be retried by adding it again.",
				body.Alias, domain),
			"plan": p,
		})
		return
	}
	started := d.Jobs.Start("route-alias", domain+" + "+body.Alias, p, func(ctx context.Context, report func(int, string)) error {
		if d.Simulated {
			return d.rehearse(p, report)
		}
		return aliaser.AddAlias(ctx, domain, body.Alias, report)
	})
	writeJSON(w, http.StatusAccepted, map[string]string{"jobId": started.Id, "domain": domain})
}

func (d Deps) removeAlias(w http.ResponseWriter, r *http.Request) {
	if !d.writable(w) {
		return
	}
	aliaser, ok := d.aliaser(w)
	if !ok {
		return
	}
	domain, alias := r.PathValue("domain"), r.URL.Query().Get("alias")
	p, err := aliaser.AliasRemovalPlan(r.Context(), domain, alias)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if wantsPlan(r) {
		writeJSON(w, http.StatusOK, map[string]any{
			"summary": fmt.Sprintf("Stop answering on %s. %s and everything else on it stay as they are.", alias, domain),
			"plan":    p,
		})
		return
	}
	started := d.Jobs.Start("route-alias", domain+" - "+alias, p, func(ctx context.Context, report func(int, string)) error {
		if d.Simulated {
			return d.rehearse(p, report)
		}
		return aliaser.RemoveAlias(ctx, domain, alias, report)
	})
	writeJSON(w, http.StatusAccepted, map[string]string{"jobId": started.Id, "domain": domain})
}
