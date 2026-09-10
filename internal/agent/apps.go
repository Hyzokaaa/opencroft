package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	appEntities "github.com/Hyzokaaa/opencroft/internal/app/domain/entities"
	appServices "github.com/Hyzokaaa/opencroft/internal/app/domain/services"
	"github.com/Hyzokaaa/opencroft/internal/app/infrastructure/container"
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

const appPrefix = "app."

type inspectRequest struct {
	Repo   string `json:"repo"`
	Branch string `json:"branch"`
	Path   string `json:"path"`
}

// Loose, but it rules out an argument that is a flag or a command.
var repoPattern = regexp.MustCompile(`^(https://|git@|ssh://)[A-Za-z0-9._~:/?#@!$&'()*+,;=%-]{3,512}$`)

var branchPattern = regexp.MustCompile(`^[A-Za-z0-9._/-]{1,128}$`)

var pathPattern = regexp.MustCompile(`^/[A-Za-z0-9._/-]{1,128}$`)

func (s *Server) acceptInspect(r *http.Request) (string, *appEntities.App, error) {
	name := r.PathValue("name")
	if err := validName(name); err != nil {
		return "", nil, err
	}

	var body inspectRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		return "", nil, err
	}

	app, err := accepted(body)
	if err != nil {
		return "", nil, err
	}
	return name, app, nil
}

// accepted is the part both plans share: where the code comes from and where
// it lands. It is checked here rather than at the edge, because the agent
// cannot assume the panel is the one calling.
func accepted(body inspectRequest) (*appEntities.App, error) {
	if !repoPattern.MatchString(body.Repo) {
		return nil, errors.New("that does not look like a repository URL")
	}
	if body.Branch == "" {
		body.Branch = "main"
	}
	if !branchPattern.MatchString(body.Branch) {
		return nil, errors.New("that is not a branch name")
	}
	if body.Path == "" {
		body.Path = "/srv/app"
	}
	if !pathPattern.MatchString(body.Path) {
		return nil, errors.New("that is not a path inside the container")
	}

	source := appEntities.Source{Repo: body.Repo, Branch: body.Branch, Auth: appEntities.AuthNone}
	if source.Private() {
		return nil, errors.New(
			"that repository needs a key, and deploy keys are not implemented yet — use an https URL of a public repository")
	}

	return appEntities.NewApp(appEntities.AppProps{Path: body.Path, Source: source}), nil
}

// inspectPlan is deliberately small: a snapshot, git, and a clone. Nothing is
// built and nothing is started, so approving it commits you to very little —
// and the snapshot means even that is reversible.
func (s *Server) inspectPlan(name string, app *appEntities.App, at time.Time) plan.Plan {
	planner := container.NewPlanner(s.bin, name)
	full := planner.Deploy(app, at).Steps

	// The first three steps of a deployment are exactly "make it safe, get
	// git, get the code". Reusing them means the two plans cannot drift.
	return plan.New(full[:3]...)
}

func (s *Server) planInspect(w http.ResponseWriter, r *http.Request) {
	name, app, err := s.acceptInspect(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, PlanResponse{Plan: s.inspectPlan(name, app, time.Now())})
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
}

func (s *Server) inspect(w http.ResponseWriter, r *http.Request) {
	name, app, err := s.acceptInspect(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	ctx := r.Context()
	for _, step := range s.inspectPlan(name, app, time.Now()).Steps {
		if _, err := s.host.Run(ctx, step.Argv[0], step.Argv[1:]...); err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
	}

	found, detection := s.look(ctx, name, app)
	s.remember(ctx, name, appServices.Apply(detection, app))

	writeJSON(w, http.StatusOK, DetectionDTO{
		Found:   found,
		Runtime: detection.Runtime, Why: detection.Why,
		Install: detection.Install, Build: detection.Build,
		Start: detection.Start, Port: detection.Port,
		Packages: detection.Packages,
		Files:    s.list(ctx, name, app.Path),
	})
}

// look reads the few files worth reading. Anything more would be guessing
// about a project we can just ask.
func (s *Server) look(ctx context.Context, name string, app *appEntities.App) (bool, appServices.Detection) {
	files := map[string]bool{}
	for _, entry := range s.list(ctx, name, app.Path) {
		files[entry] = true
	}

	contents := map[string]string{}
	if files["package.json"] {
		out, err := s.host.Run(ctx, s.bin, "exec", name, "--", "cat", app.Path+"/package.json")
		if err == nil {
			contents["package.json"] = out.Stdout
		}
	}

	return detectOrEmpty(appServices.Repository{Files: files, Contents: contents})
}

func detectOrEmpty(repo appServices.Repository) (bool, appServices.Detection) {
	detection, ok := appServices.Detect(repo)
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

// remember writes the deployment onto the container. Losing our database then
// costs nothing, and migrating the container carries the deployment with it.
func (s *Server) remember(ctx context.Context, name string, app *appEntities.App) {
	values := map[string]string{
		"repo":     app.Source.Repo,
		"branch":   app.Source.Branch,
		"path":     app.Path,
		"runtime":  app.Runtime,
		"install":  strings.Join(app.Install, " && "),
		"build":    strings.Join(app.Build, " && "),
		"start":    app.Start,
		"port":     strconv.Itoa(app.Port),
		"packages": strings.Join(app.Packages, " "),
	}

	for key, value := range values {
		if value != "" {
			_ = s.instances.Annotate(ctx, name, appPrefix+key, value)
		}
	}
}

// ── Deploying ─────────────────────────────────────────────────────────────────

type deployRequest struct {
	inspectRequest

	Install  []string `json:"install"`
	Build    []string `json:"build"`
	Start    string   `json:"start"`
	Runtime  string   `json:"runtime"`
	Port     int      `json:"port"`
	Packages []string `json:"packages"`
}

// A package name is an argument to apt, so it is held to what apt names can
// actually be. The build commands are not: they are what the project needs,
// and pretending to whitelist them would be theatre.
var packagePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9.+:-]{0,127}$`)

// oneLine is the one restriction on a command. The unit file is written with a
// heredoc, so a newline in the start command would produce a broken unit and a
// confusing failure much later.
func oneLine(command string) error {
	if strings.ContainsAny(command, "\n\r\x00") {
		return errors.New("a command has to be one line")
	}
	return nil
}

func (s *Server) acceptDeploy(r *http.Request) (string, *appEntities.App, error) {
	name := r.PathValue("name")
	if err := validName(name); err != nil {
		return "", nil, err
	}

	var body deployRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		return "", nil, err
	}

	app, err := accepted(body.inspectRequest)
	if err != nil {
		return "", nil, err
	}

	for _, command := range append(append([]string{body.Start}, body.Install...), body.Build...) {
		if err := oneLine(command); err != nil {
			return "", nil, err
		}
	}
	for _, name := range body.Packages {
		if !packagePattern.MatchString(name) {
			return "", nil, errors.New(name + " is not a package name")
		}
	}
	if body.Port < 0 || body.Port > 65535 {
		return "", nil, errors.New("the port must be between 1 and 65535")
	}

	app.Install, app.Build = body.Install, body.Build
	app.Start, app.Runtime, app.Port = strings.TrimSpace(body.Start), body.Runtime, body.Port
	app.Packages = body.Packages

	// Everything else can be missing and the deployment still means something.
	// Without a start command there is nothing to deploy.
	if !app.Runnable() {
		return "", nil, errors.New("nothing says how to start it, so there is no deployment to run")
	}
	return name, app, nil
}

func (s *Server) planDeploy(w http.ResponseWriter, r *http.Request) {
	name, app, err := s.acceptDeploy(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, PlanResponse{
		Plan: container.NewPlanner(s.bin, name).Deploy(app, time.Now()),
	})
}

// deploy walks the very plan that planDeploy returned. The narration names the
// step being run, so what scrolls past is the list that was approved.
func (s *Server) deploy(w http.ResponseWriter, r *http.Request) {
	name, app, err := s.acceptDeploy(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	ctx := r.Context()
	steps := container.NewPlanner(s.bin, name).Deploy(app, time.Now()).Steps

	s.stream(w, func(report func(int, string)) error {
		for i, step := range steps {
			report(i+1, step.Describe)
			if err := host.RunStep(ctx, s.host, step); err != nil {
				return fmt.Errorf("%s: %w", step.Describe, err)
			}
		}
		// Written last: what is recorded on the container is what actually ran.
		s.remember(ctx, name, app)
		return nil
	})
}

// appKeys is the whole of what a deployment is, which is why it can live on
// the container and nowhere else.
var appKeys = []string{"repo", "branch", "path", "runtime", "install", "build", "start", "port", "packages"}

// showApp reads the deployment back off the container, so the panel is showing
// the machine rather than a cache of it.
func (s *Server) showApp(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if err := validName(name); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	stored := map[string]string{}
	for _, key := range appKeys {
		out, err := s.host.Run(r.Context(), s.bin, "config", "get", name,
			runtime.AnnotationPrefix+"."+appPrefix+key)
		if err == nil {
			stored[key] = strings.TrimSpace(out.Stdout)
		}
	}

	port, _ := strconv.Atoi(stored["port"])
	writeJSON(w, http.StatusOK, AppDTO{
		Deployed: stored["repo"] != "",
		Repo:     stored["repo"], Branch: stored["branch"], Path: stored["path"],
		Runtime: stored["runtime"], Start: stored["start"], Port: port,
		Install:  splitCommands(stored["install"]),
		Build:    splitCommands(stored["build"]),
		Packages: strings.Fields(stored["packages"]),
	})
}

func splitCommands(joined string) []string {
	if strings.TrimSpace(joined) == "" {
		return nil
	}
	out := []string{}
	for _, command := range strings.Split(joined, " && ") {
		out = append(out, strings.TrimSpace(command))
	}
	return out
}

// appLogs is the first thing anybody asks for when a deployment does not come
// up, so it is one request away rather than an ssh session away.
func (s *Server) appLogs(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if err := validName(name); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	lines := 200
	if asked, err := strconv.Atoi(r.URL.Query().Get("lines")); err == nil && asked > 0 && asked <= 2000 {
		lines = asked
	}

	step := container.NewPlanner(s.bin, name).Logs(lines).Steps[0]
	out, err := s.host.Run(r.Context(), step.Argv[0], step.Argv[1:]...)
	if err != nil && out.Stdout == "" {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, LogsResponse{Lines: out.Stdout})
}
