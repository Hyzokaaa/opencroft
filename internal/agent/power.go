package agent

import (
	"net/http"

	"github.com/Hyzokaaa/opencroft/internal/shared/plan"
)

// Starting and stopping are one command each, but they go through the same
// door as everything else: a plan you can read, then the work.
func (s *Server) planPower(w http.ResponseWriter, r *http.Request, stop bool) {
	name := r.PathValue("name")
	if err := validName(name); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, PlanResponse{Plan: s.powerPlan(name, stop)})
}

func (s *Server) powerPlan(name string, stop bool) plan.Plan {
	if stop {
		return s.instances.StopPlan(name)
	}
	return s.instances.StartPlan(name)
}

func (s *Server) power(w http.ResponseWriter, r *http.Request, stop bool) {
	name := r.PathValue("name")
	if err := validName(name); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	s.stream(w, func(report func(int, string)) error {
		p := s.powerPlan(name, stop)
		for i, step := range p.Steps {
			report(i+1, step.Describe)
		}
		if stop {
			return s.instances.Stop(r.Context(), name)
		}
		return s.instances.Start(r.Context(), name)
	})
}
