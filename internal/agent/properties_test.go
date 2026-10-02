package agent

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func aDeployedBackend(t *testing.T) *Server {
	t.Helper()
	server, _ := testServer()
	ctx := context.Background()
	for key, value := range map[string]string{
		"services":                "backend",
		"service.backend.repo":    "https://github.com/user/app.git",
		"service.backend.branch":  "main",
		"service.backend.path":    "/srv/backend",
		"service.backend.install": "npm ci",
		"service.backend.start":   "node dist/main",
		"service.backend.port":    "3000",
	} {
		_ = server.instances.Annotate(ctx, "helpdesk", key, value)
	}
	config, _ := server.instances.Annotations(ctx, "helpdesk")
	_ = server.instances.Annotate(ctx, "helpdesk", "service.backend.deployed", Fingerprint(config, "backend"))
	return server
}

func backend(t *testing.T, server *Server) ServiceDTO {
	t.Helper()
	var listed ServicesResponse
	_ = json.Unmarshal(serve(server, http.MethodGet, "/instances/helpdesk/services", nil).Body.Bytes(), &listed)
	for _, s := range listed.Services {
		if s.Name == "backend" {
			return s
		}
	}
	t.Fatal("backend is not listed")
	return ServiceDTO{}
}

// Saving writes only the properties that change, deploys nothing, and leaves
// the service marked as having changes not deployed.
func TestSavedPropertiesWaitForTheNextDeployment(t *testing.T) {
	server := aDeployedBackend(t)
	if backend(t, server).Pending {
		t.Fatal("a service just deployed shows changes")
	}

	want := backend(t, server)
	want.Start = "node dist/server"
	recorder := serve(server, http.MethodPut, "/instances/helpdesk/services/backend/properties/plan", want)
	body := recorder.Body.String()
	if recorder.Code != http.StatusOK || !strings.Contains(body, "service.backend.start") {
		t.Fatalf("answered %d %s", recorder.Code, body)
	}
	for _, untouched := range []string{"service.backend.install", "service.backend.repo", "git", "systemctl"} {
		if strings.Contains(body, untouched) {
			t.Errorf("saving reaches %s:\n%s", untouched, body)
		}
	}

	_ = server.instances.Annotate(context.Background(), "helpdesk", "service.backend.start", "node dist/server")
	if !backend(t, server).Pending {
		t.Error("a saved change is not shown as waiting for a deployment")
	}
}

// Nothing changed is nothing to save, and a service that is not there has no
// properties to change.
func TestSavingNothingOrNoServiceIsRefused(t *testing.T) {
	server := aDeployedBackend(t)
	if recorder := serve(server, http.MethodPut, "/instances/helpdesk/services/backend/properties/plan", backend(t, server)); recorder.Code != http.StatusBadRequest {
		t.Errorf("an unchanged save: %d", recorder.Code)
	}
	if recorder := serve(server, http.MethodPut, "/instances/helpdesk/services/ghost/properties/plan", ServiceDTO{Name: "ghost", Start: "x"}); recorder.Code != http.StatusBadRequest {
		t.Errorf("a service that is not there: %d", recorder.Code)
	}
}

// A service deployed before fingerprints were kept is not called changed.
func TestAServiceWithoutAFingerprintIsNotPending(t *testing.T) {
	config := map[string]string{"user.croft.service.web.start": "x"}
	if pending(config, "web") {
		t.Error("pending without a fingerprint")
	}
}

// A service deployed before fingerprints were kept gets one, from what it runs
// now, before its change is saved — so the change shows as pending.
func TestAnOlderServiceIsFingerprintedBeforeItsFirstSave(t *testing.T) {
	server := aDeployedBackend(t)
	_ = server.instances.Annotate(context.Background(), "helpdesk", "service.backend.deployed", "")

	want := backend(t, server)
	want.Start = "node dist/server"
	body := serve(server, http.MethodPut, "/instances/helpdesk/services/backend/properties/plan", want).Body.String()
	first := strings.Index(body, "service.backend.deployed")
	change := strings.Index(body, "service.backend.start")
	if first < 0 || change < 0 || first > change {
		t.Errorf("the fingerprint is not recorded before the change:\n%s", body)
	}
}
