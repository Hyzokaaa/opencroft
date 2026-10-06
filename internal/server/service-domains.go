package server

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	routeEntities "github.com/Hyzokaaa/opencroft/internal/route/domain/entities"
	routeEnums "github.com/Hyzokaaa/opencroft/internal/route/domain/enums"
	routeServices "github.com/Hyzokaaa/opencroft/internal/route/domain/services"
	"github.com/Hyzokaaa/opencroft/internal/shared/plan"
)

// A domain is given to a service, not to a vhost. Which file it ends up in is
// croft's business: the service's domains already lead somewhere — a route
// croft manages, reaching its container on its port — and a new name joins
// that route, paths included. A service nobody can reach yet gets a route of
// its own, served over https in the same go.

type serviceDomainRequest struct {
	Domain string `json:"domain"`
}

// entryRoute is where a service's domains lead now: the managed route that
// reaches its address and port, one on https first. nil when there is none.
func entryRoute(routes []*routeEntities.Route, address string, port int) *routeEntities.Route {
	var found *routeEntities.Route
	for _, route := range routes {
		if route.State != routeEnums.StateManaged || route.Target != address || route.Port != port {
			continue
		}
		if route.SSL {
			return route
		}
		if found == nil {
			found = route
		}
	}
	return found
}

// mountedAt is the first path of a managed route that leads to address:port,
// as "domain/prefix/", or "" when none does.
func mountedAt(routes []*routeEntities.Route, address string, port int) string {
	for _, route := range routes {
		if route.State != routeEnums.StateManaged {
			continue
		}
		for _, p := range route.Paths {
			if p.Target == address && p.Port == port {
				return route.Domain + p.Prefix
			}
		}
	}
	return ""
}

// servicePort is where the service listens: its own port, or — for a site
// served by the container's web server — the container's, and that web
// server's port 80 when the container was set up by hand and has none.
func (d Deps) servicePort(ctx context.Context, container, service string) (string, int, error) {
	instance, err := d.Instances.FindByName(ctx, container)
	if err != nil {
		return "", 0, err
	}
	if instance == nil {
		return "", 0, fmt.Errorf("there is no container %s", container)
	}
	deployer, ok := d.Services.(Deployer)
	if !ok || d.Services == nil {
		return "", 0, errors.New("this host cannot deploy services")
	}
	found, err := deployer.FindAll(ctx, container)
	if err != nil {
		return "", 0, err
	}
	for _, s := range found.Services {
		if s.Name != service {
			continue
		}
		port := s.Port
		if port == 0 {
			port = instance.Port
		}
		if port == 0 && s.Adopted != nil && s.Adopted.Site != "" {
			port = 80
		}
		if port == 0 {
			return "", 0, fmt.Errorf("%s listens on no port croft knows of, so a domain has nowhere to lead", service)
		}
		return instance.Address, port, nil
	}
	return "", 0, fmt.Errorf("there is no service %s in %s", service, container)
}

func (d Deps) addServiceDomain(w http.ResponseWriter, r *http.Request) {
	if !d.writable(w) {
		return
	}
	var body serviceDomainRequest
	if err := readBody(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	ctx := r.Context()
	container, service := r.PathValue("name"), r.PathValue("service")

	address, port, err := d.servicePort(ctx, container, service)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	routes, err := d.Routes.FindAll(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	if entry := entryRoute(routes, address, port); entry != nil {
		d.joinRoute(w, r, entry, service, container, body.Domain)
		return
	}
	d.newServiceRoute(w, r, address, port, service, container, body.Domain, mountedAt(routes, address, port))
}

// joinRoute adds the name to the route the service's domains already share.
func (d Deps) joinRoute(w http.ResponseWriter, r *http.Request, entry *routeEntities.Route, service, container, domain string) {
	aliaser, ok := d.aliaser(w)
	if !ok {
		return
	}
	p, err := aliaser.AliasPlan(r.Context(), entry.Domain, domain)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if wantsPlan(r) {
		summary := fmt.Sprintf("Make %s reach %s in %s, the same way %s does — paths and websockets included.",
			domain, service, container, entry.Domain)
		if entry.SSL {
			summary += " It gets a certificate of its own once its DNS points to this server; until then it " +
				"answers over http, and croft checks again every 5 minutes."
		}
		writeJSON(w, http.StatusOK, map[string]any{"summary": summary, "plan": p})
		return
	}
	started := d.Jobs.Start("route-alias", entry.Domain+" + "+domain, p, func(ctx context.Context, report func(int, string)) error {
		if d.Simulated {
			return d.rehearse(p, report)
		}
		return aliaser.AddAlias(ctx, entry.Domain, domain, report)
	})
	writeJSON(w, http.StatusAccepted, map[string]string{"jobId": started.Id, "domain": entry.Domain})
}

// newServiceRoute is the service's first domain: a route of its own, and https
// right after, once the name is seen to reach this server.
func (d Deps) newServiceRoute(w http.ResponseWriter, r *http.Request, address string, port int, service, container, domain, mount string) {
	// AddRoute is told the container by name; it reads the address itself.
	route, p, err := d.AddRoute.Prepare(r.Context(), routeServices.AddRouteProps{
		Domain: domain, Target: container, Port: port,
	})
	if err != nil {
		writeError(w, routeStatusFor(err), err)
		return
	}
	enabler, secure := d.Routes.(TLSEnabler)
	steps := p.Steps
	if secure {
		steps = append(steps, plan.Command("Check that "+route.Domain+" reaches this server, then serve it over "+
			"https: with a wildcard croft keeps if one covers it, otherwise with a certificate from Let's Encrypt",
			"croft", "cert", "issue", route.Domain, "--http"))
	}
	full := plan.New(steps...)

	if wantsPlan(r) {
		summary := fmt.Sprintf("Make %s reach %s in %s, on port %d.", route.Domain, service, container, port)
		if secure {
			summary += " It is served over http first, then over https once croft sees it reach this server."
		}
		// A service reached only as a path of another domain — an API behind
		// /api/ — is half of an app. A domain of its own leads to it alone,
		// which is right for an API and wrong for a customer's copy of the app.
		if mount != "" {
			summary += fmt.Sprintf(" Careful: %s is reached today as %s, behind what %s serves at its root. "+
				"This domain would lead to %s alone. To give the whole app another domain, add it to the "+
				"service at that root instead.", service, mount, strings.SplitN(mount, "/", 2)[0], service)
		}
		writeJSON(w, http.StatusOK, map[string]any{"summary": summary, "plan": full})
		return
	}

	started := d.Jobs.Start("route", route.Domain, full, func(ctx context.Context, report func(int, string)) error {
		if d.Simulated {
			if err := d.rehearse(full, report); err != nil {
				return err
			}
			return d.Routes.Write(ctx, route)
		}
		for i, step := range p.Steps {
			report(i+1, step.Describe)
		}
		if err := d.Routes.Write(ctx, route); err != nil {
			return err
		}
		if !secure {
			return nil
		}
		offset := len(p.Steps)
		if err := enabler.EnableTLS(ctx, route.Domain, func(step int, text string) { report(offset+1, text) }); err != nil {
			return fmt.Errorf("%s answers over http, but not over https yet: %w", route.Domain, err)
		}
		return nil
	})
	writeJSON(w, http.StatusAccepted, map[string]string{"jobId": started.Id, "domain": route.Domain})
}
