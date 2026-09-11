// Package entities holds what a deployed service is.
//
// A Service is not a new kind of thing beside an Instance — it is something a
// container runs, with a source, a way to build it and a way to run it. A
// container can hold several, though one is usually wiser: snapshots are taken
// of containers, not of services, so rolling one back takes its neighbours
// with it. The panel says so where it matters rather than in a document.
//
// All of this lives as annotations on the container: losing our database costs
// nothing, and migrating the container carries the deployments with it.
package entities

import (
	"sort"
	"strings"
)

// AuthKind says how the source is reached. Only None is implemented, but the
// distinction exists now so that adding a deploy key later is filling in a
// gap rather than reshaping everything around it.
type AuthKind string

const (
	AuthNone AuthKind = "none"
	// AuthDeployKey means a key lives on the container and its public half is
	// registered with the forge. Not implemented yet.
	AuthDeployKey AuthKind = "deploy-key"
)

type Source struct {
	Repo   string
	Branch string
	Auth   AuthKind

	// Commit pins the deployment to one revision. Empty means the tip of the
	// branch; set, it is how going back to the previous version works without
	// restoring a snapshot and taking everything else with it.
	Commit string
}

// Private is true for anything that cannot be cloned anonymously. Today that
// is decided by the URL shape; later it will be decided by whether a key is
// registered.
func (s Source) Private() bool {
	return s.Auth != AuthNone || strings.HasPrefix(s.Repo, "git@") ||
		strings.HasPrefix(s.Repo, "ssh://")
}

// Health is the question asked after a deployment, and the answer that counts
// as success.
//
// There is no signal that says an application is ready — systemd reports a
// unit as running the moment the process forks, which is long before anything
// listens. So readiness is a question asked repeatedly until it is answered or
// the budget runs out. Everything that claims otherwise is doing this
// underneath.
type Health struct {
	// Path is what to ask for, on the service's own port. Empty means the
	// deployment is finished when the unit is up, which is a weaker promise
	// and the panel says so.
	Path string

	// Status the response must carry. Zero means 200.
	Status int

	// Contains, when set, must appear in the body. Deliberately not a regular
	// expression or a JSON path: this is a check that a deployment worked, not
	// a monitoring system.
	Contains string
}

func (h Health) Wanted() bool { return strings.TrimSpace(h.Path) != "" }

func (h Health) Code() int {
	if h.Status == 0 {
		return 200
	}
	return h.Status
}

// Service is one thing a container runs, and how.
type Service struct {
	// Name identifies it within the container, and names its unit and its
	// snapshots. One container can run several.
	Name string

	// Path is where the code lives inside the container.
	Path string

	Source Source

	// Install, Build and Start are the commands, kept as text because that is
	// what a person edits and what a plan shows. Detection proposes them; it
	// never decides them behind your back.
	Install []string
	Build   []string
	Start   string

	// Runtime is only a label, for the interface to say what it recognised.
	Runtime string
	Port    int

	// Packages the container needs before any of the above will work.
	Packages []string

	// Env becomes a file beside the code that the unit reads. Nothing starts
	// without it for most real applications, which is why it is part of the
	// service rather than a step somebody remembers afterwards.
	Env map[string]string

	Health Health
}

type ServiceProps struct {
	Name     string
	Path     string
	Source   Source
	Install  []string
	Build    []string
	Start    string
	Runtime  string
	Port     int
	Packages []string
	Env      map[string]string
	Health   Health
}

func NewService(props ServiceProps) *Service {
	path := props.Path
	if path == "" {
		path = DefaultPath(props.Name)
	}

	return &Service{
		Name:     props.Name,
		Path:     path,
		Source:   props.Source,
		Install:  props.Install,
		Build:    props.Build,
		Start:    props.Start,
		Runtime:  props.Runtime,
		Port:     props.Port,
		Packages: props.Packages,
		Env:      props.Env,
		Health:   props.Health,
	}
}

// DefaultPath keeps services out of each other's way without anybody having to
// think about it.
func DefaultPath(name string) string {
	if name == "" {
		return "/srv/app"
	}
	return "/srv/" + name
}

// Unit is the systemd service inside the container. The name carries the
// prefix so that a person reading `systemctl list-units` can tell what put it
// there — the same courtesy the generated vhosts extend.
func (s *Service) Unit() string { return "croft-" + s.Name }

// EnvFile sits beside the code rather than in /etc, so that moving or removing
// the checkout takes its configuration with it.
func (s *Service) EnvFile() string { return s.Path + "/.env" }

// Runnable is false when there is nothing to start, which is the one thing a
// deployment cannot do without.
func (s *Service) Runnable() bool { return strings.TrimSpace(s.Start) != "" }

// EnvContent renders the environment file. Sorted, so that redeploying an
// unchanged service produces an unchanged file and the plan does not show a
// difference that is not there.
func (s *Service) EnvContent() string {
	keys := make([]string, 0, len(s.Env))
	for key := range s.Env {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	var out strings.Builder
	for _, key := range keys {
		out.WriteString(key)
		out.WriteString("=")
		out.WriteString(s.Env[key])
		out.WriteString("\n")
	}
	return out.String()
}
