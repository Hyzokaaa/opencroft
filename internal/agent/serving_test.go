package agent

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
)

// A site has no process, so the only honest "is it up" is what a visitor gets.
// Its web server being active said nothing about a 502 from behind it.
func TestASiteSaysWhatItsFrontPageAnswered(t *testing.T) {
	server, _ := aFoundSite()
	siteOnHelpdesk(t, server)

	var asked []string
	server.servingCheck = func(_ context.Context, address string, port int, domain string) ServingDTO {
		asked = append(asked, address+":"+strconv.Itoa(port)+" "+domain)
		return ServingDTO{Domain: domain, Status: http.StatusBadGateway}
	}

	var body ServicesResponse
	_ = json.Unmarshal(serve(server, http.MethodGet, "/instances/helpdesk/services", nil).Body.Bytes(), &body)

	if len(asked) != 1 || asked[0] != "10.146.38.200:80 dev.openhelpdesk.dev" {
		t.Fatalf("asked %v, want the container's own address with the site's name", asked)
	}
	if len(body.Services) != 1 || body.Services[0].Serving == nil || body.Services[0].Serving.Status != http.StatusBadGateway {
		t.Errorf("the answer did not reach the panel: %+v", body.Services)
	}
}

// A service with a process says so through its unit; asking it over http
// would be a second, different answer to the same question.
func TestOnlyASiteIsAskedForItsPage(t *testing.T) {
	server, _ := aFoundSite()
	for key, value := range map[string]string{
		"services":         "api",
		"service.api.repo": "https://github.com/user/api.git",
		"service.api.port": "3000",
	} {
		if err := server.instances.Annotate(context.Background(), "helpdesk", key, value); err != nil {
			t.Fatal(err)
		}
	}
	asked := 0
	server.servingCheck = func(_ context.Context, _ string, _ int, domain string) ServingDTO {
		asked++
		return ServingDTO{Domain: domain, Status: http.StatusOK}
	}

	serve(server, http.MethodGet, "/instances/helpdesk/services", nil)

	if asked != 0 {
		t.Errorf("asked %d times with no site adopted", asked)
	}
}

// What a visitor gets depends on the name asked for: the container's nginx
// picks the site by it. Asking by address alone reaches the welcome page.
func TestTheSiteIsAskedForByItsName(t *testing.T) {
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Host != "dev.example.com:"+portOf(t, r.Context()) && r.Host != "dev.example.com" {
			http.NotFound(w, r)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer origin.Close()

	address, port := split(t, origin.Listener.Addr().String())
	answer := askSite(context.Background(), address, port, "dev.example.com")

	if answer.Status != http.StatusOK || answer.Problem != "" {
		t.Errorf("answered %+v", answer)
	}
}

// Nothing listening is not a status code: it is said as nothing answering.
func TestNothingListeningIsSaidAsNoAnswer(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	address, port := split(t, listener.Addr().String())
	_ = listener.Close()

	answer := askSite(context.Background(), address, port, "dev.example.com")

	if answer.Status != 0 || answer.Problem == "" {
		t.Errorf("answered %+v", answer)
	}
}

// A redirect to another name leaves the container: following it would test
// someone else's server and report it as this site.
func TestARedirectElsewhereIsNotFollowed(t *testing.T) {
	// The redirect is dialled to the container like every request, so the
	// same server stands in for the other name and answers it happily.
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/elsewhere" {
			w.WriteHeader(http.StatusOK)
			return
		}
		http.Redirect(w, r, "http://somewhere.example.org:"+portOf(t, r.Context())+"/elsewhere", http.StatusMovedPermanently)
	}))
	defer origin.Close()

	address, port := split(t, origin.Listener.Addr().String())
	answer := askSite(context.Background(), address, port, "dev.example.com")

	if answer.Status != http.StatusMovedPermanently {
		t.Errorf("answered %+v, want the redirect itself", answer)
	}
}

func split(t *testing.T, hostport string) (string, int) {
	t.Helper()
	host, p, err := net.SplitHostPort(hostport)
	if err != nil {
		t.Fatal(err)
	}
	port, _ := strconv.Atoi(p)
	return host, port
}

// portOf is the port the request reached, as the Host header carries it when
// it is not the default one.
func portOf(t *testing.T, ctx context.Context) string {
	t.Helper()
	addr, _ := ctx.Value(http.LocalAddrContextKey).(net.Addr)
	if addr == nil {
		return ""
	}
	_, port, _ := net.SplitHostPort(addr.String())
	return port
}
