package agent

import (
	"errors"
	"net/http"

	certificateServices "github.com/Hyzokaaa/opencroft/internal/certificate/domain/services"
	"github.com/Hyzokaaa/opencroft/internal/certificate/infrastructure/acme"
	"github.com/Hyzokaaa/opencroft/internal/shared/plan"
)

// A wildcard is one certificate for a domain and every name one label below
// it. Once croft keeps one, turning on https for a new subdomain asks the
// authority for nothing: the vhost points at the wildcard and that is all.
// It can only be proved over DNS, and croft renews it like any other.

func (s *Server) acceptWildcard(r *http.Request) (string, error) {
	domain := r.URL.Query().Get("domain")
	if !domainPattern.MatchString(domain) {
		return "", errors.New("that is not a domain name — give the one the wildcard is below, example.com for *.example.com")
	}
	if _, ok := acme.DNSAvailable(); !ok {
		return "", certificateServices.ErrWildcardNeedsDNS
	}
	return domain, nil
}

func wildcardPlan(domain string) plan.Plan {
	return plan.New(plan.Command(
		"Obtain one certificate for *."+domain+" and "+domain+" from Let's Encrypt, proved over DNS, which croft renews from then on",
		"croft", "cert", "issue", domain, "--wildcard"))
}

func (s *Server) planWildcard(w http.ResponseWriter, r *http.Request) {
	domain, err := s.acceptWildcard(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, PlanResponse{Plan: wildcardPlan(domain)})
}

func (s *Server) issueWildcard(w http.ResponseWriter, r *http.Request) {
	domain, err := s.acceptWildcard(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	ctx := r.Context()
	s.stream(w, func(report func(int, string)) error {
		report(1, "Obtaining a certificate for *."+domain)
		issuer := certificateServices.NewIssueCertificate(acme.NewIssuer())
		_, err := issuer.Execute(ctx, certificateServices.IssueRequest{
			Domain: domain, Wildcard: true, Challenge: certificateServices.ChallengeDNS,
		}, func(text string) { report(1, text) })
		return err
	})
}
