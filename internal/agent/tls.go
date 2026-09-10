package agent

import (
	"context"
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
// The route is built once and used for both the plan and the work. Building
// it twice is how the plan came to show `proxy_pass http://:0` while the file
// written was correct — the one divergence this whole design exists to make
// impossible.
func (s *Server) tlsRouteFor(ctx context.Context, domain string) (*routeEntities.Route, error) {
	existing, err := s.routes.FindByDomain(ctx, domain)
	if err != nil {
		return nil, err
	}
	if existing == nil {
		return nil, errors.New("no route for that domain; add it first")
	}
	if existing.State == routeEnums.StateUnmanaged {
		return nil, errors.New("that vhost was not created by croft, so croft will not rewrite it: " + existing.File)
	}
	if existing.Target == "" {
		return nil, errors.New("that route has no target to serve; remove it and add it again")
	}

	return routeEntities.NewRoute(routeEntities.RouteProps{
		Domain: domain,
		Target: existing.Target,
		Port:   existing.Port,
		SSL:    true,
		State:  routeEnums.StateManaged,
	}), nil
}

func (s *Server) tlsPlan(route *routeEntities.Route, challenge certificateServices.Challenge) plan.Plan {
	issue := plan.Command(
		"Obtain a certificate from Let's Encrypt ("+string(challenge)+")",
		"croft", "cert", "issue", route.Domain, challengeFlag(challenge))

	steps := []plan.Step{issue}
	steps = append(steps, s.routes.WritePlan(route).Steps...)
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

	route, err := s.tlsRouteFor(r.Context(), domain)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, PlanResponse{Plan: s.tlsPlan(route, chooseChallenge())})
}

func (s *Server) enableTLS(w http.ResponseWriter, r *http.Request) {
	domain := r.PathValue("domain")
	if !domainPattern.MatchString(domain) {
		writeError(w, http.StatusBadRequest, errors.New("that is not a domain name"))
		return
	}

	route, err := s.tlsRouteFor(r.Context(), domain)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	challenge := chooseChallenge()

	s.stream(w, func(report func(int, string)) error {
		report(1, "Obtaining a certificate")

		issuer := certificateServices.NewIssueCertificate(acme.NewIssuer())
		if _, err := issuer.Execute(r.Context(), certificateServices.IssueRequest{
			Domain:    domain,
			Challenge: challenge,
		}, func(text string) { report(1, text) }); err != nil {
			return err
		}

		for i, step := range s.routes.WritePlan(route).Steps {
			report(i+2, step.Describe)
		}
		return s.routes.Write(r.Context(), route)
	})
}
