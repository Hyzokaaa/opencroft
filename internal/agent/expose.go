package agent

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	certificateServices "github.com/Hyzokaaa/opencroft/internal/certificate/domain/services"
	"github.com/Hyzokaaa/opencroft/internal/certificate/infrastructure/acme"
	routeEntities "github.com/Hyzokaaa/opencroft/internal/route/domain/entities"
	routeEnums "github.com/Hyzokaaa/opencroft/internal/route/domain/enums"
	"github.com/Hyzokaaa/opencroft/internal/shared/plan"
)

// Loopback is where the panel listens. It is never reachable from outside;
// nginx is what the internet talks to, and it terminates TLS.
const Loopback = "127.0.0.1"

// exposeRoute is the panel treated as any other service: a domain, a
// certificate, a vhost. Nothing about it is special-cased, which is the point
// — if this path were different from the one users get, it would rot.
func exposeRoute(domain string, port int) *routeEntities.Route {
	return routeEntities.NewRoute(routeEntities.RouteProps{
		Domain: domain,
		Target: Loopback,
		Port:   port,
		SSL:    true,
		State:  routeEnums.StateManaged,
	})
}

func (s *Server) exposePlan(domain string, port int, challenge certificateServices.Challenge) plan.Plan {
	steps := []plan.Step{
		plan.Command("Obtain a certificate from Let's Encrypt ("+string(challenge)+")",
			"croft", "cert", "issue", domain, challengeFlag(challenge)),
	}
	steps = append(steps, s.routes.WritePlan(exposeRoute(domain, port)).Steps...)
	return plan.New(steps...)
}

type exposeRequest struct {
	Domain string `json:"domain"`
	Port   int    `json:"port"`
}

func (s *Server) acceptExpose(r *http.Request) (string, int, error) {
	var body exposeRequest
	if err := decode(r, &body); err != nil {
		return "", 0, err
	}

	if !domainPattern.MatchString(body.Domain) {
		return "", 0, errors.New("that is not a domain name")
	}
	if body.Port < 1 || body.Port > 65535 {
		return "", 0, errors.New("the port must be between 1 and 65535")
	}

	// Refusing to shadow a domain something else already answers on matters
	// more here than anywhere: getting it wrong takes the panel offline along
	// with whatever was there.
	existing, err := s.routes.FindByDomain(r.Context(), body.Domain)
	if err != nil {
		return "", 0, err
	}
	if existing != nil && existing.State == routeEnums.StateUnmanaged {
		return "", 0, fmt.Errorf("something else already serves %s: %s", body.Domain, existing.File)
	}

	return body.Domain, body.Port, nil
}

func (s *Server) planExpose(w http.ResponseWriter, r *http.Request) {
	domain, port, err := s.acceptExpose(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, PlanResponse{Plan: s.exposePlan(domain, port, chooseChallenge())})
}

func (s *Server) expose(w http.ResponseWriter, r *http.Request) {
	domain, port, err := s.acceptExpose(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	challenge := chooseChallenge()
	route := exposeRoute(domain, port)

	s.stream(w, func(report func(int, string)) error {
		report(1, "Obtaining a certificate for "+domain)

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

func decode(r *http.Request, into any) error {
	return json.NewDecoder(r.Body).Decode(into)
}
