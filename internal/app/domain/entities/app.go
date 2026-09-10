// Package entities holds what a deployed application is.
//
// An App is not a new kind of thing beside an Instance — it is an Instance
// with a source, a way to build it and a way to run it. That is why all of
// this lives as annotations on the container: losing our database costs
// nothing, and migrating the container carries the deployment with it.
package entities

import "strings"

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
}

// Private is true for anything that cannot be cloned anonymously. Today that
// is decided by the URL shape; later it will be decided by whether a key is
// registered.
func (s Source) Private() bool {
	return s.Auth != AuthNone || strings.HasPrefix(s.Repo, "git@") ||
		strings.HasPrefix(s.Repo, "ssh://")
}

// App is what a container runs, and how.
type App struct {
	// Where the code lives inside the container.
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
}

type AppProps struct {
	Path     string
	Source   Source
	Install  []string
	Build    []string
	Start    string
	Runtime  string
	Port     int
	Packages []string
}

func NewApp(props AppProps) *App {
	path := props.Path
	if path == "" {
		path = "/srv/app"
	}

	return &App{
		Path:     path,
		Source:   props.Source,
		Install:  props.Install,
		Build:    props.Build,
		Start:    props.Start,
		Runtime:  props.Runtime,
		Port:     props.Port,
		Packages: props.Packages,
	}
}

// Runnable is false when there is nothing to start, which is the one thing a
// deployment cannot do without.
func (a *App) Runnable() bool {
	return strings.TrimSpace(a.Start) != ""
}
