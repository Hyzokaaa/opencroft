package services

import (
	"encoding/json"
	"strings"

	"github.com/Hyzokaaa/opencroft/internal/app/domain/entities"
)

// Detection looks at what a repository contains and proposes how to build and
// run it.
//
// Every buildpack in existence does this and then hides the result. We do the
// opposite: what is detected becomes visible commands in the plan and editable
// annotations on the container. Guessing is fine; guessing invisibly is not.
type Detection struct {
	Runtime  string
	Packages []string
	Install  []string
	Build    []string
	Start    string
	Port     int

	// Why is shown to the person, so a wrong guess is obvious rather than
	// mysterious.
	Why string
}

// Repository is what the agent found at the root of the checkout: the names of
// the files, and the contents of the few worth reading.
type Repository struct {
	Files    map[string]bool
	Contents map[string]string
}

func (r Repository) has(name string) bool { return r.Files[name] }

// Detect returns false when nothing recognisable is there. That is not a
// failure — it means the commands have to be given rather than guessed, and
// saying so is better than inventing something that will not work.
func Detect(repo Repository) (Detection, bool) {
	switch {
	case repo.has("package.json"):
		return node(repo), true
	case repo.has("go.mod"):
		return golang(), true
	case repo.has("index.html"):
		return static(), true
	default:
		return Detection{}, false
	}
}

type packageJSON struct {
	Scripts map[string]string `json:"scripts"`
}

func node(repo Repository) Detection {
	detection := Detection{
		Runtime:  "node",
		Packages: []string{"nodejs", "npm"},
		Port:     3000,
		Why:      "package.json is present",
	}

	// npm ci needs a lockfile and fails without one; npm install does not.
	if repo.has("package-lock.json") {
		detection.Install = []string{"npm ci"}
	} else {
		detection.Install = []string{"npm install"}
		detection.Why += ", without a lockfile"
	}

	var manifest packageJSON
	_ = json.Unmarshal([]byte(repo.Contents["package.json"]), &manifest)

	// Proposing `npm run build` for a project with no build script is a plan
	// that fails on step two. Read the scripts and only propose what exists.
	if _, ok := manifest.Scripts["build"]; ok {
		detection.Build = []string{"npm run build"}
	}

	switch {
	case manifest.Scripts["start"] != "":
		detection.Start = "npm start"
	case repo.has("server.js"):
		detection.Start = "node server.js"
	case repo.has("index.js"):
		detection.Start = "node index.js"
	}

	if detection.Start == "" {
		detection.Why += ", but nothing says how to start it"
	}
	return detection
}

func golang() Detection {
	return Detection{
		Runtime:  "go",
		Packages: []string{"golang"},
		Build:    []string{"go build -o app ./..."},
		Start:    "./app",
		Port:     8080,
		Why:      "go.mod is present",
	}
}

// A directory of files is served by nginx inside the container, which keeps
// the shape identical to everything else: something listens on a port and the
// host proxies to it.
func static() Detection {
	return Detection{
		Runtime:  "static",
		Packages: []string{"nginx"},
		Start:    "nginx -g 'daemon off;'",
		Port:     80,
		Why:      "index.html is present and nothing else was recognised",
	}
}

// Apply turns a detection into an app, leaving anything already set alone.
// A command somebody typed always wins over one we guessed.
func Apply(detection Detection, existing *entities.App) *entities.App {
	app := existing
	if app == nil {
		app = entities.NewApp(entities.AppProps{})
	}

	if len(app.Install) == 0 {
		app.Install = detection.Install
	}
	if len(app.Build) == 0 {
		app.Build = detection.Build
	}
	if app.Start == "" {
		app.Start = detection.Start
	}
	if app.Port == 0 {
		app.Port = detection.Port
	}
	if app.Runtime == "" {
		app.Runtime = detection.Runtime
	}
	if len(app.Packages) == 0 {
		app.Packages = detection.Packages
	}
	return app
}

// Summarise is the sentence the panel shows before anything runs.
func Summarise(detection Detection) string {
	parts := []string{"Detected " + detection.Runtime}

	if len(detection.Install) > 0 {
		parts = append(parts, "install with `"+strings.Join(detection.Install, " && ")+"`")
	}
	if len(detection.Build) > 0 {
		parts = append(parts, "build with `"+strings.Join(detection.Build, " && ")+"`")
	}
	if detection.Start != "" {
		parts = append(parts, "start with `"+detection.Start+"`")
	}
	return strings.Join(parts, ", ")
}
