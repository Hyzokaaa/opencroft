package agent

import (
	"context"
	"errors"
	"net/http"
	"path"
	"strings"

	routeEntities "github.com/Hyzokaaa/opencroft/internal/route/domain/entities"
	routeEnums "github.com/Hyzokaaa/opencroft/internal/route/domain/enums"
	"github.com/Hyzokaaa/opencroft/internal/shared/host"
	"github.com/Hyzokaaa/opencroft/internal/shared/plan"
)

// An alias is one more name a route answers on — a customer's own domain,
// pointed at this server, answering exactly as the route does. It is added
// the way https is turned on, because it usually needs both: the name is
// served over http first, so that the authority can reach it, then its
// certificate is obtained and the name moves to https.
//
// A certificate that cannot be issued yet — the customer's DNS does not point
// here yet — leaves the name served over http, and adding it again later
// retries just the certificate.

type aliasChange struct {
	route *routeEntities.Route
	alias routeEntities.Alias
	// issue is true when a certificate has to be obtained for the alias; a
	// wildcard croft keeps may already cover it.
	issue bool
}

// owned is the route an alias may be added to: one croft writes.
func (s *Server) owned(ctx context.Context, domain string) (*routeEntities.Route, []*routeEntities.Route, error) {
	all, err := s.routes.FindAll(ctx)
	if err != nil {
		return nil, nil, err
	}
	for _, route := range all {
		if route.Domain == domain {
			if route.State != routeEnums.StateManaged {
				return nil, nil, errors.New(domain + " is not a route croft manages — take it over first")
			}
			return route, all, nil
		}
	}
	return nil, nil, errors.New("no route for " + domain)
}

func (s *Server) aliasAddition(ctx context.Context, domain, alias string) (aliasChange, error) {
	if !domainPattern.MatchString(alias) || strings.Contains(alias, "*") {
		return aliasChange{}, errors.New("that is not a domain name")
	}
	route, all, err := s.owned(ctx, domain)
	if err != nil {
		return aliasChange{}, err
	}
	if alias == domain {
		return aliasChange{}, errors.New(alias + " is the route's own domain")
	}
	if existing, ok := route.Alias(alias); ok && (existing.SSL || !route.SSL) {
		return aliasChange{}, errors.New(alias + " already answers here")
	}
	for _, other := range all {
		if other.Domain == domain {
			continue
		}
		if other.Domain == alias {
			return aliasChange{}, errors.New(alias + " is already a domain of its own here")
		}
		if _, ok := other.Alias(alias); ok {
			return aliasChange{}, errors.New(alias + " already answers for " + other.Domain)
		}
	}

	change := aliasChange{route: route, alias: routeEntities.Alias{Domain: alias}}
	if !route.SSL {
		return change, nil
	}
	if wildcard, covered, err := s.wildcardFor(ctx, alias); err == nil && covered {
		change.alias = routeEntities.Alias{Domain: alias, SSL: true, Certificates: wildcard}
		return change, nil
	}
	change.issue = true
	return change, nil
}

func (s *Server) aliasPlan(change aliasChange) plan.Plan {
	steps := []plan.Step{}
	if change.issue {
		// Served over http first: that is how the authority reaches it.
		steps = append(steps, s.routes.WritePlan(change.route.WithAlias(routeEntities.Alias{Domain: change.alias.Domain})).Steps...)
		steps = append(steps,
			plan.Command("Check that "+change.alias.Domain+" reaches this server before asking Let's Encrypt — if its DNS "+
				"does not point here yet, it waits, served over http, and croft checks again every few minutes",
				"curl", "-fsS", "http://"+change.alias.Domain+"/.well-known/acme-challenge/croft-check"),
			plan.Command("Obtain a certificate for "+change.alias.Domain+" from Let's Encrypt, proved over http",
				"croft", "cert", "issue", change.alias.Domain, "--http"))
		change.alias.SSL = true
	}
	return plan.New(append(steps, s.routes.WritePlan(change.route.WithAlias(change.alias)).Steps...)...)
}

func (s *Server) acceptAlias(r *http.Request) (aliasChange, error) {
	domain := r.PathValue("domain")
	if !domainPattern.MatchString(domain) {
		return aliasChange{}, errors.New("that is not a domain name")
	}
	return s.aliasAddition(r.Context(), domain, r.PathValue("alias"))
}

func (s *Server) planAddAlias(w http.ResponseWriter, r *http.Request) {
	change, err := s.acceptAlias(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, PlanResponse{Plan: s.aliasPlan(change)})
}

func (s *Server) addAlias(w http.ResponseWriter, r *http.Request) {
	change, err := s.acceptAlias(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	ctx := r.Context()
	alias := change.alias.Domain

	s.stream(w, func(report func(int, string)) error {
		if !change.issue {
			// One line for the whole write, because the repository walks
			// the plan itself and puts the file back if nginx refuses it.
			report(1, "Serving "+alias+" and reloading nginx")
			return s.routes.Write(ctx, change.route.WithAlias(change.alias))
		}

		report(1, "Serving "+alias+" over http")
		if err := s.routes.Write(ctx, change.route.WithAlias(routeEntities.Alias{Domain: alias})); err != nil {
			return err
		}
		report(4, "Checking that "+alias+" reaches this server")
		err := s.certifyAlias(ctx, change.route.Domain, alias, func(text string) { report(5, text) })
		if isWaiting(err) {
			// Not a failure: the customer has not pointed their DNS here
			// yet. It is served over http meanwhile, and croft keeps looking.
			report(4, "Waiting for its DNS: "+err.Error()+". croft checks again every 5 minutes and turns on https by itself.")
			return nil
		}
		if err != nil {
			return errors.New(alias + " answers over http, but its certificate could not be issued: " + err.Error())
		}
		report(6, "Serving "+alias+" over https and reloading nginx")
		return nil
	})
}

// ── Letting a name go ────────────────────────────────────────────────────────

func (s *Server) aliasRemoval(ctx context.Context, domain, alias string) (plan.Plan, error) {
	route, _, err := s.owned(ctx, domain)
	if err != nil {
		return plan.Plan{}, err
	}
	existing, ok := route.Alias(alias)
	if !ok {
		return plan.Plan{}, errors.New(alias + " is not an alias of " + domain)
	}

	steps := s.routes.WritePlan(route.WithoutAlias(alias)).Steps
	// Its own certificate goes with it, or renewal would keep a certificate
	// for a name nothing serves. A wildcard it was borrowing stays.
	if existing.SSL && existing.CertDir() == "/var/lib/croft/certificates/"+alias {
		steps = append(steps, plan.Optional("Remove the certificate croft obtained for "+alias,
			"rm", "-rf", path.Clean(existing.CertDir())))
	}
	return plan.New(steps...), nil
}

func (s *Server) planRemoveAlias(w http.ResponseWriter, r *http.Request) {
	p, err := s.aliasRemoval(r.Context(), r.PathValue("domain"), r.PathValue("alias"))
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, PlanResponse{Plan: p})
}

func (s *Server) removeAlias(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	p, err := s.aliasRemoval(ctx, r.PathValue("domain"), r.PathValue("alias"))
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	s.stream(w, func(report func(int, string)) error {
		route, _, err := s.owned(ctx, r.PathValue("domain"))
		if err != nil {
			return err
		}
		report(1, "Writing the vhost without "+r.PathValue("alias")+" and reloading nginx")
		if err := s.routes.Write(ctx, route.WithoutAlias(r.PathValue("alias"))); err != nil {
			return err
		}
		for i, step := range p.Steps {
			if step.Optional {
				report(i+1, step.Describe)
				_ = host.RunStep(ctx, s.host, step)
			}
		}
		return nil
	})
}
