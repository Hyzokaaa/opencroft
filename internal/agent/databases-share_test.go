package agent

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"
)

// twoInOneProject puts helpdesk, holding a postgres database, and landing in
// the same project; staging stays outside it.
func twoInOneProject(t *testing.T) *Server {
	t.Helper()
	server, _ := testServer()
	ctx := context.Background()
	for name, value := range map[string]string{
		"helpdesk": "shop", "landing": "shop", "staging": "",
	} {
		if value != "" {
			if err := server.instances.Annotate(ctx, name, "project", value); err != nil {
				t.Fatal(err)
			}
		}
	}
	for key, value := range map[string]string{
		"databases": "main", "database.main.engine": "postgres",
		"database.main.db": "main", "database.main.user": "main", "database.main.port": "5432",
	} {
		if err := server.instances.Annotate(ctx, "helpdesk", key, value); err != nil {
			t.Fatal(err)
		}
	}
	return server
}

// A container is offered the databases of its own project, and nothing from
// outside it.
func TestAContainerIsOfferedItsProjectsDatabases(t *testing.T) {
	server := twoInOneProject(t)

	var offers []OfferDTO
	_ = json.Unmarshal(serve(server, http.MethodGet, "/instances/landing/databases/shareable", nil).Body.Bytes(), &offers)
	if len(offers) != 1 || offers[0].Container != "helpdesk" || offers[0].Name != "main" {
		t.Errorf("landing is offered %+v", offers)
	}

	offers = nil
	_ = json.Unmarshal(serve(server, http.MethodGet, "/instances/staging/databases/shareable", nil).Body.Bytes(), &offers)
	if len(offers) != 0 {
		t.Errorf("a container in no project is offered %+v", offers)
	}
}

// The plan reaches the provider by its name on the bridge, and a container
// from another project is refused before anything runs.
func TestConnectingIsPlannedByNameAndOnlyInsideTheProject(t *testing.T) {
	server := twoInOneProject(t)

	recorder := serve(server, http.MethodPost, "/instances/landing/databases/connect/plan", ShareDTO{Location: "helpdesk", Name: "main"})
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), "@helpdesk.lxd:5432/main") {
		t.Errorf("answered %d %s", recorder.Code, recorder.Body.String())
	}

	recorder = serve(server, http.MethodPost, "/instances/staging/databases/connect/plan", ShareDTO{Location: "helpdesk", Name: "main"})
	if recorder.Code != http.StatusBadRequest || !strings.Contains(recorder.Body.String(), "same project") {
		t.Errorf("another project: %d %s", recorder.Code, recorder.Body.String())
	}
}

// Dropping a database somebody else is connected to is refused, naming them.
func TestADatabaseOthersReadIsNotDropped(t *testing.T) {
	server := twoInOneProject(t)
	ctx := context.Background()
	for key, value := range map[string]string{
		"databases": "main", "database.main.engine": "postgres", "database.main.db": "main",
		"database.main.user": "from_landing", "database.main.location": "helpdesk",
	} {
		_ = server.instances.Annotate(ctx, "landing", key, value)
	}

	recorder := serve(server, http.MethodGet, "/instances/helpdesk/databases/main/destroy/plan", nil)
	if recorder.Code != http.StatusBadRequest || !strings.Contains(recorder.Body.String(), "disconnect landing") {
		t.Errorf("answered %d %s", recorder.Code, recorder.Body.String())
	}

	// And landing letting go of it is a disconnection, not a drop.
	recorder = serve(server, http.MethodGet, "/instances/landing/databases/main/destroy/plan", nil)
	body := recorder.Body.String()
	if recorder.Code != http.StatusOK || strings.Contains(body, "DROP DATABASE") || !strings.Contains(body, "DROP USER IF EXISTS from_landing") {
		t.Errorf("letting go: %d %s", recorder.Code, body)
	}
}

// A bridge with its DNS off gives names to nobody, and none are offered.
func TestNoNameIsOfferedWhereTheBridgeGivesNone(t *testing.T) {
	server, fake := testServer()
	fake.Responses["lxc network get lxdbr0 dns.mode"] = "none\n"
	if name := server.internalName(context.Background(), "helpdesk"); name != "" {
		t.Errorf("offered %q", name)
	}

	server, fake = testServer()
	fake.Failures["dns.mode"] = errors.New("no such key")
	fake.Responses["lxc network get lxdbr0 dns.domain"] = "croft\n"
	if name := server.internalName(context.Background(), "helpdesk"); name != "helpdesk.croft" {
		t.Errorf("named %q", name)
	}
}
