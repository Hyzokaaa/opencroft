package server

import (
	"context"
	"fmt"
	"net/http"

	routeEntities "github.com/Hyzokaaa/opencroft/internal/route/domain/entities"
	routeServices "github.com/Hyzokaaa/opencroft/internal/route/domain/services"
	"github.com/Hyzokaaa/opencroft/internal/shared/plan"
)

type setPathRequest struct {
	Prefix string `json:"prefix"`
	Target string `json:"target"`
	Port   int    `json:"port"`
	Strip  bool   `json:"strip"`
}

// setPath sends a prefix of a domain somewhere of its own — /api/ to a
// backend, say — or points one that already does somewhere else.
func (d Deps) setPath(w http.ResponseWriter, r *http.Request) {
	if !d.writable(w) {
		return
	}

	var body setPathRequest
	if err := readBody(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	domain := r.PathValue("domain")
	route, p, err := d.RoutePaths.PrepareSet(r.Context(), routeServices.SetPathProps{
		Domain: domain, Prefix: body.Prefix, Target: body.Target, Port: body.Port, Strip: body.Strip,
	})
	if err != nil {
		writeError(w, editStatusFor(err), err)
		return
	}

	summary := fmt.Sprintf("Send %s%s to %s:%d", domain, body.Prefix, body.Target, pathPort(route, body.Prefix))
	if body.Strip {
		summary += fmt.Sprintf(", without the %s in front", body.Prefix)
	}
	d.writeRouteJob(w, r, "route-path", domain, route, p,
		summary+". The rest of the domain keeps going where it went, and websockets pass through.")
}

// removePath sends a prefix back to wherever the rest of its domain goes.
func (d Deps) removePath(w http.ResponseWriter, r *http.Request) {
	if !d.writable(w) {
		return
	}

	domain, prefix := r.PathValue("domain"), r.URL.Query().Get("prefix")
	route, p, err := d.RoutePaths.PrepareRemove(r.Context(), domain, prefix)
	if err != nil {
		writeError(w, editStatusFor(err), err)
		return
	}

	d.writeRouteJob(w, r, "route-path", domain, route, p,
		fmt.Sprintf("Serve %s%s like the rest of %s again.", domain, prefix, domain))
}

// writeRouteJob shows a route's plan or writes it, the way every route write
// does: the steps narrated, the file written by the half allowed to.
func (d Deps) writeRouteJob(w http.ResponseWriter, r *http.Request, kind, domain string,
	route *routeEntities.Route, p plan.Plan, summary string) {
	if wantsPlan(r) {
		writeJSON(w, http.StatusOK, map[string]any{"summary": summary, "plan": p})
		return
	}

	started := d.Jobs.Start(kind, domain, p, func(ctx context.Context, report func(int, string)) error {
		if d.Simulated {
			if err := d.rehearse(p, report); err != nil {
				return err
			}
			return d.Routes.Write(ctx, route)
		}
		for i, step := range p.Steps {
			report(i+1, step.Describe)
		}
		return d.Routes.Write(ctx, route)
	})

	writeJSON(w, http.StatusAccepted, map[string]string{"jobId": started.Id, "domain": domain})
}

func pathPort(route *routeEntities.Route, prefix string) int {
	if p, ok := route.Path(prefix); ok {
		return p.Port
	}
	return route.Port
}
