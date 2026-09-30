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

// TakerOver moves a hand-written vhost aside and writes croft's own in its
// place. Which file to move is decided on the privileged side, from what nginx
// loaded; the panel only names the domain.
type TakerOver interface {
	TakeOverPlan(ctx context.Context, domain string) (plan.Plan, error)
	TakeOver(ctx context.Context, domain string, report func(int, string)) error
}

func (d Deps) takeOver(w http.ResponseWriter, r *http.Request) {
	if !d.writable(w) {
		return
	}
	taker, ok := d.Routes.(TakerOver)
	if !ok {
		writeError(w, http.StatusServiceUnavailable, errors.New("this host cannot take over a domain from the panel"))
		return
	}

	domain := r.PathValue("domain")
	p, err := taker.TakeOverPlan(r.Context(), domain)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	if wantsPlan(r) {
		writeJSON(w, http.StatusOK, map[string]any{
			"summary": fmt.Sprintf(
				"Take over %s: croft writes its own vhost for it — same target, same certificate, so nothing "+
					"a visitor sees changes — and moves the hand-written one aside, where moving it back undoes "+
					"this. From then on croft can add paths to it and edit it.", domain),
			"plan": p,
		})
		return
	}

	started := d.Jobs.Start("takeover", domain, p, func(ctx context.Context, report func(int, string)) error {
		if d.Simulated {
			return d.rehearse(p, report)
		}
		return taker.TakeOver(ctx, domain, report)
	})
	writeJSON(w, http.StatusAccepted, map[string]string{"jobId": started.Id, "domain": domain})
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
				"Serve %s over https with a certificate croft obtains from Let's Encrypt and renews from then on. "+
					"The http address will redirect. If certbot renewed the one it had, certbot stops — its files stay.", domain),
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

// WildcardIssuer obtains one certificate for a domain and every name one label
// below it, after which https for a new subdomain needs nothing issued.
type WildcardIssuer interface {
	WildcardPlan(ctx context.Context, domain string) (plan.Plan, error)
	IssueWildcard(ctx context.Context, domain string, report func(int, string)) error
}

type wildcardRequest struct {
	Domain string `json:"domain"`
}

func (d Deps) issueWildcard(w http.ResponseWriter, r *http.Request) {
	if !d.writable(w) {
		return
	}
	issuer, ok := d.Routes.(WildcardIssuer)
	if !ok {
		writeError(w, http.StatusServiceUnavailable, errors.New("this host cannot issue certificates from the panel"))
		return
	}

	var body wildcardRequest
	if err := readBody(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	p, err := issuer.WildcardPlan(r.Context(), body.Domain)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	if wantsPlan(r) {
		writeJSON(w, http.StatusOK, map[string]any{
			"summary": fmt.Sprintf(
				"One certificate for *.%s and %s, proved over DNS and renewed by croft. Nothing is served "+
					"with it yet: Enable https on a domain it covers uses it without asking for another.",
				body.Domain, body.Domain),
			"plan": p,
		})
		return
	}

	started := d.Jobs.Start("wildcard", body.Domain, p, func(ctx context.Context, report func(int, string)) error {
		if d.Simulated {
			return d.rehearse(p, report)
		}
		return issuer.IssueWildcard(ctx, body.Domain, report)
	})
	writeJSON(w, http.StatusAccepted, map[string]string{"jobId": started.Id, "domain": body.Domain})
}
