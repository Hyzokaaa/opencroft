package agent

import (
	"context"
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
	if answers, _ := byDomain["up.example.com"].Answers(); !answers {
		t.Error("up.example.com should answer")
	}
}
