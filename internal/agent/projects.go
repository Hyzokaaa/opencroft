package agent

import (
	"encoding/json"
	"errors"
	"net/http"

	projectEntities "github.com/Hyzokaaa/opencroft/internal/project/domain/entities"
	projectServices "github.com/Hyzokaaa/opencroft/internal/project/domain/services"
	"github.com/Hyzokaaa/opencroft/internal/shared/host"
	"github.com/Hyzokaaa/opencroft/internal/shared/plan"
)

// A project is containers that share a label, and a declaration on the host
// so that one can exist before its first container. Both are written here,
// on the privileged side: the label with the runtime's own command, the
// declaration in /etc/croft/projects. The panel names what it wants.

type ProjectDTO struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Declared    bool     `json:"declared"`
	Instances   []string `json:"instances"`
}

type AssignmentDTO struct {
	Project string `json:"project"`
}

func toProjectDTO(p projectEntities.Project) ProjectDTO {
	instances := p.Instances
	if instances == nil {
		instances = []string{}
	}
	return ProjectDTO{Name: p.Name, Description: p.Description, Declared: p.Declared, Instances: instances}
}

// projectStatus says whose mistake a refusal was: a request that cannot be
// done as asked is the caller's, anything else is this side's.
func projectStatus(err error) int {
	for _, known := range []error{
		projectEntities.ErrNameInvalid, projectEntities.ErrDescriptionInvalid,
		projectServices.ErrProjectNotEmpty, projectServices.ErrUndeclared, projectServices.ErrNothingToChange,
	} {
		if errors.Is(err, known) {
			return http.StatusBadRequest
		}
	}
	if errors.Is(err, projectServices.ErrProjectUnknown) || errors.Is(err, projectServices.ErrInstanceUnknown) {
		return http.StatusNotFound
	}
	return http.StatusInternalServerError
}

func (s *Server) listProjects(w http.ResponseWriter, r *http.Request) {
	found, err := s.projects.List(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	out := make([]ProjectDTO, 0, len(found))
	for _, p := range found {
		out = append(out, toProjectDTO(p))
	}
	writeJSON(w, http.StatusOK, out)
}

// projectPlan reads what is asked and answers with its plan, the same one for
// showing and for running.
type projectPlan func(r *http.Request) (plan.Plan, error)

func (s *Server) declarePlan(r *http.Request) (plan.Plan, error) {
	var body ProjectDTO
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		return plan.Plan{}, projectEntities.ErrNameInvalid
	}
	return s.projects.PrepareDeclare(r.Context(), projectEntities.Declaration{
		Name: r.PathValue("project"), Description: body.Description,
	})
}

func (s *Server) removeProjectPlan(r *http.Request) (plan.Plan, error) {
	return s.projects.PrepareRemove(r.Context(), r.PathValue("project"))
}

func (s *Server) assignPlan(r *http.Request) (plan.Plan, error) {
	name := r.PathValue("name")
	if err := validName(name); err != nil {
		return plan.Plan{}, err
	}
	var body AssignmentDTO
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		return plan.Plan{}, err
	}
	return s.projects.PrepareAssign(r.Context(), name, body.Project)
}

func (s *Server) showProjectPlan(prepare projectPlan) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		p, err := prepare(r)
		if err != nil {
			writeError(w, projectStatus(err), err)
			return
		}
		writeJSON(w, http.StatusOK, PlanResponse{Plan: p})
	}
}

// runProjectPlan decides again rather than trusting the plan the panel was
// shown: between the two, somebody may have put a container in the project
// that was about to be removed.
func (s *Server) runProjectPlan(prepare projectPlan) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		p, err := prepare(r)
		if err != nil {
			writeError(w, projectStatus(err), err)
			return
		}
		ctx := r.Context()
		s.stream(w, func(report func(int, string)) error {
			for i, step := range p.Steps {
				report(i+1, step.Describe)
				if err := host.RunStep(ctx, s.host, step); err != nil {
					return errors.New(step.Describe + ": " + err.Error())
				}
			}
			return nil
		})
	}
}
