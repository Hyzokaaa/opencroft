package services_test

import (
	"strings"
	"testing"

	"github.com/Hyzokaaa/opencroft/internal/deploy/domain/entities"
	"github.com/Hyzokaaa/opencroft/internal/deploy/domain/services"
)

func repo(files map[string]string) services.Repository {
	present := map[string]bool{}
	for name := range files {
		present[name] = true
	}
	return services.Repository{Files: present, Contents: files}
}

func TestNothingRecognisableIsSaidPlainly(t *testing.T) {
	if _, ok := services.Detect(repo(map[string]string{"README.md": "# hello"})); ok {
		t.Fatal("invented a runtime for a repository with nothing in it")
	}
}

// Proposing `npm run build` for a project that has no build script is a plan
// that fails on step two. The scripts are read, not assumed.
func TestBuildIsOnlyProposedWhenTheScriptExists(t *testing.T) {
	withBuild, _ := services.Detect(repo(map[string]string{
		"package.json": `{"scripts":{"build":"vite build","start":"node ."}}`,
	}))
	if len(withBuild.Build) == 0 {
		t.Error("a project with a build script was not given one")
	}

	without, _ := services.Detect(repo(map[string]string{
		"package.json": `{"scripts":{"start":"node ."}}`,
	}))
	if len(without.Build) != 0 {
		t.Errorf("proposed %v for a project with no build script", without.Build)
	}
}

// npm ci needs a lockfile and fails without one.
func TestTheInstallCommandFollowsTheLockfile(t *testing.T) {
	locked, _ := services.Detect(repo(map[string]string{
		"package.json":      `{"scripts":{"start":"node ."}}`,
		"package-lock.json": "{}",
	}))
	if strings.Join(locked.Install, " ") != "npm ci" {
		t.Errorf("with a lockfile: %v", locked.Install)
	}

	loose, _ := services.Detect(repo(map[string]string{
		"package.json": `{"scripts":{"start":"node ."}}`,
	}))
	if strings.Join(loose.Install, " ") != "npm install" {
		t.Errorf("without a lockfile: %v", loose.Install)
	}
}

func TestAStartCommandIsFoundWhereverItIs(t *testing.T) {
	cases := map[string]struct {
		files map[string]string
		want  string
	}{
		"a start script": {map[string]string{"package.json": `{"scripts":{"start":"node ."}}`}, "npm start"},
		"a server file":  {map[string]string{"package.json": `{}`, "server.js": ""}, "node server.js"},
		"an index file":  {map[string]string{"package.json": `{}`, "index.js": ""}, "node index.js"},
	}

	for label, test := range cases {
		t.Run(label, func(t *testing.T) {
			detection, _ := services.Detect(repo(test.files))
			if detection.Start != test.want {
				t.Errorf("got %q, wanted %q", detection.Start, test.want)
			}
		})
	}
}

// Guessing wrong is fine. Guessing wrong silently is not, so a detection that
// found no way to start says so in the reason the panel shows.
func TestNotKnowingHowToStartIsSaidOutLoud(t *testing.T) {
	detection, _ := services.Detect(repo(map[string]string{"package.json": `{}`}))

	if detection.Start != "" {
		t.Fatalf("invented a start command: %q", detection.Start)
	}
	if !strings.Contains(detection.Why, "start") {
		t.Errorf("the reason does not mention it: %q", detection.Why)
	}
}

func TestGoAndStaticAreRecognised(t *testing.T) {
	goDetection, ok := services.Detect(repo(map[string]string{"go.mod": "module x"}))
	if !ok || goDetection.Runtime != "go" || goDetection.Start == "" {
		t.Errorf("go: %+v", goDetection)
	}

	staticDetection, ok := services.Detect(repo(map[string]string{"index.html": "<h1>hi</h1>"}))
	if !ok || staticDetection.Runtime != "static" {
		t.Errorf("static: %+v", staticDetection)
	}
}

// A repository with both is a Go project with a web front end, not a static
// site. The more specific signal wins.
func TestASpecificSignalBeatsAGenericOne(t *testing.T) {
	detection, _ := services.Detect(repo(map[string]string{
		"go.mod":     "module x",
		"index.html": "<h1>hi</h1>",
	}))
	if detection.Runtime != "go" {
		t.Errorf("got %q", detection.Runtime)
	}
}

// A command somebody typed always wins over one we guessed.
func TestWhatYouTypedSurvivesDetection(t *testing.T) {
	existing := entities.NewService(entities.ServiceProps{
		Start: "node dist/main.js",
		Port:  4000,
	})

	detection, _ := services.Detect(repo(map[string]string{
		"package.json": `{"scripts":{"start":"node ."}}`,
	}))
	app := services.Apply(detection, existing)

	if app.Start != "node dist/main.js" {
		t.Errorf("detection overwrote the start command: %q", app.Start)
	}
	if app.Port != 4000 {
		t.Errorf("detection overwrote the port: %d", app.Port)
	}
	if len(app.Install) == 0 {
		t.Error("detection did not fill in what was missing")
	}
}

// The distinction has to exist before deploy keys do, or adding them later
// means reshaping everything around them.
func TestAnSSHSourceIsKnownToBePrivate(t *testing.T) {
	cases := map[string]bool{
		"https://github.com/user/app.git": false,
		"git@github.com:user/app.git":     true,
		"ssh://git@example.com/app.git":   true,
	}

	for url, private := range cases {
		source := entities.Source{Repo: url, Auth: entities.AuthNone}
		if source.Private() != private {
			t.Errorf("%s: got %v", url, source.Private())
		}
	}
}
