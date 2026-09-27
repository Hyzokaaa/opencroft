package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Hyzokaaa/opencroft/internal/shared/host"
)

const checkout = "/opt/open-helpdesk/backend"

// aFoundUnit is a container with one unit written by hand, the way an install
// script leaves one: its own user, its own .env, a checkout on a branch with
// one file edited in place.
func aFoundUnit() (*Server, *host.Fake) {
	server, fake := testServer()
	git := "lxc exec helpdesk -- git -c safe.directory=" + checkout + " -C " + checkout + " "

	fake.Responses["lxc exec helpdesk -- sh -lc for f in"] = "openhelpdesk-backend"
	fake.Responses["lxc exec helpdesk -- systemctl show openhelpdesk-backend"] = strings.Join([]string{
		"WorkingDirectory=" + checkout,
		"User=openhelpdesk",
		"EnvironmentFiles=" + checkout + "/.env (ignore_errors=no)",
		"ExecStart={ path=/usr/bin/node ; argv[]=/usr/bin/node " + checkout + "/dist/main ; ignore_errors=no }",
	}, "\n")
	fake.Responses[git+"remote get-url origin"] = "https://github.com/user/backend.git"
	fake.Responses[git+"rev-parse --abbrev-ref HEAD"] = "dev"
	fake.Responses[git+"rev-parse HEAD"] = "2a7851ea3887c21d2634dcc3f05c3d6ede92406a"
	fake.Responses[git+"diff --name-only HEAD"] = "package-lock.json"
	fake.Responses["lxc exec helpdesk -- ls -A "+checkout] = "package.json package-lock.json src"
	fake.Responses["lxc exec helpdesk -- cat "+checkout+"/package.json"] = `{"scripts":{"build":"nest build","start":"node dist/main"}}`

	return server, fake
}

func serve(server *Server, method, path string, body any) *httptest.ResponseRecorder {
	var reader *bytes.Reader
	if body != nil {
		encoded, _ := json.Marshal(body)
		reader = bytes.NewReader(encoded)
	} else {
		reader = bytes.NewReader(nil)
	}
	recorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(recorder, httptest.NewRequest(method, path, reader))
	return recorder
}

// Everything adoption records is read off the container — the unit says where
// the code is and who runs it, the checkout where it came from — and what a
// deployment would discard is said before anybody agrees to one.
func TestAdoptionIsReadOffTheContainer(t *testing.T) {
	server, _ := aFoundUnit()

	recorder := serve(server, http.MethodGet, "/instances/helpdesk/units/openhelpdesk-backend/adoption", nil)
	if recorder.Code != http.StatusOK {
		t.Fatalf("answered %d: %s", recorder.Code, recorder.Body.String())
	}

	var found AdoptionDTO
	if err := json.Unmarshal(recorder.Body.Bytes(), &found); err != nil {
		t.Fatal(err)
	}

	if found.Problem != "" {
		t.Fatalf("refused: %s", found.Problem)
	}
	if found.Path != checkout || found.RunAs != "openhelpdesk" || found.EnvFile != checkout+"/.env" {
		t.Errorf("the unit was misread: %+v", found)
	}
	if found.Repo != "https://github.com/user/backend.git" || found.Branch != "dev" {
		t.Errorf("the checkout was misread: %+v", found)
	}
	if len(found.Changed) != 1 || found.Changed[0] != "package-lock.json" {
		t.Errorf("what a deployment would discard is not said: %v", found.Changed)
	}
	// It is running already; the runtime under it does not need installing
	// again on every deployment.
	if strings.Join(found.Install, " && ") != "npm ci" || strings.Join(found.Build, "") != "npm run build" {
		t.Errorf("proposed install %v, build %v", found.Install, found.Build)
	}
}

func TestAdoptingWritesDownWhatWasFoundAndNothingElse(t *testing.T) {
	server, _ := aFoundUnit()

	recorder := serve(server, http.MethodPost, "/instances/helpdesk/units/openhelpdesk-backend/adopt/plan",
		AdoptDTO{Name: "backend", Install: []string{"npm ci"}, Build: []string{"npm run build"}, Port: 3000})
	if recorder.Code != http.StatusOK {
		t.Fatalf("answered %d: %s", recorder.Code, recorder.Body.String())
	}

	var response PlanResponse
	_ = json.Unmarshal(recorder.Body.Bytes(), &response)

	body := ""
	for _, step := range response.Plan.Steps {
		if step.Argv[1] != "config" {
			t.Errorf("adopting would run %s", step.Shell())
		}
		body += step.Shell() + "\n"
	}
	for _, wanted := range []string{
		"service.backend.adopted-unit openhelpdesk-backend",
		"service.backend.repo https://github.com/user/backend.git",
		"service.backend.branch dev",
		"service.backend.path " + checkout,
	} {
		if !strings.Contains(body, wanted) {
			t.Errorf("never records %q:\n%s", wanted, body)
		}
	}
}

// Only what croft found can be taken on: its own units are services already,
// and anything else — sshd, say — is not something anybody offered it.
func TestOnlyAFoundUnitCanBeAdopted(t *testing.T) {
	server, _ := aFoundUnit()

	for _, unit := range []string{"ssh", "croft-backend"} {
		recorder := serve(server, http.MethodPost, "/instances/helpdesk/units/"+unit+"/adopt/plan",
			AdoptDTO{Name: "backend"})
		if recorder.Code == http.StatusOK {
			t.Errorf("%s was adopted", unit)
		}
	}
}

// Adoption decides which unit croft restarts and who it hands a checkout to,
// so it is read from the container and never from a request. A deployment
// that claims to be adopted is deployed as what it is.
func TestADeploymentCannotClaimToBeAdopted(t *testing.T) {
	server, _ := testServer()

	claim := aService()
	claim.Adopted = &AdoptedDTO{Unit: "ssh", RunAs: "root"}

	var response PlanResponse
	recorder := serve(server, http.MethodPost, "/instances/helpdesk/services/deploy/plan", claim)
	_ = json.Unmarshal(recorder.Body.Bytes(), &response)

	body := ""
	for _, step := range response.Plan.Steps {
		body += step.Shell() + "\n"
	}
	if strings.Contains(body, "restart ssh") || !strings.Contains(body, "croft-backend") {
		t.Errorf("the claim was believed:\n%s", body)
	}
}

func adoptedOnHelpdesk(t *testing.T, server *Server) {
	t.Helper()
	ctx := context.Background()
	for key, value := range map[string]string{
		"services":                     "backend",
		"service.backend.repo":         "https://github.com/user/backend.git",
		"service.backend.branch":       "dev",
		"service.backend.path":         checkout,
		"service.backend.adopted-unit": "openhelpdesk-backend",
		"service.backend.run-as":       "openhelpdesk",
	} {
		if err := server.instances.Annotate(ctx, "helpdesk", key, value); err != nil {
			t.Fatal(err)
		}
	}
}

// Once adopted, the next deployment is of the unit that was there, from the
// checkout that was there — whatever path or environment the request carried.
func TestAnAdoptedServiceIsDeployedWhereItAlreadyIs(t *testing.T) {
	server, _ := testServer()
	adoptedOnHelpdesk(t, server)

	request := aService()
	request.Path = "/srv/elsewhere"
	request.Env = map[string]string{"JWT_SECRET": "overwrite"}
	request.Start = ""

	recorder := serve(server, http.MethodPost, "/instances/helpdesk/services/deploy/plan", request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("answered %d: %s", recorder.Code, recorder.Body.String())
	}
	var response PlanResponse
	_ = json.Unmarshal(recorder.Body.Bytes(), &response)

	body := ""
	for _, step := range response.Plan.Steps {
		body += step.Shell() + "\n"
	}
	for _, forbidden := range []string{"/srv/elsewhere", "JWT_SECRET", "croft-backend"} {
		if strings.Contains(body, forbidden) {
			t.Errorf("the plan carries %q:\n%s", forbidden, body)
		}
	}
	if !strings.Contains(body, "systemctl restart openhelpdesk-backend") ||
		!strings.Contains(body, "-C "+checkout+" fetch") {
		t.Errorf("not deployed where it is:\n%s", body)
	}
}

// An adopted unit is a service now: listed once, as one, with its own state.
func TestAnAdoptedUnitIsListedOnce(t *testing.T) {
	server, fake := aFoundUnit()
	adoptedOnHelpdesk(t, server)
	fake.Responses["lxc exec helpdesk -- systemctl is-active"] = "active"

	var body ServicesResponse
	_ = json.Unmarshal(serve(server, http.MethodGet, "/instances/helpdesk/services", nil).Body.Bytes(), &body)

	if len(body.External) != 0 {
		t.Errorf("still listed as found: %v", body.External)
	}
	if len(body.Services) != 1 || body.Services[0].Adopted == nil || body.Services[0].State != "active" {
		t.Errorf("not listed as the adopted service it is: %+v", body.Services)
	}
}

// Croft did not put an adopted service there, so letting go of it takes
// nothing away: no snapshot is needed, because nothing is removed.
func TestLettingGoOfAnAdoptedServiceRemovesNothing(t *testing.T) {
	server, _ := testServer()
	adoptedOnHelpdesk(t, server)

	var response RollbackResponse
	recorder := serve(server, http.MethodGet, "/instances/helpdesk/services/backend/destroy/plan", nil)
	if recorder.Code != http.StatusOK {
		t.Fatalf("answered %d: %s", recorder.Code, recorder.Body.String())
	}
	_ = json.Unmarshal(recorder.Body.Bytes(), &response)

	for _, step := range response.Plan.Steps {
		if step.Argv[1] != "config" {
			t.Errorf("letting go would run %s", step.Shell())
		}
	}
}
