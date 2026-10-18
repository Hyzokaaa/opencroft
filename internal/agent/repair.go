package agent

import (
	"net/http"

	"github.com/Hyzokaaa/opencroft/internal/shared/host"
	"github.com/Hyzokaaa/opencroft/internal/shared/plan"
)

// The checkout is repaired from what the container records — its path, its
// repository, its branch — never from the request, which only names the
// service. A request that could choose the path would choose what is deleted.
func (s *Server) repairPlan(r *http.Request) (string, plan.Plan, error) {
	name, service, err := s.recorded(r)
	if err != nil {
		return "", plan.Plan{}, err
	}
	planner, err := s.planner(r.Context(), name)
	if err != nil {
		return "", plan.Plan{}, err
	}
	return name, planner.Repair(service), nil
}

func (s *Server) planRepair(w http.ResponseWriter, r *http.Request) {
	_, p, err := s.repairPlan(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, PlanResponse{Plan: p})
}

func (s *Server) repair(w http.ResponseWriter, r *http.Request) {
	name, p, err := s.repairPlan(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	ctx := r.Context()
	s.streamOn(w, r, name, func(report func(int, string)) error {
		for i, step := range p.Steps {
			report(i+1, step.Describe)
			if err := host.RunStep(ctx, s.host, step); err != nil {
				return explain(step, err)
			}
		}
		return nil
	})
}
