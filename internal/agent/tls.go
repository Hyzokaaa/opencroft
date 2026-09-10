package agent

import (
	"errors"
	"net/http"

	certificateServices "github.com/Hyzokaaa/opencroft/internal/certificate/domain/services"
	"github.com/Hyzokaaa/opencroft/internal/certificate/infrastructure/acme"
	routeEntities "github.com/Hyzokaaa/opencroft/internal/route/domain/entities"
	routeEnums "github.com/Hyzokaaa/opencroft/internal/route/domain/enums"
	"github.com/Hyzokaaa/opencroft/internal/shared/plan"
)

// tlsPlan is what turning on HTTPS for a domain does.
//
// The order is forced by how validation works: the http vhost has to exist and
// be serving before the authority is asked, because that is what answers the
// challenge. Once the certificate is in hand, the same file is rewritten to
// serve TLS and redirect http to it.
func (s *Server) tlsPlan(domain string, challenge certificateServices.Challenge) plan.Plan {
	issue := plan.Command(
		"Obtain a certificate from Let's Encrypt ("+string(challenge)+")",
		"croft", "cert", "issue", domain, challengeFlag(challenge))

	steps := []plan.Step{issue}
	steps = append(steps, s.routes.WritePlan(tlsRoute(domain)).Steps...)
	return plan.New(steps...)
}

func challengeFlag(challenge certificateServices.Challenge) string {
	if challenge == certificateServices.ChallengeDNS {
		return "--dns"
	}
	return "--http"
}

// chooseChallenge prefers DNS when credentials are configured: it does not
// depend on port 80 being reachable from the internet, which is not something
// this machine can find out about itself.
func chooseChallenge() certificateServices.Challenge {
	if _, ok := acme.DNSAvailable(); ok {
		return certificateServices.ChallengeDNS
	}
	return certificateServices.ChallengeHTTP
}

func (s *Server) planTLS(w http.ResponseWriter, r *http.Request) {
	domain := r.PathValue("domain")
	if !domainPattern.MatchString(domain) {
		writeError(w, http.StatusBadRequest, errors.New("that is not a domain name"))
		return
	}
	writeJSON(w, http.StatusOK, PlanResponse{Plan: s.tlsPlan(domain, chooseChallenge())})
}

func (s *Server) enableTLS(w http.ResponseWriter, r *http.Request) {
	domain := r.PathValue("domain")
	if !domainPattern.MatchString(domain) {
		writeError(w, http.StatusBadRequest, errors.New("that is not a domain name"))
		return
	}

	existing, err := s.routes.FindByDomain(r.Context(), domain)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	if existing == nil {
		writeError(w, http.StatusNotFound, errors.New("no route for that domain; add it first"))
		return
	}
	if existing.State == routeEnums.StateUnmanaged {
		writeError(w, http.StatusForbidden,
			errors.New("that vhost was not created by croft, so croft will not rewrite it: "+existing.File))
		return
	}

	challenge := chooseChallenge()
	p := s.tlsPlan(domain, challenge)

	s.stream(w, func(report func(int, string)) error {
		report(1, "Obtaining a certificate")

		issuer := certificateServices.NewIssueCertificate(acme.NewIssuer())
		if _, err := issuer.Execute(r.Context(), certificateServices.IssueRequest{
			Domain:    domain,
			Challenge: challenge,
		}, func(text string) { report(1, text) }); err != nil {
			return err
		}

		// The route keeps its target and port; only TLS changes.
		route := tlsRoute(domain)
		route.Target = existing.Target
		route.Port = existing.Port

		for i, step := range s.routes.WritePlan(route).Steps {
			report(i+2, step.Describe)
		}
		return s.routes.Write(r.Context(), route)
	})

	_ = p
}

func tlsRoute(domain string) *routeEntities.Route {
	return routeEntities.NewRoute(routeEntities.RouteProps{
		Domain: domain,
		SSL:    true,
		State:  routeEnums.StateManaged,
	})
}
