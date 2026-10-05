package agent

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func aContainerWithSnapshots(t *testing.T) *Server {
	t.Helper()
	server, fake := testServer()
	fake.Responses["lxc query /1.0/instances/helpdesk/snapshots"] =
		`["/1.0/instances/helpdesk/snapshots/croft-deploy-api-20261001-100000","/1.0/instances/helpdesk/snapshots/croft-destroy-old-20260901-100000","/1.0/instances/helpdesk/snapshots/before-upgrade"]`
	ctx := context.Background()
	_ = server.instances.Annotate(ctx, "helpdesk", "services", "api")
	_ = server.instances.Annotate(ctx, "helpdesk", "service.api.healthy", "croft-deploy-api-20261001-100000")
	return server
}

func removal(t *testing.T, server *Server, snapshot string) (string, string, int) {
	t.Helper()
	recorder := serve(server, http.MethodGet, "/instances/helpdesk/snapshots/"+snapshot+"/destroy/plan", nil)
	var response RollbackResponse
	_ = json.Unmarshal(recorder.Body.Bytes(), &response)
	shell := ""
	for _, step := range response.Plan.Steps {
		shell += step.Shell() + "\n"
	}
	return shell, response.Warning, recorder.Code
}

// Deleting the last version known to work says so, and stops pointing at it.
func TestDeletingTheLastGoodSnapshotSaysWhatIsLost(t *testing.T) {
	server := aContainerWithSnapshots(t)
	shell, warning, _ := removal(t, server, "croft-deploy-api-20261001-100000")
	if !strings.Contains(shell, "lxc delete helpdesk/croft-deploy-api-20261001-100000") ||
		!strings.Contains(shell, "config unset helpdesk user.croft.service.api.healthy") {
		t.Errorf("plan:\n%s", shell)
	}
	if !strings.Contains(warning, "last version of api known to work") {
		t.Errorf("warning: %s", warning)
	}
}

// The way back to a removed service, and a snapshot somebody took by hand,
// are each called what they are.
func TestEverySnapshotKindIsNamedBeforeItGoes(t *testing.T) {
	server := aContainerWithSnapshots(t)
	if _, warning, _ := removal(t, server, "croft-destroy-old-20260901-100000"); !strings.Contains(warning, "only way back") {
		t.Errorf("destroy kind: %s", warning)
	}
	if _, warning, _ := removal(t, server, "before-upgrade"); !strings.Contains(warning, "by hand") {
		t.Errorf("a person's snapshot: %s", warning)
	}
	if _, _, code := removal(t, server, "nothing-like-this"); code != http.StatusBadRequest {
		t.Errorf("a snapshot that is not there: %d", code)
	}
}
