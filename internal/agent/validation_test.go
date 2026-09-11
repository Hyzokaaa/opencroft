package agent

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Hyzokaaa/opencroft/internal/certificate/infrastructure/pem"
	"github.com/Hyzokaaa/opencroft/internal/instance/infrastructure/runtime"
	routeMemory "github.com/Hyzokaaa/opencroft/internal/route/infrastructure/memory"
	"github.com/Hyzokaaa/opencroft/internal/shared/host"
)

// The agent is the security boundary: it runs as root and the panel does not.
// Everything below is a field that must never reach a command line.
func testServer() (*Server, *host.Fake) {
	fake := host.NewFake()
	return NewServer(
		runtime.NewDemoInstanceRepository(),
		routeMemory.NewMemoryRouteRepository(),
		pem.NewDemoCertificateRepository(),
		fake,
		"lxd",
		"test",
		"lxc",
	), fake
}

func post(t *testing.T, path string, body any) *httptest.ResponseRecorder {
	t.Helper()

	server, _ := testServer()
	encoded, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}

	request := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(encoded))
	recorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(recorder, request)
	return recorder
}

func validInstance() InstanceDTO {
	return InstanceDTO{
		Id: "01ABC", Name: "helpdesk", Image: "ubuntu:24.04",
		Address: "10.0.0.200", Port: 80, CPULimit: 4, MemLimit: "4GB",
		Created: "2026-09-10",
	}
}

func TestTheAgentAcceptsAReasonableInstance(t *testing.T) {
	if code := post(t, "/instances/plan", validInstance()).Code; code != http.StatusOK {
		t.Fatalf("a valid instance was refused with %d", code)
	}
}

func TestTheAgentRefusesDangerousInstanceFields(t *testing.T) {
	cases := map[string]func(*InstanceDTO){
		"a name with a shell separator":  func(i *InstanceDTO) { i.Name = "app;rm -rf /" },
		"a name with a space":            func(i *InstanceDTO) { i.Name = "app name" },
		"a name that is a flag":          func(i *InstanceDTO) { i.Name = "--force" },
		"an empty name":                  func(i *InstanceDTO) { i.Name = "" },
		"an image with a substitution":   func(i *InstanceDTO) { i.Image = "ubuntu$(id)" },
		"an image with a backtick":       func(i *InstanceDTO) { i.Image = "ubuntu`id`" },
		"an address that is a command":   func(i *InstanceDTO) { i.Address = "10.0.0.1; reboot" },
		"an address that is not one":     func(i *InstanceDTO) { i.Address = "not-an-address" },
		"a port below the range":         func(i *InstanceDTO) { i.Port = 0 },
		"a port above the range":         func(i *InstanceDTO) { i.Port = 70000 },
		"an absurd cpu limit":            func(i *InstanceDTO) { i.CPULimit = 99999 },
		"a memory limit with a pipe":     func(i *InstanceDTO) { i.MemLimit = "4GB|sh" },
		"metadata carrying a separator":  func(i *InstanceDTO) { i.Created = "2026-09-10; id" },
		"an identifier carrying a space": func(i *InstanceDTO) { i.Id = "01 ABC" },
	}

	for name, break_ := range cases {
		t.Run(name, func(t *testing.T) {
			instance := validInstance()
			break_(&instance)

			recorder := post(t, "/instances/plan", instance)
			if recorder.Code != http.StatusBadRequest {
				t.Fatalf("accepted with %d: %s", recorder.Code, recorder.Body.String())
			}
		})
	}
}

// A refused field must not appear anywhere in a plan, even quoted. The plan is
// what the privileged side is willing to run.
func TestARefusedNameNeverReachesAPlan(t *testing.T) {
	instance := validInstance()
	instance.Name = "app;rm -rf /"

	body := post(t, "/instances/plan", instance).Body.String()
	if strings.Contains(body, "rm -rf") {
		t.Fatalf("the refused name came back in the response: %s", body)
	}
}

func TestTheAgentRefusesBadRouteFields(t *testing.T) {
	cases := map[string]RouteDTO{
		"a domain with a space":      {Domain: "a b.com", Target: "10.0.0.1", Port: 80},
		"a domain that is a command": {Domain: "x.com;id", Target: "10.0.0.1", Port: 80},
		"no domain at all":           {Domain: "", Target: "10.0.0.1", Port: 80},
		"a target that is not an ip": {Domain: "x.com", Target: "somewhere", Port: 80},
		"a port out of range":        {Domain: "x.com", Target: "10.0.0.1", Port: 0},
	}

	for name, route := range cases {
		t.Run(name, func(t *testing.T) {
			if code := post(t, "/routes/plan", route).Code; code != http.StatusBadRequest {
				t.Fatalf("accepted with %d", code)
			}
		})
	}
}

func TestTheAgentAcceptsAReasonableRoute(t *testing.T) {
	route := RouteDTO{Domain: "app.example.com", Target: "10.0.0.200", Port: 3000}
	if code := post(t, "/routes/plan", route).Code; code != http.StatusOK {
		t.Fatalf("a valid route was refused with %d", code)
	}
}

// There is no endpoint that runs what it is given, and there must never be.
// This test fails the day somebody adds one.
func TestTheAgentExposesNoWayToRunAnArbitraryCommand(t *testing.T) {
	server, fake := testServer()

	for _, path := range []string{"/exec", "/run", "/command", "/shell", "/eval"} {
		request := httptest.NewRequest(http.MethodPost, path,
			strings.NewReader(`{"command":"rm -rf /"}`))
		recorder := httptest.NewRecorder()
		server.Handler().ServeHTTP(recorder, request)

		if recorder.Code != http.StatusNotFound {
			t.Errorf("%s answered %d — the agent should have no such endpoint", path, recorder.Code)
		}
	}

	if len(fake.Commands) != 0 {
		t.Errorf("something ran:\n%s", fake)
	}
}
