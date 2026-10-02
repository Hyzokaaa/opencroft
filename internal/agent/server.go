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
	"sync"
	"time"

	certificateRepositories "github.com/Hyzokaaa/opencroft/internal/certificate/domain/repositories"
	instanceEntities "github.com/Hyzokaaa/opencroft/internal/instance/domain/entities"
	instanceRepositories "github.com/Hyzokaaa/opencroft/internal/instance/domain/repositories"
	projectServices "github.com/Hyzokaaa/opencroft/internal/project/domain/services"
	projectFiles "github.com/Hyzokaaa/opencroft/internal/project/infrastructure/files"
	routeEntities "github.com/Hyzokaaa/opencroft/internal/route/domain/entities"
	routeEnums "github.com/Hyzokaaa/opencroft/internal/route/domain/enums"
	routeRepositories "github.com/Hyzokaaa/opencroft/internal/route/domain/repositories"
	"github.com/Hyzokaaa/opencroft/internal/shared/host"
)

type Server struct {
	instances    instanceRepositories.InstanceRepository
	routes       routeRepositories.RouteRepository
	certificates certificateRepositories.CertificateRepository
	host         host.Host
	flavor       string
	version      string
	defaultImage string

	// bin is the runtime command, lxc or incus. The agent is the only half
	// that knows it, because it is the only half allowed to run it.
	bin string

	turns *turns

	projects *projectServices.Projects

	dns internalDNS
}

func NewServer(
	instances instanceRepositories.InstanceRepository,
	routes routeRepositories.RouteRepository,
	certificates certificateRepositories.CertificateRepository,
	h host.Host,
	flavor string,
	version string,
	bin string,
) *Server {
	return &Server{
		instances:    instances,
		routes:       routes,
		certificates: certificates,
		host:         h,
		flavor:       flavor,
		version:      version,
		defaultImage: instances.DefaultImage(),
		bin:          bin,
		turns:        newTurns(),
		projects:     projectServices.NewProjects(projectFiles.NewFileProjectRepository(h, bin), instances),
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
		writeJSON(w, http.StatusOK, RuntimeResponse{Flavor: s.flavor, DefaultImage: s.defaultImage, Version: s.version})
	})

	mux.HandleFunc("GET /instances", s.listInstances)
	mux.HandleFunc("GET /instances/address", s.allocateAddress)
	mux.HandleFunc("POST /instances/plan", s.planCreate)
	mux.HandleFunc("POST /instances", s.create)
	mux.HandleFunc("GET /instances/{name}/destroy/plan", s.planDestroy)
	mux.HandleFunc("DELETE /instances/{name}", s.destroy)
	mux.HandleFunc("GET /instances/{name}/start/plan", func(w http.ResponseWriter, r *http.Request) { s.planPower(w, r, false) })
	mux.HandleFunc("POST /instances/{name}/start", func(w http.ResponseWriter, r *http.Request) { s.power(w, r, false) })
	mux.HandleFunc("GET /instances/{name}/stop/plan", func(w http.ResponseWriter, r *http.Request) { s.planPower(w, r, true) })
	mux.HandleFunc("POST /instances/{name}/stop", func(w http.ResponseWriter, r *http.Request) { s.power(w, r, true) })
	mux.HandleFunc("GET /routes", s.listRoutes)
	mux.HandleFunc("GET /certificates", s.listCertificates)
	mux.HandleFunc("POST /routes/plan", s.planRoute)
	mux.HandleFunc("POST /routes", s.writeRoute)
	mux.HandleFunc("GET /routes/{domain}/remove/plan", s.planRemoveRoute)
	mux.HandleFunc("DELETE /routes/{domain}", s.removeRoute)
	mux.HandleFunc("GET /routes/{domain}/takeover/plan", s.planTakeOver)
	mux.HandleFunc("POST /routes/{domain}/takeover", s.takeOverDomain)
	mux.HandleFunc("GET /routes/{domain}/tls/plan", s.planTLS)
	mux.HandleFunc("POST /routes/{domain}/tls", s.enableTLS)
	mux.HandleFunc("GET /certificates/wildcard/plan", s.planWildcard)
	mux.HandleFunc("POST /certificates/wildcard", s.issueWildcard)
	mux.HandleFunc("POST /expose/plan", s.planExpose)
	mux.HandleFunc("POST /expose", s.expose)
	mux.HandleFunc("GET /instances/{name}/annotations", s.showAnnotations)
	mux.HandleFunc("GET /instances/{name}/services", s.listServices)
	mux.HandleFunc("POST /instances/{name}/services/inspect/plan", s.planInspect)
	mux.HandleFunc("POST /instances/{name}/services/inspect", s.inspect)
	mux.HandleFunc("POST /instances/{name}/services/deploy/plan", s.planDeploy)
	mux.HandleFunc("POST /instances/{name}/services/deploy", s.deploy)
	mux.HandleFunc("GET /instances/{name}/services/{service}/logs", s.serviceLogs)
	mux.HandleFunc("GET /instances/{name}/services/{service}/destroy/plan", s.planDestroyService)
	mux.HandleFunc("DELETE /instances/{name}/services/{service}", s.destroyService)
	mux.HandleFunc("POST /instances/{name}/services/rollback/plan", s.planRollback)
	mux.HandleFunc("POST /instances/{name}/services/rollback", s.rollback)
	for _, action := range PowerActions {
		verb := string(action)
		mux.HandleFunc("GET /instances/{name}/services/{service}/"+verb+"/plan",
			func(w http.ResponseWriter, r *http.Request) { s.planServicePower(w, r, action) })
		mux.HandleFunc("POST /instances/{name}/services/{service}/"+verb,
			func(w http.ResponseWriter, r *http.Request) { s.servicePower(w, r, action) })
		mux.HandleFunc("GET /instances/{name}/units/{unit}/"+verb+"/plan",
			func(w http.ResponseWriter, r *http.Request) { s.planUnitPower(w, r, action) })
		mux.HandleFunc("POST /instances/{name}/units/{unit}/"+verb,
			func(w http.ResponseWriter, r *http.Request) { s.unitPower(w, r, action) })
	}
	mux.HandleFunc("GET /instances/{name}/services/{service}/env", s.showEnvironment)
	mux.HandleFunc("POST /instances/{name}/services/{service}/env/plan", s.planEnvironment)
	mux.HandleFunc("POST /instances/{name}/services/{service}/env", s.changeEnvironment)
	mux.HandleFunc("GET /instances/{name}/services/{service}/redeploy/plan", s.planRedeploy)
	mux.HandleFunc("POST /instances/{name}/services/{service}/redeploy", s.redeploy)
	mux.HandleFunc("GET /instances/{name}/units/{unit}/logs", s.unitLogs)
	for kind, key := range map[Adoptable]string{AdoptUnit: "{unit}", AdoptSite: "{site}"} {
		base := "/instances/{name}/" + string(kind) + "/" + key
		mux.HandleFunc("GET "+base+"/adoption", s.showAdoption(kind))
		mux.HandleFunc("POST "+base+"/adopt/plan", s.planAdopt(kind))
		mux.HandleFunc("POST "+base+"/adopt", s.adopt(kind))
	}
	mux.HandleFunc("GET /instances/{name}/databases", s.listDatabases)
	mux.HandleFunc("POST /instances/{name}/databases/plan", s.planProvision)
	mux.HandleFunc("POST /instances/{name}/databases", s.provision)
	mux.HandleFunc("GET /instances/{name}/databases/shareable", s.listShareable)
	mux.HandleFunc("POST /instances/{name}/databases/connect/plan", s.planShare)
	mux.HandleFunc("POST /instances/{name}/databases/connect", s.share)
	mux.HandleFunc("GET /instances/{name}/databases/{database}/destroy/plan", s.planDestroyDatabase)
	mux.HandleFunc("DELETE /instances/{name}/databases/{database}", s.destroyDatabase)
	mux.HandleFunc("GET /projects", s.listProjects)
	mux.HandleFunc("PUT /projects/{project}/plan", s.showProjectPlan(s.declarePlan))
	mux.HandleFunc("PUT /projects/{project}", s.runProjectPlan(s.declarePlan))
	mux.HandleFunc("GET /projects/{project}/remove/plan", s.showProjectPlan(s.removeProjectPlan))
	mux.HandleFunc("DELETE /projects/{project}", s.runProjectPlan(s.removeProjectPlan))
	mux.HandleFunc("PUT /instances/{name}/project/plan", s.showProjectPlan(s.assignPlan))
	mux.HandleFunc("PUT /instances/{name}/project", s.runProjectPlan(s.assignPlan))
	mux.HandleFunc("GET /dns", s.showDNS)
	mux.HandleFunc("POST /dns", s.saveDNS)

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
		dto := toDTO(i)
		dto.InternalName = s.internalName(r.Context(), i.Name)
		out = append(out, dto)
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

	s.streamOn(w, r, name, func(report func(int, string)) error {
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

	out := make([]RouteDTO, len(found))
	for i, route := range found {
		out[i] = RouteDTO{
			Domain: route.Domain, Target: route.Target, Port: route.Port,
			SSL: route.SSL, State: string(route.State), File: route.File,
			Certificates: route.Certificates, Paths: toRoutePathDTOs(route.Paths),
		}
	}

	// Asked all at once. A target that is down costs the whole dial timeout,
	// and in a row that is the overview taking a second per broken route — on
	// a page that refreshes every ten.
	var wg sync.WaitGroup
	for i, route := range found {
		wg.Add(1)
		go func() {
			defer wg.Done()
			out[i].Answers = answers(route.Target, route.Port)
		}()
	}
	wg.Wait()

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
	// A prefix goes into a location line of a file nginx loads as root. It
	// ends in a slash so /api/ cannot also swallow /apiary.
	prefixPattern = regexp.MustCompile(`^/([A-Za-z0-9_~-][A-Za-z0-9._~-]*/)+$`)
	// nginx reads the certificate as root, so where from is not open-ended:
	// croft's own directory, or certbot's for a domain it set up first.
	certificatesPattern = regexp.MustCompile(`^(/var/lib/croft/certificates|/etc/letsencrypt/live)/(_\.)?[A-Za-z0-9][A-Za-z0-9.-]*$`)
	memoryPattern       = regexp.MustCompile(`^\d{1,6}(B|KB|MB|GB|TB|KiB|MiB|GiB|TiB)?$`)

	// Loose on purpose: this rejects obvious mistakes, not unusual but valid
	// names. Whether a domain resolves is a question only DNS can answer.
	domainPattern = regexp.MustCompile(`^[a-zA-Z0-9]([a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?(\.[a-zA-Z0-9]([a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?)+$`)
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
	// Only into a project declared here: a name typed wrong would otherwise
	// make one.
	if dto.Project != "" {
		declared, err := s.projects.List(r.Context())
		if err != nil {
			return nil, err
		}
		known := false
		for _, p := range declared {
			known = known || (p.Name == dto.Project && p.Declared)
		}
		if !known {
			return nil, errors.New("there is no declared project called " + dto.Project)
		}
	}

	return instanceEntities.NewInstance(instanceEntities.InstanceProps{
		Id: dto.Id, Name: dto.Name, Image: dto.Image, Address: dto.Address,
		Port: dto.Port, CPULimit: dto.CPULimit, MemLimit: dto.MemLimit,
		Status: dto.Status, Created: dto.Created, Managed: true, Project: dto.Project,
	}), nil
}

func toDTO(i *instanceEntities.Instance) InstanceDTO {
	return InstanceDTO{
		Id: i.GetId(), Name: i.Name, Image: i.Image, Address: i.Address,
		Port: i.Port, Domain: i.Domain, CPULimit: i.CPULimit, MemLimit: i.MemLimit,
		Status: i.Status, Created: i.Created, Managed: i.Managed,
		Project: i.Project,
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

func (s *Server) listCertificates(w http.ResponseWriter, r *http.Request) {
	found, err := s.certificates.FindAll(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	out := make([]CertificateDTO, 0, len(found))
	for _, c := range found {
		out = append(out, CertificateDTO{
			Domain: c.Domain, Names: c.Names, Issuer: c.Issuer,
			NotAfter: c.NotAfter.Format(time.RFC3339), Path: c.Path,
			Managed: c.Managed, SelfSigned: c.SelfSigned,
		})
	}
	writeJSON(w, http.StatusOK, out)
}

// ── Routes ────────────────────────────────────────────────────────────────────

func (s *Server) acceptRoute(r *http.Request) (*routeEntities.Route, error) {
	var dto RouteDTO
	if err := json.NewDecoder(r.Body).Decode(&dto); err != nil {
		return nil, err
	}

	if !domainPattern.MatchString(dto.Domain) {
		return nil, errors.New("that is not a domain name")
	}
	if !addressPattern.MatchString(dto.Target) {
		return nil, errors.New("a route must point at an IPv4 address")
	}
	if dto.Port < 1 || dto.Port > 65535 {
		return nil, errors.New("the port must be between 1 and 65535")
	}
	if dto.Certificates != "" && !certificatesPattern.MatchString(dto.Certificates) {
		return nil, errors.New("a certificate is read from croft's own directory or certbot's, nowhere else")
	}

	paths := make([]routeEntities.PathRoute, 0, len(dto.Paths))
	seen := map[string]bool{}
	for _, p := range dto.Paths {
		if !prefixPattern.MatchString(p.Prefix) || seen[p.Prefix] {
			return nil, errors.New(p.Prefix + " is not a path croft can route")
		}
		if !addressPattern.MatchString(p.Target) {
			return nil, errors.New("a path must point at an IPv4 address")
		}
		if p.Port < 1 || p.Port > 65535 {
			return nil, errors.New("the port must be between 1 and 65535")
		}
		seen[p.Prefix] = true
		paths = append(paths, routeEntities.PathRoute{Prefix: p.Prefix, Target: p.Target, Port: p.Port, Strip: p.Strip})
	}

	return routeEntities.NewRoute(routeEntities.RouteProps{
		Domain: dto.Domain, Target: dto.Target, Port: dto.Port, SSL: dto.SSL,
		Certificates: dto.Certificates, Paths: paths,
	}), nil
}

func toRoutePathDTOs(paths []routeEntities.PathRoute) []PathDTO {
	out := make([]PathDTO, len(paths))
	for i, p := range paths {
		out[i] = PathDTO{Prefix: p.Prefix, Target: p.Target, Port: p.Port, Strip: p.Strip}
	}
	return out
}

func (s *Server) planRoute(w http.ResponseWriter, r *http.Request) {
	route, err := s.acceptRoute(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, PlanResponse{Plan: s.routes.WritePlan(route)})
}

func (s *Server) writeRoute(w http.ResponseWriter, r *http.Request) {
	route, err := s.acceptRoute(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if err := s.routes.Write(r.Context(), route); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) planRemoveRoute(w http.ResponseWriter, r *http.Request) {
	domain := r.PathValue("domain")
	if !domainPattern.MatchString(domain) {
		writeError(w, http.StatusBadRequest, errors.New("that is not a domain name"))
		return
	}
	writeJSON(w, http.StatusOK, PlanResponse{Plan: s.routes.RemovePlan(domain)})
}

// removeRoute refuses to delete a file it did not write. Somebody else's vhost
// is somebody else's business, and the panel says so rather than silently
// declining.
func (s *Server) removeRoute(w http.ResponseWriter, r *http.Request) {
	domain := r.PathValue("domain")
	if !domainPattern.MatchString(domain) {
		writeError(w, http.StatusBadRequest, errors.New("that is not a domain name"))
		return
	}

	existing, err := s.routes.FindByDomain(r.Context(), domain)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	if existing == nil {
		writeError(w, http.StatusNotFound, errors.New("no route for that domain"))
		return
	}
	if existing.State == routeEnums.StateUnmanaged {
		writeError(w, http.StatusForbidden,
			errors.New("that vhost was not created by croft, so croft will not remove it: "+existing.File))
		return
	}

	if err := s.routes.Remove(r.Context(), domain); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
