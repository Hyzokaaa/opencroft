package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	deployEntities "github.com/Hyzokaaa/opencroft/internal/deploy/domain/entities"
	deployServices "github.com/Hyzokaaa/opencroft/internal/deploy/domain/services"
	"github.com/Hyzokaaa/opencroft/internal/deploy/infrastructure/container"
	"github.com/Hyzokaaa/opencroft/internal/instance/infrastructure/runtime"
	"github.com/Hyzokaaa/opencroft/internal/shared/host"
	"github.com/Hyzokaaa/opencroft/internal/shared/plan"
)

// Deploying happens in two plans, not one.
//
// You cannot know how to build code you have not seen, so the first plan says
// "fetch it so I can look" and the second says exactly what building and
// running it will do. Both are read before either runs. A single plan with a
// decision in the middle would be approving commands that do not exist yet,
// which is the thing every buildpack does and the thing we said we would not.
//
// A container can run several services. Everything about one is stored on the
// container itself, under its own name, so that losing our database costs
// nothing and moving the container carries its deployments with it.

// index lists the services on a container. Without it there is no way to
// enumerate annotations, only to ask for ones whose names are already known.
const index = "services"

func annotation(service, key string) string {
	return "service." + service + "." + key
}

// full is the key as the runtime sees it, for reading back.
func full(service, key string) string {
	return runtime.AnnotationPrefix + "." + annotation(service, key)
}

var (
	// Loose, but it rules out an argument that is a flag or a command.
	repoPattern    = regexp.MustCompile(`^https://[A-Za-z0-9._~:/?#@!$&'()*+,;=%-]{3,512}$`)
	sshRepoPattern = regexp.MustCompile(`^(git@|ssh://)[A-Za-z0-9._~:/?#@!$&'()*+,;=%-]{3,512}$`)
	branchPattern  = regexp.MustCompile(`^[A-Za-z0-9._/-]{1,128}$`)
	commitPattern  = regexp.MustCompile(`^[0-9a-f]{7,40}$`)
	pathPattern    = regexp.MustCompile(`^/[A-Za-z0-9._/-]{1,128}$`)
	servicePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,30}$`)
	packagePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9.+:-]{0,127}$`)
	envKeyPattern  = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]{0,127}$`)
	healthPattern  = regexp.MustCompile(`^/[A-Za-z0-9._~:/?#@!$&'()*+,;=%-]{0,255}$`)
)

// ── What crosses the socket ───────────────────────────────────────────────────

type HealthDTO struct {
	Path     string `json:"path"`
	Status   int    `json:"status"`
	Contains string `json:"contains"`
}

type ServiceDTO struct {
	Name    string `json:"name"`
	Repo    string `json:"repo"`
	Branch  string `json:"branch"`
	Path    string `json:"path"`
	Runtime string `json:"runtime"`

	Install  []string          `json:"install"`
	Build    []string          `json:"build"`
	Start    string            `json:"start"`
	Port     int               `json:"port"`
	Packages []string          `json:"packages"`
	Env      map[string]string `json:"env,omitempty"`
	Health   HealthDTO         `json:"health"`

	// Commit is what is deployed right now, and Healthy is the snapshot that
	// last passed its check. Between them they are the two ways back.
	Commit  string `json:"commit,omitempty"`
	Healthy string `json:"healthy,omitempty"`

	// State is read from the container, not remembered. The machine is the
	// source of truth about whether something is running.
	State string `json:"state,omitempty"`
}

type ServicesResponse struct {
	Services []ServiceDTO `json:"services"`
	// Snapshots is everything on the container, ours and yours. The panel
	// needs both to say what a rollback would reach.
	Snapshots []string `json:"snapshots"`
}

// ── Validation ────────────────────────────────────────────────────────────────

func validService(name string) error {
	if !servicePattern.MatchString(name) {
		return errors.New("a service name is lowercase letters, digits and dashes")
	}
	return nil
}

// oneLine is the one restriction on a command. The unit file is written with a
// heredoc, so a newline in the start command would produce a broken unit and a
// confusing failure much later.
func oneLine(command string) error {
	if strings.ContainsAny(command, "\n\r\x00") {
		return errors.New("a command has to be one line")
	}
	return nil
}

func (dto ServiceDTO) entity() (*deployEntities.Service, error) {
	if err := validService(dto.Name); err != nil {
		return nil, err
	}

	if sshRepoPattern.MatchString(dto.Repo) {
		return nil, errors.New(
			"that repository needs a key, and deploy keys are not implemented yet — use an https URL of a public repository")
	}
	if !repoPattern.MatchString(dto.Repo) {
		return nil, errors.New("that does not look like a repository URL")
	}

	branch := dto.Branch
	if branch == "" {
		branch = "main"
	}
	if !branchPattern.MatchString(branch) {
		return nil, errors.New("that is not a branch name")
	}
	if dto.Commit != "" && !commitPattern.MatchString(dto.Commit) {
		return nil, errors.New("that is not a commit")
	}

	path := dto.Path
	if path == "" {
		path = deployEntities.DefaultPath(dto.Name)
	}
	if !pathPattern.MatchString(path) {
		return nil, errors.New("that is not a path inside the container")
	}

	for _, command := range append(append([]string{dto.Start}, dto.Install...), dto.Build...) {
		if err := oneLine(command); err != nil {
			return nil, err
		}
	}
	for _, name := range dto.Packages {
		if !packagePattern.MatchString(name) {
			return nil, errors.New(name + " is not a package name")
		}
	}
	if dto.Port < 0 || dto.Port > 65535 {
		return nil, errors.New("the port must be between 1 and 65535")
	}

	env, err := environment(dto.Env)
	if err != nil {
		return nil, err
	}

	health, err := wanted(dto.Health)
	if err != nil {
		return nil, err
	}
	if health.Wanted() && dto.Port < 1 {
		return nil, errors.New("a health check needs a port to ask on")
	}

	return deployEntities.NewService(deployEntities.ServiceProps{
		Name: dto.Name,
		Path: path,
		Source: deployEntities.Source{
			Repo: dto.Repo, Branch: branch, Commit: dto.Commit,
			Auth: deployEntities.AuthNone,
		},
		Install: dto.Install, Build: dto.Build, Start: strings.TrimSpace(dto.Start),
		Runtime: dto.Runtime, Port: dto.Port, Packages: dto.Packages,
		Env: env, Health: health,
	}), nil
}

// environment is checked hard because it is written into a file that a shell
// reads. A key that is not a key, or a value spanning lines, would end the
// heredoc early and leave the rest as commands.
func environment(env map[string]string) (map[string]string, error) {
	out := map[string]string{}

	for key, value := range env {
		if !envKeyPattern.MatchString(key) {
			return nil, errors.New(key + " is not an environment variable name")
		}
		if strings.ContainsAny(value, "\n\r\x00") {
			return nil, errors.New(key + " has a value spanning several lines")
		}
		if strings.Contains(value, "CROFT_ENV") {
			return nil, errors.New(key + " contains the marker that ends the file")
		}
		out[key] = value
	}
	return out, nil
}

func wanted(dto HealthDTO) (deployEntities.Health, error) {
	health := deployEntities.Health{
		Path: strings.TrimSpace(dto.Path), Status: dto.Status, Contains: dto.Contains,
	}
	if !health.Wanted() {
		return deployEntities.Health{}, nil
	}

	if !healthPattern.MatchString(health.Path) {
		return health, errors.New("a health check is a path beginning with /")
	}
	if health.Status != 0 && (health.Status < 100 || health.Status > 599) {
		return health, errors.New("that is not a status code")
	}
	// It goes inside a shell case pattern, so it cannot carry the characters
	// that would end one.
	if strings.ContainsAny(health.Contains, "\n\r\x00*?[]()|&;$`\"'\\") {
		return health, errors.New("the expected text cannot contain shell characters")
	}
	return health, nil
}

// ── Reading what is on the container ──────────────────────────────────────────

func (s *Server) get(ctx context.Context, name, key string) string {
	out, err := s.host.Run(ctx, s.bin, "config", "get", name, key)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(out.Stdout)
}

// names reads the index, and falls back to the single unnamed deployment that
// version 0.15 wrote. Upgrading should not lose what is already deployed.
func (s *Server) names(ctx context.Context, container string) []string {
	listed := strings.Fields(s.get(ctx, container, runtime.AnnotationPrefix+"."+index))
	if len(listed) > 0 {
		return listed
	}
	if s.get(ctx, container, runtime.AnnotationPrefix+".app.repo") != "" {
		return []string{"app"}
	}
	return nil
}

// readService prefers the current keys and falls back to the old ones, so a
// deployment made before services had names keeps working.
func (s *Server) readService(ctx context.Context, container, name string) ServiceDTO {
	read := func(key string) string {
		if value := s.get(ctx, container, full(name, key)); value != "" {
			return value
		}
		if name == "app" {
			return s.get(ctx, container, runtime.AnnotationPrefix+".app."+key)
		}
		return ""
	}

	port, _ := strconv.Atoi(read("port"))
	status, _ := strconv.Atoi(read("health-status"))

	return ServiceDTO{
		Name: name, Repo: read("repo"), Branch: read("branch"), Path: read("path"),
		Runtime: read("runtime"), Start: read("start"), Port: port,
		Install:  commands(read("install")),
		Build:    commands(read("build")),
		Packages: strings.Fields(read("packages")),
		Env:      decodeEnv(read("env")),
		Health: HealthDTO{
			Path: read("health"), Status: status, Contains: read("health-contains"),
		},
		Commit:  read("commit"),
		Healthy: read("healthy"),
	}
}

func commands(joined string) []string {
	if strings.TrimSpace(joined) == "" {
		return nil
	}
	out := []string{}
	for _, command := range strings.Split(joined, " && ") {
		out = append(out, strings.TrimSpace(command))
	}
	return out
}

// The environment is stored as JSON in one annotation rather than one
// annotation per variable: it is written and read as a whole, and a partial
// write would leave a service with half its configuration.
func decodeEnv(stored string) map[string]string {
	if strings.TrimSpace(stored) == "" {
		return nil
	}
	env := map[string]string{}
	if err := json.Unmarshal([]byte(stored), &env); err != nil {
		return nil
	}
	return env
}

// remember writes the deployment onto the container.
func (s *Server) remember(ctx context.Context, container string, service *deployEntities.Service) error {
	encoded, err := json.Marshal(service.Env)
	if err != nil {
		return err
	}

	values := map[string]string{
		"repo": service.Source.Repo, "branch": service.Source.Branch,
		"commit": service.Source.Commit, "path": service.Path,
		"runtime": service.Runtime, "start": service.Start,
		"port":            strconv.Itoa(service.Port),
		"install":         strings.Join(service.Install, " && "),
		"build":           strings.Join(service.Build, " && "),
		"packages":        strings.Join(service.Packages, " "),
		"health":          service.Health.Path,
		"health-contains": service.Health.Contains,
	}
	if service.Health.Status != 0 {
		values["health-status"] = strconv.Itoa(service.Health.Status)
	}
	if len(service.Env) > 0 {
		values["env"] = string(encoded)
	}

	for key, value := range values {
		if value != "" {
			if err := s.instances.Annotate(ctx, container, annotation(service.Name, key), value); err != nil {
				return err
			}
		}
	}
	return s.enrol(ctx, container, service.Name)
}

// enrol adds the service to the index if it is not already there.
func (s *Server) enrol(ctx context.Context, container, name string) error {
	listed := s.names(ctx, container)
	for _, existing := range listed {
		if existing == name {
			return nil
		}
	}

	listed = append(listed, name)
	sort.Strings(listed)
	return s.instances.Annotate(ctx, container, index, strings.Join(listed, " "))
}

// snapshots asks the runtime rather than remembering, because a person can
// take and remove them without us.
func (s *Server) snapshots(ctx context.Context, container string) []string {
	out, err := s.host.Run(ctx, s.bin, "query", "/1.0/instances/"+container+"/snapshots")
	if err != nil {
		return nil
	}

	var urls []string
	if err := json.Unmarshal([]byte(out.Stdout), &urls); err != nil {
		return nil
	}

	names := make([]string, 0, len(urls))
	for _, url := range urls {
		if at := strings.LastIndex(url, "/"); at >= 0 {
			names = append(names, url[at+1:])
		}
	}
	sort.Strings(names)
	return names
}

// state is read, never remembered.
func (s *Server) state(ctx context.Context, container string, service *deployEntities.Service) string {
	out, err := s.host.Run(ctx, s.bin, "exec", container, "--",
		"systemctl", "is-active", service.Unit())
	if answer := strings.TrimSpace(out.Stdout); answer != "" {
		return answer
	}
	if err != nil {
		return "unknown"
	}
	return "unknown"
}

// ── The planner ───────────────────────────────────────────────────────────────

func (s *Server) planner(ctx context.Context, name string) (*container.Planner, error) {
	instance, err := s.instances.FindByName(ctx, name)
	if err != nil {
		return nil, err
	}
	if instance == nil {
		return nil, errors.New("there is no container called " + name)
	}
	return container.NewPlanner(s.bin, name, instance.Address), nil
}

func (s *Server) deployment(ctx context.Context, name string, service *deployEntities.Service) container.Deployment {
	return container.Deployment{
		Service:  service,
		At:       time.Now(),
		Existing: s.snapshots(ctx, name),
		Healthy:  s.get(ctx, name, full(service.Name, "healthy")),
	}
}

// accept turns the request into something the agent is willing to act on.
func (s *Server) acceptService(r *http.Request) (string, *deployEntities.Service, error) {
	name := r.PathValue("name")
	if err := validName(name); err != nil {
		return "", nil, err
	}

	var dto ServiceDTO
	if err := json.NewDecoder(r.Body).Decode(&dto); err != nil {
		return "", nil, err
	}
	if dto.Name == "" {
		dto.Name = r.PathValue("service")
	}

	service, err := dto.entity()
	if err != nil {
		return "", nil, err
	}
	return name, service, nil
}

// ── Handlers ──────────────────────────────────────────────────────────────────

func (s *Server) listServices(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if err := validName(name); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	ctx := r.Context()
	out := []ServiceDTO{}

	for _, service := range s.names(ctx, name) {
		dto := s.readService(ctx, name, service)
		if entity, err := dto.entity(); err == nil {
			dto.State = s.state(ctx, name, entity)
		}
		out = append(out, dto)
	}

	writeJSON(w, http.StatusOK, ServicesResponse{Services: out, Snapshots: s.snapshots(ctx, name)})
}

// inspectPlan is deliberately small: a snapshot, the tools, and a checkout.
// Nothing is built and nothing is started, so approving it commits you to very
// little — and the snapshot means even that is reversible.
func (s *Server) inspectPlan(ctx context.Context, name string, service *deployEntities.Service) (plan.Plan, error) {
	planner, err := s.planner(ctx, name)
	if err != nil {
		return plan.Plan{}, err
	}

	// Reusing the opening of the deployment means the two plans cannot drift.
	bare := *service
	bare.Install, bare.Build, bare.Env = nil, nil, nil
	bare.Health = deployEntities.Health{}

	full := planner.Deploy(s.deployment(ctx, name, &bare)).Steps
	return plan.New(full[:3]...), nil
}

func (s *Server) planInspect(w http.ResponseWriter, r *http.Request) {
	name, service, err := s.acceptService(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	p, err := s.inspectPlan(r.Context(), name, service)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, PlanResponse{Plan: p})
}

type DetectionDTO struct {
	Found    bool     `json:"found"`
	Runtime  string   `json:"runtime"`
	Why      string   `json:"why"`
	Install  []string `json:"install"`
	Build    []string `json:"build"`
	Start    string   `json:"start"`
	Port     int      `json:"port"`
	Packages []string `json:"packages"`
	Files    []string `json:"files"`
	Commit   string   `json:"commit"`
}

func (s *Server) inspect(w http.ResponseWriter, r *http.Request) {
	name, service, err := s.acceptService(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	ctx := r.Context()
	p, err := s.inspectPlan(ctx, name, service)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	for _, step := range p.Steps {
		if err := host.RunStep(ctx, s.host, step); err != nil {
			writeError(w, http.StatusInternalServerError, fmt.Errorf("%s: %w", step.Describe, err))
			return
		}
	}

	found, detection := s.look(ctx, name, service)
	writeJSON(w, http.StatusOK, DetectionDTO{
		Found:   found,
		Runtime: detection.Runtime, Why: detection.Why,
		Install: detection.Install, Build: detection.Build,
		Start: detection.Start, Port: detection.Port,
		Packages: detection.Packages,
		Files:    s.list(ctx, name, service.Path),
		Commit:   s.commit(ctx, name, service),
	})
}

// look reads the few files worth reading. Anything more would be guessing
// about a project we can just ask.
func (s *Server) look(ctx context.Context, name string, service *deployEntities.Service) (bool, deployServices.Detection) {
	files := map[string]bool{}
	for _, entry := range s.list(ctx, name, service.Path) {
		files[entry] = true
	}

	contents := map[string]string{}
	if files["package.json"] {
		out, err := s.host.Run(ctx, s.bin, "exec", name, "--", "cat", service.Path+"/package.json")
		if err == nil {
			contents["package.json"] = out.Stdout
		}
	}

	detection, ok := deployServices.Detect(deployServices.Repository{Files: files, Contents: contents})
	if !ok {
		detection.Why = "nothing recognisable at the root, so the commands have to be given"
	}
	return ok, detection
}

func (s *Server) list(ctx context.Context, name, path string) []string {
	out, err := s.host.Run(ctx, s.bin, "exec", name, "--", "ls", "-A", path)
	if err != nil {
		return nil
	}
	return strings.Fields(out.Stdout)
}

// commit records what was actually checked out, which is what going back to
// the previous version needs.
func (s *Server) commit(ctx context.Context, name string, service *deployEntities.Service) string {
	out, err := s.host.Run(ctx, s.bin, "exec", name, "--",
		"git", "-C", service.Path, "rev-parse", "HEAD")
	if err != nil {
		return ""
	}
	return strings.TrimSpace(out.Stdout)
}

func (s *Server) planDeploy(w http.ResponseWriter, r *http.Request) {
	name, service, err := s.acceptService(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if !service.Runnable() {
		writeError(w, http.StatusBadRequest,
			errors.New("nothing says how to start it, so there is no deployment to run"))
		return
	}

	planner, err := s.planner(r.Context(), name)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, PlanResponse{
		Plan: planner.Deploy(s.deployment(r.Context(), name, service)),
	})
}

// deploy walks the very plan that planDeploy returned. What is recorded on the
// container, and what counts as the last version that worked, are both written
// only once the plan has run through.
func (s *Server) deploy(w http.ResponseWriter, r *http.Request) {
	name, service, err := s.acceptService(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if !service.Runnable() {
		writeError(w, http.StatusBadRequest,
			errors.New("nothing says how to start it, so there is no deployment to run"))
		return
	}

	ctx := r.Context()
	planner, err := s.planner(ctx, name)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	deployment := s.deployment(ctx, name, service)
	steps := planner.Deploy(deployment).Steps
	snapshot := deployEntities.SnapshotName(service.Name, deployment.At)

	s.stream(w, func(report func(int, string)) error {
		for i, step := range steps {
			report(i+1, step.Describe)

			if err := host.RunStep(ctx, s.host, step); err != nil {
				if step.Optional {
					continue
				}
				// Remembering a failed deployment would make the panel show
				// commands that are not what is running.
				return fmt.Errorf("%s: %w", step.Describe, err)
			}
		}

		service.Source.Commit = s.commit(ctx, name, service)
		if err := s.remember(ctx, name, service); err != nil {
			return err
		}

		// Only now is this a version known to work, and only now is it worth
		// keeping as the one to come back to.
		if service.Health.Wanted() {
			_ = s.instances.Annotate(ctx, name, annotation(service.Name, "healthy"), snapshot)
		}
		return nil
	})
}

func (s *Server) serviceLogs(w http.ResponseWriter, r *http.Request) {
	name, service, err := s.stored(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	lines := 200
	if asked, err := strconv.Atoi(r.URL.Query().Get("lines")); err == nil && asked > 0 && asked <= 2000 {
		lines = asked
	}

	planner, err := s.planner(r.Context(), name)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	step := planner.Logs(service, lines).Steps[0]
	out, _ := s.host.Run(r.Context(), step.Argv[0], step.Argv[1:]...)
	writeJSON(w, http.StatusOK, LogsResponse{Lines: out.Stdout})
}

// stored reads a service the container already knows about, for the operations
// that act on what is deployed rather than on what was asked for.
func (s *Server) stored(r *http.Request) (string, *deployEntities.Service, error) {
	name := r.PathValue("name")
	if err := validName(name); err != nil {
		return "", nil, err
	}

	service := r.PathValue("service")
	if err := validService(service); err != nil {
		return "", nil, err
	}

	dto := s.readService(r.Context(), name, service)
	if dto.Repo == "" {
		return "", nil, errors.New(service + " is not deployed on " + name)
	}

	entity, err := dto.entity()
	if err != nil {
		return "", nil, err
	}
	return name, entity, nil
}

// ── Going back ────────────────────────────────────────────────────────────────

type rollbackRequest struct {
	Snapshot string `json:"snapshot"`
}

// rollbackPlan restores a snapshot, which takes the whole container back —
// every service in it, and everything written since. The plan says so, and
// says it louder when there is more than one service to lose.
func (s *Server) rollbackPlan(ctx context.Context, name, snapshot string) (plan.Plan, string, error) {
	if !deployEntities.Ours(snapshot) && !containsSnapshot(s.snapshots(ctx, name), snapshot) {
		return plan.Plan{}, "", errors.New("there is no snapshot called " + snapshot + " on " + name)
	}

	planner, err := s.planner(ctx, name)
	if err != nil {
		return plan.Plan{}, "", err
	}

	warning := ""
	if others := s.names(ctx, name); len(others) > 1 {
		warning = "This restores the whole container, so " + strings.Join(others, ", ") +
			" all go back — along with anything written since."
	}
	return planner.Rollback(snapshot), warning, nil
}

func containsSnapshot(all []string, want string) bool {
	for _, snapshot := range all {
		if snapshot == want {
			return true
		}
	}
	return false
}

func (s *Server) acceptRollback(r *http.Request) (string, string, error) {
	name := r.PathValue("name")
	if err := validName(name); err != nil {
		return "", "", err
	}

	var body rollbackRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		return "", "", err
	}
	if strings.TrimSpace(body.Snapshot) == "" {
		return "", "", errors.New("which snapshot to restore is required")
	}
	if strings.ContainsAny(body.Snapshot, " \t\n;|&$`/") {
		return "", "", errors.New("that is not a snapshot name")
	}
	return name, body.Snapshot, nil
}

type RollbackResponse struct {
	Plan    plan.Plan `json:"plan"`
	Warning string    `json:"warning,omitempty"`
}

func (s *Server) planRollback(w http.ResponseWriter, r *http.Request) {
	name, snapshot, err := s.acceptRollback(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	p, warning, err := s.rollbackPlan(r.Context(), name, snapshot)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, RollbackResponse{Plan: p, Warning: warning})
}

func (s *Server) rollback(w http.ResponseWriter, r *http.Request) {
	name, snapshot, err := s.acceptRollback(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	ctx := r.Context()
	p, _, err := s.rollbackPlan(ctx, name, snapshot)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	s.stream(w, func(report func(int, string)) error {
		for i, step := range p.Steps {
			report(i+1, step.Describe)
			if err := host.RunStep(ctx, s.host, step); err != nil {
				return fmt.Errorf("%s: %w", step.Describe, err)
			}
		}
		return nil
	})
}
