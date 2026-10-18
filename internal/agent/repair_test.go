package agent

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Hyzokaaa/opencroft/internal/shared/plan"
)

// After a power cut, git's own words are a wall of the same line, and none of
// them say the copy is damaged or that it can be mended. The person reading
// it has to decide what to do next, so the failure says that instead.
func TestAFetchStoppedByDamagedObjectsSaysHowToRepair(t *testing.T) {
	step := plan.Command("Fetch the repository", "lxc", "exec", "helpdesk", "--", "sh", "-lc", "git fetch")

	for _, said := range []string{
		"exit status 128: error: object file .git/objects/0c/d031 is empty fatal: cannot read existing object info 0cd031",
		"exit status 128: fatal: bad object HEAD",
		"exit status 128: error: fd2b4f: object corrupt or missing: .git/objects/fd/2b4f",
	} {
		if got := explain(step, errors.New(said)).Error(); !strings.Contains(got, "Repair the checkout") {
			t.Errorf("%q is explained as:\n%s", said, got)
		}
	}

	other := explain(step, errors.New("exit status 128: fatal: repository not found")).Error()
	if strings.Contains(other, "Repair") {
		t.Errorf("a repository that is not there is called damaged:\n%s", other)
	}
}

// What a repair deletes and fetches comes from the container, not the
// request: the path decides what is removed, and the agent runs as root.
func TestARepairIsPlannedFromWhatTheContainerRecords(t *testing.T) {
	server, fake := testServer()
	withBackend(t, server, nil)

	recorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(recorder,
		httptest.NewRequest(http.MethodGet, "/instances/helpdesk/services/backend/repair/plan", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("answered %d: %s", recorder.Code, recorder.Body.String())
	}
	body := shell(t, recorder)
	for _, wanted := range []string{"find /srv/backend/.git -type f -empty -delete", "app.git dev"} {
		if !strings.Contains(body, wanted) {
			t.Errorf("the repair lacks %q:\n%s", wanted, body)
		}
	}

	for _, name := range []string{"nothing", "-rf", "$(id)", "backend;reboot"} {
		recorder := httptest.NewRecorder()
		server.Handler().ServeHTTP(recorder,
			httptest.NewRequest(http.MethodGet, "/instances/helpdesk/services/"+name+"/repair/plan", nil))
		if recorder.Code == http.StatusOK {
			t.Errorf("%q was given a repair", name)
		}
	}
	for _, command := range fake.Commands {
		if strings.Contains(command, "-delete") {
			t.Errorf("planning ran something: %s", command)
		}
	}
}
