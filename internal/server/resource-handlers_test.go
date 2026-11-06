package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"

	"github.com/Hyzokaaa/opencroft/internal/agent"
	"github.com/Hyzokaaa/opencroft/internal/instance/domain/entities"
	"github.com/Hyzokaaa/opencroft/internal/instance/domain/enums"
	"github.com/Hyzokaaa/opencroft/internal/instance/infrastructure/runtime"
	"github.com/Hyzokaaa/opencroft/internal/shared/plan"
)

type fakeProvisioner struct{ fail map[string]bool }

func (f fakeProvisioner) FindAll(_ context.Context, container string) (agent.DatabasesResponse, error) {
	if f.fail[container] {
		return agent.DatabasesResponse{}, errors.New("could not read " + container)
	}
	return agent.DatabasesResponse{Databases: []agent.DatabaseDTO{{Name: "postgres-" + container}}}, nil
}
func (fakeProvisioner) ProvisionPlan(context.Context, string, agent.DatabaseDTO) (plan.Plan, error) {
	return onePlan(), nil
}
func (fakeProvisioner) Provision(context.Context, string, agent.DatabaseDTO, func(int, string)) error {
	return nil
}
func (fakeProvisioner) DestroyPlan(context.Context, string, string) (plan.Plan, string, error) {
	return onePlan(), "", nil
}
func (fakeProvisioner) Destroy(context.Context, string, string, func(int, string)) error { return nil }

func resourcesHost(t *testing.T, provisioner fakeProvisioner) http.Handler {
	t.Helper()
	instances := runtime.NewMemoryInstanceRepository()
	for _, props := range []entities.InstanceProps{
		{Name: "web", Project: "openhelpdesk", Status: enums.StatusRunning},
		{Name: "db", Project: "openhelpdesk", Status: enums.StatusStopped},
		{Name: "demo", Project: "croft-demo", Status: enums.StatusRunning},
		{Name: "scratch", Status: enums.StatusRunning},
	} {
		_ = instances.Create(context.Background(), entities.NewInstance(props))
	}
	deployer := &fakeDeployer{services: []agent.ServiceDTO{{Name: "backend"}}}
	return api(Deps{Instances: instances, Services: deployer, Databases: provisioner})
}

func resources(t *testing.T, handler http.Handler, query string) map[string]ContainerResourcesDTO {
	t.Helper()
	recorder := send(handler, http.MethodGet, "/api/hosts/local/resources"+query, nil)
	if recorder.Code != http.StatusOK {
		t.Fatalf("answered %d: %s", recorder.Code, recorder.Body.String())
	}
	var listed []ContainerResourcesDTO
	_ = json.Unmarshal(recorder.Body.Bytes(), &listed)
	byName := map[string]ContainerResourcesDTO{}
	for _, c := range listed {
		byName[c.Name] = c
	}
	return byName
}

// A project's page is what its containers hold, read in one request: the
// services were a click away inside each container, and that click was the
// difference between seeing what runs and seeing boxes.
func TestAProjectListsWhatEachOfItsContainersHolds(t *testing.T) {
	listed := resources(t, resourcesHost(t, fakeProvisioner{}), "?project=openhelpdesk")

	if len(listed) != 2 {
		t.Fatalf("listed %v, want the project's two containers", listed)
	}
	web := listed["web"]
	if web.Services == nil || len(web.Services.Services) != 1 || web.Services.Services[0].Name != "backend" {
		t.Errorf("web's services were lost: %+v", web.Services)
	}
	if web.Databases == nil || web.Databases.Databases[0].Name != "postgres-web" {
		t.Errorf("web's databases were lost: %+v", web.Databases)
	}
}

// A stopped container still says what it holds: it is all in its annotations.
// Hiding it would read as though it had been removed.
func TestAStoppedContainerStillSaysWhatItHolds(t *testing.T) {
	db := resources(t, resourcesHost(t, fakeProvisioner{}), "?project=openhelpdesk")["db"]

	if db.Status != string(enums.StatusStopped) || db.Databases == nil || len(db.Databases.Databases) != 1 {
		t.Errorf("a stopped container was emptied: %+v", db)
	}
}

// An empty project filter is the containers in no project — the "Other
// containers" at the end of Home — not every container.
func TestNoProjectIsItsOwnList(t *testing.T) {
	listed := resources(t, resourcesHost(t, fakeProvisioner{}), "?project=")

	if len(listed) != 1 || listed["scratch"].Name != "scratch" {
		t.Errorf("listed %v, want only the container in no project", listed)
	}
}

// One container that cannot be read does not blank the page, and says why
// rather than reading as holding nothing.
func TestAContainerThatCannotBeReadSaysSo(t *testing.T) {
	listed := resources(t, resourcesHost(t, fakeProvisioner{fail: map[string]bool{"web": true}}), "?project=openhelpdesk")

	if listed["web"].DatabasesError == "" || listed["web"].Databases != nil {
		t.Errorf("a failed read looks like an answer: %+v", listed["web"])
	}
	if listed["db"].Databases == nil {
		t.Errorf("one failure took the others with it: %+v", listed["db"])
	}
}
