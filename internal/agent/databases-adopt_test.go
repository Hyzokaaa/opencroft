package agent

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

// aContainerWithItsOwnPostgres is what an install script leaves: postgres
// running, a database owned by the application's user, nothing recorded.
func aContainerWithItsOwnPostgres(t *testing.T) *Server {
	t.Helper()
	server, fake := testServer()
	fake.Responses["lxc exec helpdesk -- sh -c"] =
		"postgres|helpdesk|helpdesk_app|5432\npostgres|Bad-Name|someone|5432\nnoise\n"
	return server
}

// What the engines report is offered, and what croft has recorded is not.
func TestADatabaseRunningInAContainerIsFound(t *testing.T) {
	server := aContainerWithItsOwnPostgres(t)

	var found []FoundDatabaseDTO
	_ = json.Unmarshal(serve(server, http.MethodGet, "/instances/helpdesk/databases/found", nil).Body.Bytes(), &found)
	if len(found) != 2 || found[0].DB != "helpdesk" || found[0].Owner != "helpdesk_app" || found[0].Port != 5432 {
		t.Fatalf("found %+v", found)
	}

	_ = server.instances.Annotate(context.Background(), "helpdesk", "databases", "helpdesk")
	_ = server.instances.Annotate(context.Background(), "helpdesk", "database.helpdesk.engine", "postgres")
	_ = server.instances.Annotate(context.Background(), "helpdesk", "database.helpdesk.db", "helpdesk")
	found = nil
	_ = json.Unmarshal(serve(server, http.MethodGet, "/instances/helpdesk/databases/found", nil).Body.Bytes(), &found)
	if len(found) != 1 || found[0].DB != "Bad-Name" {
		t.Errorf("a recorded database is offered again: %+v", found)
	}
}

// Adopting writes down what the engine said — owner and port included — and
// changes nothing in the container: no install, no snapshot, no credentials.
func TestAdoptingADatabaseOnlyTakesNote(t *testing.T) {
	server := aContainerWithItsOwnPostgres(t)

	body := shellOfPlan(t, serve(server, http.MethodPost, "/instances/helpdesk/databases/adopt/plan", AdoptDatabaseDTO{Engine: "postgres", DB: "helpdesk"}).Body.Bytes())
	for _, want := range []string{"user.croft.database.helpdesk.user helpdesk_app", "user.croft.database.helpdesk.adopted true", "user.croft.databases helpdesk"} {
		if !strings.Contains(body, want) {
			t.Errorf("the plan lacks %q:\n%s", want, body)
		}
	}
	for _, never := range []string{"snapshot", "apt-get", "CREATE", "db.env"} {
		if strings.Contains(body, never) {
			t.Errorf("adopting reaches %s:\n%s", never, body)
		}
	}

	if r := serve(server, http.MethodPost, "/instances/helpdesk/databases/adopt/plan", AdoptDatabaseDTO{Engine: "postgres", DB: "Bad-Name"}); r.Code != http.StatusBadRequest {
		t.Errorf("a name that cannot reach SQL unquoted was taken: %d", r.Code)
	}
	if r := serve(server, http.MethodPost, "/instances/helpdesk/databases/adopt/plan", AdoptDatabaseDTO{Engine: "postgres", DB: "ghost"}); r.Code != http.StatusBadRequest {
		t.Errorf("a database that is not there was taken: %d", r.Code)
	}
}

// Letting an adopted database go forgets it and never drops it.
func TestReleasingAnAdoptedDatabaseNeverDropsIt(t *testing.T) {
	server := aContainerWithItsOwnPostgres(t)
	ctx := context.Background()
	for key, value := range map[string]string{
		"databases": "helpdesk", "database.helpdesk.engine": "postgres", "database.helpdesk.db": "helpdesk",
		"database.helpdesk.user": "helpdesk_app", "database.helpdesk.adopted": "true",
	} {
		_ = server.instances.Annotate(ctx, "helpdesk", key, value)
	}

	recorder := serve(server, http.MethodGet, "/instances/helpdesk/databases/helpdesk/destroy/plan", nil)
	body := shellOfPlan(t, recorder.Body.Bytes()) + recorder.Body.String()
	if strings.Contains(body, "DROP") || strings.Contains(body, "snapshot") || !strings.Contains(body, "config unset") {
		t.Errorf("releasing:\n%s", body)
	}
	if !strings.Contains(body, "stay exactly as they are") {
		t.Errorf("the warning does not say the data stays:\n%s", body)
	}
}

// shellOfPlan is a plan answer as the commands it runs, one per line.
func shellOfPlan(t *testing.T, raw []byte) string {
	t.Helper()
	var response PlanResponse
	if err := json.Unmarshal(raw, &response); err != nil {
		t.Fatalf("not a plan: %s", raw)
	}
	out := ""
	for _, step := range response.Plan.Steps {
		out += step.Shell() + "\n"
	}
	return out
}
