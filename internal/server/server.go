// Package server exposes the HTTP API and serves the embedded interface.
//
// Handlers are entry points, nothing more: they build the Command or Query by
// hand and hand off. The CLI builds the very same ones.
package server

import (
	"encoding/json"
	"errors"
	"io/fs"
	"log"
	"net/http"
	"strings"

	authHttp "github.com/Hyzokaaa/opencroft/internal/auth/infrastructure/http"
	instanceRepositories "github.com/Hyzokaaa/opencroft/internal/instance/domain/repositories"
	instanceServices "github.com/Hyzokaaa/opencroft/internal/instance/domain/services"
	overviewQueries "github.com/Hyzokaaa/opencroft/internal/overview/application/queries"
	routeRepositories "github.com/Hyzokaaa/opencroft/internal/route/domain/repositories"
	routeServices "github.com/Hyzokaaa/opencroft/internal/route/domain/services"
	"github.com/Hyzokaaa/opencroft/internal/shared/host"
	"github.com/Hyzokaaa/opencroft/internal/shared/job"
)

// Deps carries the concrete implementations chosen at startup.
type Deps struct {
	Overview        *overviewQueries.OverviewQuery
	CreateInstance  *instanceServices.CreateInstance
	DestroyInstance *instanceServices.DestroyInstance
	ReadOnly        bool

	// Instances and Host are what a write actually touches; Jobs runs the
	// plan in the background. Simulated swaps execution for a rehearsal, so
	// the plan screen can be shown on a machine with no runtime.
	Instances instanceRepositories.InstanceRepository
	Routes    routeRepositories.RouteRepository
	AddRoute  *routeServices.AddRoute
	EditRoute *routeServices.EditRoute
	Host      host.Host
	Jobs      *job.Runner
	DNS       DNSConfig
	Simulated bool

	// Auth guards every data endpoint. It is required: a nil here would
	// serve the host to anyone who can reach the port.
	Auth *authHttp.Handler
}

func Handler(deps Deps) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /api/health", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})

	// hostId is a parameter from the first day, even though only "local"
	// exists. Adding remote hosts later is then a driver, not a rewrite.
	mux.HandleFunc("GET /api/hosts/{hostId}/overview", func(w http.ResponseWriter, r *http.Request) {
		response, err := deps.Overview.Execute(r.Context())
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, http.StatusOK, response)
	})

	mux.HandleFunc("POST /api/hosts/{hostId}/instances", deps.createInstance)
	mux.HandleFunc("DELETE /api/hosts/{hostId}/instances/{name}", deps.destroyInstance)
	mux.HandleFunc("POST /api/hosts/{hostId}/instances/{name}/start", deps.startInstance)
	mux.HandleFunc("POST /api/hosts/{hostId}/instances/{name}/stop", deps.stopInstance)

	mux.HandleFunc("POST /api/hosts/{hostId}/routes", deps.addRoute)
	mux.HandleFunc("DELETE /api/hosts/{hostId}/routes/{domain}", deps.removeRoute)

	mux.HandleFunc("PUT /api/hosts/{hostId}/routes/{domain}", deps.editRoute)
	mux.HandleFunc("POST /api/hosts/{hostId}/routes/{domain}/tls", deps.enableTLS)

	mux.HandleFunc("GET /api/hosts/{hostId}/dns", deps.showDNS)
	mux.HandleFunc("POST /api/hosts/{hostId}/dns", deps.saveDNS)

	mux.HandleFunc("GET /api/jobs/{id}", deps.showJob)
	mux.HandleFunc("GET /api/jobs/{id}/events", deps.streamJob)

	deps.Auth.Register(mux)
	mux.Handle("/", staticHandler())

	return logging(deps.Auth.Guard(mux))
}

func statusFor(err error) int {
	switch {
	case errors.Is(err, instanceServices.ErrAlreadyExists):
		return http.StatusConflict
	case errors.Is(err, instanceServices.ErrNotFound):
		return http.StatusNotFound
	case errors.Is(err, instanceServices.ErrNameRequired), errors.Is(err, instanceServices.ErrNameInvalid):
		return http.StatusBadRequest
	default:
		return http.StatusInternalServerError
	}
}

func staticHandler() http.Handler {
	dist, err := fs.Sub(assets, "dist")
	if err != nil {
		panic(err)
	}
	files := http.FileServer(http.FS(dist))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Single page app: unknown paths fall back to the entry document.
		if _, err := fs.Stat(dist, strings.TrimPrefix(r.URL.Path, "/")); err != nil {
			r = r.Clone(r.Context())
			r.URL.Path = "/"
		}
		files.ServeHTTP(w, r)
	})
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func writeError(w http.ResponseWriter, status int, err error) {
	writeJSON(w, status, map[string]string{"error": err.Error()})
}

func logging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/api/") {
			next.ServeHTTP(w, r)
			return
		}
		log.Printf("%s %s", r.Method, r.URL.Path)
		next.ServeHTTP(w, r)
	})
}
