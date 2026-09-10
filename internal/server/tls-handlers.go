package server

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/Hyzokaaa/opencroft/internal/shared/plan"
)

// TLSEnabler is the privileged side again: obtaining a certificate and
// rewriting a vhost are both root's work.
type TLSEnabler interface {
	TLSPlan(ctx context.Context, domain string) (plan.Plan, error)
	EnableTLS(ctx context.Context, domain string, report func(int, string)) error
}

func (d Deps) enableTLS(w http.ResponseWriter, r *http.Request) {
	if d.ReadOnly {
		writeError(w, http.StatusForbidden, errors.New("this instance is read-only"))
		return
	}

	enabler, ok := d.Routes.(TLSEnabler)
	if !ok {
		writeError(w, http.StatusServiceUnavailable,
			errors.New("this host cannot issue certificates from the panel"))
		return
	}

	domain := r.PathValue("domain")
	p, err := enabler.TLSPlan(r.Context(), domain)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	if wantsPlan(r) {
		writeJSON(w, http.StatusOK, map[string]any{
			"summary": fmt.Sprintf(
				"Obtain a certificate for %s and serve it over https. The http address will redirect.", domain),
			"plan": p,
		})
		return
	}

	started := d.Jobs.Start("tls", domain, p, func(ctx context.Context, report func(int, string)) error {
		if d.Simulated {
			return d.rehearse(p, report)
		}
		return enabler.EnableTLS(ctx, domain, report)
	})

	writeJSON(w, http.StatusAccepted, map[string]string{"jobId": started.Id, "domain": domain})
}
