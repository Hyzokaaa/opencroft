package agent

import (
	"context"
	"errors"
	"net/http"
	"path"
	"strings"
	"time"

	certificateServices "github.com/Hyzokaaa/opencroft/internal/certificate/domain/services"
	"github.com/Hyzokaaa/opencroft/internal/certificate/infrastructure/acme"
	routeEntities "github.com/Hyzokaaa/opencroft/internal/route/domain/entities"
	routeEnums "github.com/Hyzokaaa/opencroft/internal/route/domain/enums"
	"github.com/Hyzokaaa/opencroft/internal/shared/host"
	"github.com/Hyzokaaa/opencroft/internal/shared/plan"
)

const (
	certbotLive    = "/etc/letsencrypt/live/"
	certbotRenewal = "/etc/letsencrypt/renewal/"
)

// tlsChange is what serving a domain with a certificate croft issues does —
// whether it had none, or had one certbot renews.
//
// The order is forced by how validation works: the http vhost has to exist and
// be serving before the authority is asked, because that is what answers the
// challenge. Once the certificate is in hand, the same file is rewritten to
// serve TLS and redirect http to it.
//
// The route is built once and used for both the plan and the work. Building
// it twice is how the plan came to show `proxy_pass http://:0` while the file
// written was correct — the one divergence this whole design exists to make
// impossible.
type tlsChange struct {
	route *routeEntities.Route
	// certbot's renewal file for the certificate the domain stops using, moved
	// aside once croft's is served. Two programs renewing for one domain is
	// how a rate limit runs out, and certbot's would go on failing — or
	// succeeding, for nothing — twice a day.
	retire string
	moved  string
	// issue is false when a wildcard croft keeps already covers the domain:
	// there is nothing to ask the authority for, only a vhost to write.
	issue bool
}

func (s *Server) tlsChangeFor(ctx context.Context, domain string) (tlsChange, error) {
	all, err := s.routes.FindAll(ctx)
	if err != nil {
		return tlsChange{}, err
	}
	var existing *routeEntities.Route
	for _, route := range all {
		if route.Domain == domain {
			existing = route
		}
	}

	switch {
	case existing == nil:
		return tlsChange{}, errors.New("no route for that domain; add it first")
	case existing.State == routeEnums.StateUnmanaged:
		return tlsChange{}, errors.New("that vhost was not created by croft, so croft will not rewrite it — take it over first: " + existing.File)
	case existing.Target == "":
		return tlsChange{}, errors.New("that route has no target to serve; remove it and add it again")
	case existing.SSL && !strings.HasPrefix(existing.Certificates, certbotLive):
		return tlsChange{}, errors.New(domain + " is already served with a certificate croft issues and renews")
	}

	wildcard, covered, err := s.wildcardFor(ctx, domain)
	if err != nil {
		return tlsChange{}, err
	}
	change := tlsChange{issue: !covered, route: routeEntities.NewRoute(routeEntities.RouteProps{
		Domain:       domain,
		Target:       existing.Target,
		Port:         existing.Port,
		SSL:          true,
		Certificates: wildcard,
		Paths:        existing.Paths,
		State:        routeEnums.StateManaged,
	})}

	if !existing.SSL {
		return change, nil
	}

	// certbot's copy stops being renewed only when nothing else serves it.
	// A certificate is often shared, and letting it lapse would take down
	// whichever vhost still reads it.
	for _, other := range all {
		if other.Domain != domain && other.Certificates == existing.Certificates {
			return change, nil
		}
	}
	renewal := certbotRenewal + path.Base(existing.Certificates) + ".conf"
	if _, err := s.host.ReadFile(ctx, renewal); err != nil {
		return change, nil // certbot is not renewing it anyway
	}
	change.retire = renewal
	change.moved = takenOverDir + "/" + path.Base(renewal) + "." + time.Now().UTC().Format("20060102-150405")
	return change, nil
}

func (s *Server) tlsPlan(change tlsChange, challenge certificateServices.Challenge) plan.Plan {
	issue := plan.Command(
		"Obtain a certificate from Let's Encrypt ("+string(challenge)+"), which croft renews from then on",
		"croft", "cert", "issue", change.route.Domain, challengeFlag(challenge))

	steps := []plan.Step{}
	if change.issue {
		steps = append(steps, issue)
	}
	steps = append(steps, s.routes.WritePlan(change.route).Steps...)
	return plan.New(append(steps, retireSteps(change)...)...)
}

// retireSteps come last: until croft's certificate is being served, certbot's
// is the one keeping the domain up, and it should go on renewing it.
func retireSteps(change tlsChange) []plan.Step {
	if change.retire == "" {
		return nil
	}
	return []plan.Step{
		plan.Command("Make a place for the file croft moves aside", "mkdir", "-p", takenOverDir),
		plan.Command("Stop certbot renewing the certificate nothing serves any more — its files stay, and moving this back undoes it",
			"mv", change.retire, change.moved),
	}
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

func (s *Server) acceptTLS(r *http.Request) (tlsChange, error) {
	domain := r.PathValue("domain")
	if !domainPattern.MatchString(domain) {
		return tlsChange{}, errors.New("that is not a domain name")
	}
	return s.tlsChangeFor(r.Context(), domain)
}

func (s *Server) planTLS(w http.ResponseWriter, r *http.Request) {
	change, err := s.acceptTLS(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, PlanResponse{Plan: s.tlsPlan(change, chooseChallenge())})
}

func (s *Server) enableTLS(w http.ResponseWriter, r *http.Request) {
	change, err := s.acceptTLS(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	ctx := r.Context()
	domain := change.route.Domain
	challenge := chooseChallenge()

	s.stream(w, func(report func(int, string)) error {
		if change.issue {
			report(1, "Obtaining a certificate")

			issuer := certificateServices.NewIssueCertificate(acme.NewIssuer())
			if _, err := issuer.Execute(ctx, certificateServices.IssueRequest{
				Domain:    domain,
				Challenge: challenge,
			}, func(text string) { report(1, text) }); err != nil {
				return err
			}
		}

		// One line for the whole write, because the repository walks the plan
		// itself and will undo what it wrote if nginx refuses it. Announcing
		// each step here would tick them all off before any of them ran.
		// Numbers follow the plan the panel showed, which has no issuing step
		// when a wildcard covers the domain.
		done := 0
		if change.issue {
			done = 1
		}
		report(done+1, "Writing the vhost and reloading nginx")
		if err := s.routes.Write(ctx, change.route); err != nil {
			return err
		}

		done += len(s.routes.WritePlan(change.route).Steps)
		for i, step := range retireSteps(change) {
			report(done+i+1, step.Describe)
			if err := host.RunStep(ctx, s.host, step); err != nil {
				return errors.New(domain + " is served with croft's certificate, but certbot still renews its own: " + err.Error())
			}
		}
		return nil
	})
}

// wildcardFor finds a wildcard croft keeps that answers for the domain: the
// name itself, or one label below it. *.example.com covers app.example.com,
// not api.app.example.com — that is how the authority reads it too.
func (s *Server) wildcardFor(ctx context.Context, domain string) (string, bool, error) {
	found, err := s.certificates.FindAll(ctx)
	if err != nil {
		return "", false, err
	}
	now := time.Now()
	for _, certificate := range found {
		base, ok := strings.CutPrefix(certificate.Domain, "*.")
		if !ok || !certificate.Managed || certificate.DaysLeft(now) <= 0 {
			continue
		}
		if covers(base, domain) {
			return path.Dir(certificate.Path), true, nil
		}
	}
	return "", false, nil
}

func covers(base, domain string) bool {
	label, below := strings.CutSuffix(domain, "."+base)
	return domain == base || (below && label != "" && !strings.Contains(label, "."))
}
