package agent

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"testing"

	routeEntities "github.com/Hyzokaaa/opencroft/internal/route/domain/entities"
)

// listening opens a port on loopback for the length of the test, so a route
// can point at something that accepts the connection.
func listening(t *testing.T) int {
	t.Helper()
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	t.Cleanup(backend.Close)
	_, port, _ := net.SplitHostPort(backend.Listener.Addr().String())
	n, _ := strconv.Atoi(port)
	return n
}

// closedPort is a port nothing listens on: one that was open and is not.
func closedPort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := l.Addr().(*net.TCPAddr).Port
	_ = l.Close()
	return port
}

func routeTo(t *testing.T, server *Server, domain string, port int) {
	t.Helper()
	route := routeEntities.NewRoute(routeEntities.RouteProps{Domain: domain, Target: "127.0.0.1", Port: port})
	if err := server.routes.Write(context.Background(), route); err != nil {
		t.Fatal(err)
	}
}

// A process accepting the connection says nothing about what it answers: an
// app that started badly listens and returns 502. The Domains page shows what
// a visitor gets, so the page is asked for, by the domain's own name.
func TestADomainSaysWhatItsFrontPageAnswered(t *testing.T) {
	server, _ := testServer()
	port := listening(t)
	routeTo(t, server, "api.example.com", port)

	var asked []string
	server.servingCheck = func(_ context.Context, address string, p int, domain string) ServingDTO {
		asked = append(asked, address+":"+strconv.Itoa(p)+" "+domain)
		return ServingDTO{Domain: domain, Status: http.StatusBadGateway}
	}

	var routes []RouteDTO
	_ = json.Unmarshal(serve(server, http.MethodGet, "/routes", nil).Body.Bytes(), &routes)

	want := "127.0.0.1:" + strconv.Itoa(port) + " api.example.com"
	if len(asked) != 1 || asked[0] != want {
		t.Fatalf("asked %v, want %q", asked, want)
	}
	if len(routes) != 1 || routes[0].Serving == nil || routes[0].Serving.Status != http.StatusBadGateway {
		t.Errorf("the answer did not reach the panel: %+v", routes)
	}
}

// Where nothing listens, "nothing listening" is already the answer. Asking
// again would cost the page's whole timeout on every refresh to say the same.
func TestADomainWithNothingListeningIsNotAskedForItsPage(t *testing.T) {
	server, _ := testServer()
	routeTo(t, server, "api.example.com", closedPort(t))

	asked := 0
	server.servingCheck = func(_ context.Context, _ string, _ int, domain string) ServingDTO {
		asked++
		return ServingDTO{Domain: domain, Status: http.StatusOK}
	}

	var routes []RouteDTO
	_ = json.Unmarshal(serve(server, http.MethodGet, "/routes", nil).Body.Bytes(), &routes)

	if asked != 0 {
		t.Errorf("asked %d times where nothing listens", asked)
	}
	if len(routes) != 1 || routes[0].Answers || routes[0].Serving != nil {
		t.Errorf("want nothing listening and no page answer: %+v", routes)
	}
}

// The agent opens the sockets; the panel only reads what it was told. The
// client once dropped the answer on the way, and every route read as
// listening — so "nothing listening" never showed anywhere.
func TestWhatTheAgentLearntOfARouteReachesThePanel(t *testing.T) {
	server, _ := testServer()
	routeTo(t, server, "up.example.com", listening(t))
	routeTo(t, server, "down.example.com", closedPort(t))

	socket := filepath.Join(t.TempDir(), "agent.sock")
	listener, err := net.Listen("unix", socket)
	if err != nil {
		t.Skipf("no unix sockets here: %v", err)
	}
	go func() { _ = http.Serve(listener, server.Handler()) }()
	t.Cleanup(func() { _ = listener.Close() })

	routes, err := newClient(socket).Routes(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	byDomain := map[string]*routeEntities.Route{}
	for _, r := range routes {
		byDomain[r.Domain] = r
	}
	if answers, checked := byDomain["down.example.com"].Answers(); !checked || answers {
		t.Errorf("down.example.com: answers=%v checked=%v, want known not to answer", answers, checked)
	}
	up := byDomain["up.example.com"]
	if answers, _ := up.Answers(); !answers {
		t.Error("up.example.com should answer")
	}
	if up.Serving() == nil || up.Serving().Status != http.StatusOK {
		t.Errorf("up.example.com's page answer was lost: %+v", up.Serving())
	}
}
