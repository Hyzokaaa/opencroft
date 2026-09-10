package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	routeServices "github.com/Hyzokaaa/opencroft/internal/route/domain/services"
)

type editRouteRequest struct {
	Target string `json:"target"`
	Port   int    `json:"port"`
}

func (d Deps) editRoute(w http.ResponseWriter, r *http.Request) {
	if d.ReadOnly {
		writeError(w, http.StatusForbidden, errors.New("this instance is read-only"))
		return
	}

	var body editRouteRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	domain := r.PathValue("domain")
	route, p, err := d.EditRoute.Prepare(r.Context(), routeServices.EditRouteProps{
		Domain: domain,
		Target: body.Target,
		Port:   body.Port,
	})
	if err != nil {
		writeError(w, editStatusFor(err), err)
		return
	}

	if wantsPlan(r) {
		scheme := "http"
		if route.SSL {
			scheme = "https"
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"summary": fmt.Sprintf("Serve %s from %s:%d over %s. The certificate is untouched.",
				route.Domain, route.Target, route.Port, scheme),
			"plan": p,
		})
		return
	}

	started := d.Jobs.Start("route-edit", domain, p, func(ctx context.Context, report func(int, string)) error {
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

func editStatusFor(err error) int {
	switch {
	case errors.Is(err, routeServices.ErrRouteUnknown), errors.Is(err, routeServices.ErrTargetUnknown):
		return http.StatusNotFound
	case errors.Is(err, routeServices.ErrRouteForeign):
		return http.StatusForbidden
	default:
		return http.StatusBadRequest
	}
}
