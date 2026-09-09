package agent

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	instanceEntities "github.com/Hyzokaaa/opencroft/internal/instance/domain/entities"
	instanceRepositories "github.com/Hyzokaaa/opencroft/internal/instance/domain/repositories"
	routeRepositories "github.com/Hyzokaaa/opencroft/internal/route/domain/repositories"
	"github.com/Hyzokaaa/opencroft/internal/shared/host"
)

type Server struct {
	instances    instanceRepositories.InstanceRepository
	routes       routeRepositories.RouteRepository
	host         host.Host
	flavor       string
	defaultImage string
}

func NewServer(
	instances instanceRepositories.InstanceRepository,
	routes routeRepositories.RouteRepository,
	h host.Host,
	flavor string,
) *Server {
	return &Server{
		instances:    instances,
		routes:       routes,
		host:         h,
		flavor:       flavor,
		defaultImage: instances.DefaultImage(),
	}
}

// Listen creates the socket with the given group ownership. Membership of that
// group is the whole access control: there is no password, because a unix
// socket already knows who is on the other end.
func Listen(path, group string) (net.Listener, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	// A stale socket from a killed process would refuse to bind.
	_ = os.Remove(path)

	listener, err := net.Listen("unix", path)
	if err != nil {
		return nil, err
	}

	if err := os.Chmod(path, 0o660); err != nil {
		listener.Close()
		return nil, err
	}

	if group != "" {
		if err := chownToGroup(path, group); err != nil {
			listener.Close()
			return nil, fmt.Errorf("giving group %q access to the socket: %w", group, err)
		}
	}
	return listener, nil
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /runtime", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, RuntimeResponse{Flavor: s.flavor, DefaultImage: s.defaultImage})
	})

	mux.HandleFunc("GET /instances", s.listInstances)
	mux.HandleFunc("GET /instances/address", s.allocateAddress)
	mux.HandleFunc("POST /instances/plan", s.planCreate)
	mux.HandleFunc("POST /instances", s.create)
	mux.HandleFunc("GET /instances/{name}/destroy/plan", s.planDestroy)
	mux.HandleFunc("DELETE /instances/{name}", s.destroy)
	mux.HandleFunc("GET /routes", s.listRoutes)

	return mux
}

func (s *Server) listInstances(w http.ResponseWriter, r *http.Request) {
	found, err := s.instances.FindAll(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	out := make([]InstanceDTO, 0, len(found))
	for _, i := range found {
		out = append(out, toDTO(i))
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) allocateAddress(w http.ResponseWriter, r *http.Request) {
	address, err := s.instances.AllocateAddress(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, AddressResponse{Address: address})
}

func (s *Server) planCreate(w http.ResponseWriter, r *http.Request) {
	instance, err := s.accept(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, PlanResponse{Plan: s.instances.CreatePlan(instance)})
}

func (s *Server) create(w http.ResponseWriter, r *http.Request) {
	instance, err := s.accept(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	s.stream(w, func(report func(int, string)) error {
		if narrator, ok := s.instances.(reporter); ok {
			return narrator.CreateWithProgress(r.Context(), instance, report)
		}
		return s.instances.Create(r.Context(), instance)
	})
}

func (s *Server) planDestroy(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if err := validName(name); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, PlanResponse{Plan: s.instances.DeletePlan(name)})
}

func (s *Server) destroy(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if err := validName(name); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	s.stream(w, func(report func(int, string)) error {
		if narrator, ok := s.instances.(reporter); ok {
			return narrator.DeleteWithProgress(r.Context(), name, report)
		}
		return s.instances.Delete(r.Context(), name)
	})
}

func (s *Server) listRoutes(w http.ResponseWriter, r *http.Request) {
	found, err := s.routes.FindAll(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	out := make([]RouteDTO, 0, len(found))
	for _, route := range found {
		out = append(out, RouteDTO{
			Domain: route.Domain, Target: route.Target, Port: route.Port,
			SSL: route.SSL, State: string(route.State), File: route.File,
		})
	}
	writeJSON(w, http.StatusOK, out)
}

// ── Validation ────────────────────────────────────────────────────────────────
//
// Everything arriving over the socket is checked again here. The API validates
// too, but the agent cannot assume the API is the one calling.

var (
	namePattern    = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9-]{0,62}$`)
	imagePattern   = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._:/-]{0,127}$`)
	addressPattern = regexp.MustCompile(`^\d{1,3}(\.\d{1,3}){3}$`)
	memoryPattern  = regexp.MustCompile(`^\d{1,6}(B|KB|MB|GB|TB|KiB|MiB|GiB|TiB)?$`)
)

func validName(name string) error {
	if !namePattern.MatchString(name) {
		return errors.New("that name is not a valid container name")
	}
	return nil
}

// accept turns the request body into an entity the agent is willing to act on.
// A field that fails here never reaches a command line.
func (s *Server) accept(r *http.Request) (*instanceEntities.Instance, error) {
	var dto InstanceDTO
	if err := json.NewDecoder(r.Body).Decode(&dto); err != nil {
		return nil, err
	}

	if err := validName(dto.Name); err != nil {
		return nil, err
	}
	if dto.Image == "" {
		dto.Image = s.defaultImage
	}
	if !imagePattern.MatchString(dto.Image) {
		return nil, errors.New("that image name is not acceptable")
	}
	if !addressPattern.MatchString(dto.Address) {
		return nil, errors.New("that is not an IPv4 address")
	}
	if dto.Port < 1 || dto.Port > 65535 {
		return nil, errors.New("the port must be between 1 and 65535")
	}
	if dto.CPULimit < 1 || dto.CPULimit > 1024 {
		return nil, errors.New("the CPU limit is out of range")
	}
	if !memoryPattern.MatchString(dto.MemLimit) {
		return nil, errors.New("that is not a memory limit")
	}
	if strings.ContainsAny(dto.Id+dto.Created, " \t\n;|&$`") {
		return nil, errors.New("unacceptable metadata")
	}

	return instanceEntities.NewInstance(instanceEntities.InstanceProps{
		Id: dto.Id, Name: dto.Name, Image: dto.Image, Address: dto.Address,
		Port: dto.Port, CPULimit: dto.CPULimit, MemLimit: dto.MemLimit,
		Status: dto.Status, Created: dto.Created, Managed: true,
	}), nil
}

func toDTO(i *instanceEntities.Instance) InstanceDTO {
	return InstanceDTO{
		Id: i.GetId(), Name: i.Name, Image: i.Image, Address: i.Address,
		Port: i.Port, Domain: i.Domain, CPULimit: i.CPULimit, MemLimit: i.MemLimit,
		Status: i.Status, Created: i.Created, Managed: i.Managed,
	}
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func writeError(w http.ResponseWriter, status int, err error) {
	writeJSON(w, status, ErrorResponse{Error: err.Error()})
}
