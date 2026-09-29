package agent

import (
	"context"
	"errors"
	"net/http"
	"path"
	"strings"
	"time"

	routeEntities "github.com/Hyzokaaa/opencroft/internal/route/domain/entities"
	routeEnums "github.com/Hyzokaaa/opencroft/internal/route/domain/enums"
	"github.com/Hyzokaaa/opencroft/internal/shared/host"
	"github.com/Hyzokaaa/opencroft/internal/shared/plan"
)

// Taking over a domain is adopting its vhost. croft writes its own for the
// domain — the same target, the same paths, the same certificate, so nothing a
// visitor sees changes — and moves the hand-written file out of nginx's way,
// into a directory nothing includes. Putting it back is one mv, and the plan
// says which.
//
// It is never deleted, and a file that also serves other domains is not moved
// at all: that would take them down with it.
//
// Everything here is decided on this side, from the configuration nginx
// actually loaded. The panel names a domain and nothing else — least of all
// which file to move.

const takenOverDir = "/var/lib/croft/taken-over"

type takeOver struct {
	route    *routeEntities.Route
	original string
	moved    string
}

func (s *Server) takeOverOf(ctx context.Context, domain string) (takeOver, error) {
	all, err := s.routes.FindAll(ctx)
	if err != nil {
		return takeOver{}, err
	}

	var existing *routeEntities.Route
	for _, route := range all {
		if route.Domain == domain {
			existing = route
		}
	}

	switch {
	case existing == nil:
		return takeOver{}, errors.New("nginx serves nothing for " + domain)
	case existing.State != routeEnums.StateUnmanaged:
		return takeOver{}, errors.New("croft already manages " + domain)
	case existing.Target == "":
		return takeOver{}, errors.New(domain + " does not pass requests anywhere croft can point at, so there is nothing to carry over")
	case existing.SSL && existing.Certificates == "":
		return takeOver{}, errors.New(domain + "'s certificate is not a fullchain.pem and privkey.pem pair croft can read")
	case existing.SSL && !certificatesPattern.MatchString(existing.Certificates):
		return takeOver{}, errors.New(domain + " reads its certificate from " + existing.Certificates + ", which croft does not point nginx at")
	case path.Base(existing.File) == "nginx.conf":
		return takeOver{}, errors.New(domain + " is served from nginx.conf itself, which croft will not move")
	}

	for _, other := range all {
		if other.File == existing.File && other.Domain != domain {
			return takeOver{}, errors.New(existing.File + " also serves " + other.Domain +
				"; moving it would take that down too, so split it first")
		}
	}

	route := routeEntities.NewRoute(routeEntities.RouteProps{
		Domain: domain, Target: existing.Target, Port: existing.Port, SSL: existing.SSL,
		Certificates: existing.Certificates, Paths: existing.Paths, State: routeEnums.StateManaged,
	})
	moved := takenOverDir + "/" + path.Base(existing.File) + "." + time.Now().UTC().Format("20060102-150405")
	return takeOver{route: route, original: existing.File, moved: moved}, nil
}

func (s *Server) takeOverPlan(t takeOver) plan.Plan {
	steps := []plan.Step{
		plan.Command("Make a place for the file croft moves aside", "mkdir", "-p", takenOverDir),
		plan.Command("Move "+t.original+" out of nginx's way — moving it back undoes this",
			"mv", t.original, t.moved),
	}
	return plan.New(append(steps, s.routes.WritePlan(t.route).Steps...)...)
}

func (s *Server) acceptTakeOver(r *http.Request) (takeOver, error) {
	domain := r.PathValue("domain")
	if !domainPattern.MatchString(domain) {
		return takeOver{}, errors.New("that is not a domain name")
	}
	return s.takeOverOf(r.Context(), domain)
}

func (s *Server) planTakeOver(w http.ResponseWriter, r *http.Request) {
	t, err := s.acceptTakeOver(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, PlanResponse{Plan: s.takeOverPlan(t)})
}

// takeOverDomain walks the plan and, if anything after the move fails, puts
// the original back and removes croft's file — so nginx is left exactly as it
// was found, rather than with neither.
func (s *Server) takeOverDomain(w http.ResponseWriter, r *http.Request) {
	t, err := s.acceptTakeOver(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	ctx := r.Context()
	p := s.takeOverPlan(t)
	s.stream(w, func(report func(int, string)) error {
		moved, written := false, ""
		for i, step := range p.Steps {
			report(i+1, step.Describe)
			if err := host.RunStep(ctx, s.host, step); err != nil {
				if written != "" {
					_ = s.host.RemoveFile(ctx, written)
				}
				if moved {
					_, _ = s.host.Run(ctx, "mv", t.moved, t.original)
				}
				return errors.New(strings.TrimSpace(step.Describe+": "+err.Error()) +
					" (" + t.original + " is back where it was, so nginx is unchanged)")
			}
			if len(step.Argv) > 0 && step.Argv[0] == "mv" {
				moved = true
			}
			if step.IsFile() {
				written = step.File
			}
		}
		return nil
	})
}
