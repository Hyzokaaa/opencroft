package agent

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/Hyzokaaa/opencroft/internal/project/infrastructure/files"
)

// The agent writes the declaration as a file and the label with the runtime,
// deciding again when asked to run rather than trusting the plan it showed.
func TestAProjectIsDeclaredAndAContainerMovedIntoIt(t *testing.T) {
	server, fake := testServer()

	recorder := serve(server, http.MethodPut, "/projects/billing", ProjectDTO{Description: "Invoices"})
	if recorder.Code != http.StatusOK || !strings.Contains(fake.Files[files.Dir+"/billing.conf"], "description = Invoices") {
		t.Fatalf("declaring answered %d %s; files %v", recorder.Code, recorder.Body.String(), fake.Files)
	}

	fake.Dirs[files.Dir] = []string{"billing.conf"}
	recorder = serve(server, http.MethodPut, "/instances/helpdesk/project", AssignmentDTO{Project: "billing"})
	if recorder.Code != http.StatusOK || !fake.Ran("lxc config set helpdesk user.croft.project billing") {
		t.Fatalf("moving answered %d %s; ran %v", recorder.Code, recorder.Body.String(), fake.Commands)
	}
}

// A name nobody declared, or a container that is not there, is refused
// before anything runs.
func TestAMoveIntoNothingIsRefused(t *testing.T) {
	server, fake := testServer()

	if recorder := serve(server, http.MethodPut, "/instances/helpdesk/project/plan", AssignmentDTO{Project: "typo"}); recorder.Code != http.StatusBadRequest {
		t.Errorf("into an undeclared project: %d", recorder.Code)
	}
	if recorder := serve(server, http.MethodPut, "/projects/Bad_Name/plan", ProjectDTO{}); recorder.Code == http.StatusOK {
		t.Errorf("a name with capitals and an underscore was taken")
	}
	if len(fake.Commands) > 0 {
		t.Errorf("something ran: %v", fake.Commands)
	}
}

// The listing carries what the panel shows: declared or only found, and the
// containers in each.
func TestProjectsAreListedWithTheirContainers(t *testing.T) {
	server, fake := testServer()
	fake.Dirs[files.Dir] = []string{"billing.conf"}
	fake.Files[files.Dir+"/billing.conf"] = "description = Invoices\n"
	if err := server.instances.Annotate(context.Background(), "landing", "project", "migrated"); err != nil {
		t.Fatal(err)
	}

	var listed []ProjectDTO
	recorder := serve(server, http.MethodGet, "/projects", nil)
	if err := json.Unmarshal(recorder.Body.Bytes(), &listed); err != nil {
		t.Fatal(err)
	}
	if len(listed) != 2 || listed[0].Name != "billing" || !listed[0].Declared || len(listed[0].Instances) != 0 ||
		listed[1].Name != "migrated" || listed[1].Declared || listed[1].Instances[0] != "landing" {
		t.Errorf("listed %+v", listed)
	}
}

// A container is created into a project only if that project is declared:
// a name typed wrong would otherwise make one.
func TestANewContainerGoesOnlyIntoADeclaredProject(t *testing.T) {
	server, fake := testServer()
	want := InstanceDTO{Name: "web", Image: "images:ubuntu/24.04", Address: "10.146.38.210", Port: 80,
		CPULimit: 1, MemLimit: "1GB", Project: "typo"}

	if recorder := serve(server, http.MethodPost, "/instances/plan", want); recorder.Code != http.StatusBadRequest ||
		!strings.Contains(recorder.Body.String(), "no declared project") {
		t.Errorf("into an undeclared project: %d %s", recorder.Code, recorder.Body.String())
	}

	fake.Dirs[files.Dir] = []string{"shop.conf"}
	fake.Files[files.Dir+"/shop.conf"] = "description = \n"
	want.Project = "shop"
	if recorder := serve(server, http.MethodPost, "/instances/plan", want); recorder.Code != http.StatusOK {
		t.Errorf("into a declared project: %d %s", recorder.Code, recorder.Body.String())
	}
}
