package agent

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"

	routeEntities "github.com/Hyzokaaa/opencroft/internal/route/domain/entities"
	routeEnums "github.com/Hyzokaaa/opencroft/internal/route/domain/enums"
)

func aRouteWithHTTPS(t *testing.T) *Server {
	t.Helper()
	server, _ := testServer()
	for _, props := range []routeEntities.RouteProps{
		{Domain: "app.example.com", Target: "10.146.38.200", Port: 80, SSL: true, State: routeEnums.StateManaged,
			Aliases: []routeEntities.Alias{{Domain: "old.customer.com", SSL: true}}},
		{Domain: "other.example.com", Target: "10.146.38.201", Port: 80, State: routeEnums.StateManaged},
	} {
		if err := server.routes.Write(context.Background(), routeEntities.NewRoute(props)); err != nil {
			t.Fatal(err)
		}
	}
	return server
}

// A customer's domain is served over http first, then given a certificate of
// its own, then moved to https — and its DNS is said to matter.
func TestAnAliasIsServedThenCertified(t *testing.T) {
	server := aRouteWithHTTPS(t)
	shell := shellOfPlan(t, serve(server, http.MethodGet, "/routes/app.example.com/aliases/help.customer.com/plan", nil).Body.Bytes())
	if !strings.Contains(shell, "croft cert issue help.customer.com --http") {
		t.Errorf("no certificate for the alias:\n%s", shell)
	}
	if strings.Count(shell, "nginx -t") != 2 {
		t.Errorf("not served over http before it is certified:\n%s", shell)
	}
}

// A name already answering — here, elsewhere, or as a route of its own — is
// refused before anything is written.
func TestANameAnswersInOnePlaceOnly(t *testing.T) {
	server := aRouteWithHTTPS(t)
	for _, alias := range []string{"old.customer.com", "other.example.com", "app.example.com", "not a domain"} {
		r := serve(server, http.MethodGet, "/routes/app.example.com/aliases/"+strings.ReplaceAll(alias, " ", "%20")+"/plan", nil)
		if r.Code != http.StatusBadRequest {
			t.Errorf("%s was taken: %d %s", alias, r.Code, r.Body.String())
		}
	}
}

// Letting a name go takes its own certificate with it.
func TestRemovingAnAliasRemovesItsCertificate(t *testing.T) {
	server := aRouteWithHTTPS(t)
	shell := shellOfPlan(t, serve(server, http.MethodGet, "/routes/app.example.com/aliases/old.customer.com/remove/plan", nil).Body.Bytes())
	if !strings.Contains(shell, "rm -rf /var/lib/croft/certificates/old.customer.com") {
		t.Errorf("plan:\n%s", shell)
	}
}

// A name whose DNS does not point here yet is not a failure: it is served
// over http, croft says why, and nothing is asked of Let's Encrypt.
func TestANameWaitingForItsDNSIsServedAndExplained(t *testing.T) {
	server := aRouteWithHTTPS(t)
	server.reachCheck = func(_ context.Context, domain string) error {
		return errors.New(domain + " has no DNS record yet")
	}

	recorder := serve(server, http.MethodPost, "/routes/app.example.com/aliases/help.customer.com", nil)
	body := recorder.Body.String()
	if strings.Contains(body, `"failed":true`) || !strings.Contains(body, "Waiting for its DNS") {
		t.Fatalf("answered:\n%s", body)
	}

	route, _, _ := server.owned(context.Background(), "app.example.com")
	alias, ok := route.Alias("help.customer.com")
	if !ok || alias.SSL {
		t.Fatalf("not served over http meanwhile: %+v", route.Aliases)
	}
	var listed []RouteDTO
	_ = json.Unmarshal(serve(server, http.MethodGet, "/routes", nil).Body.Bytes(), &listed)
	for _, r := range listed {
		for _, a := range r.Aliases {
			if a.Domain == "help.customer.com" && !strings.Contains(a.Waiting, "no DNS record") {
				t.Errorf("the reason does not reach the panel: %+v", a)
			}
		}
	}
}

// Removing a domain never takes the names set up with it: the first of them
// becomes the route's own, with its certificate, paths and other names.
func TestRemovingADomainKeepsItsOtherNames(t *testing.T) {
	server := aRouteWithHTTPS(t)
	if r := serve(server, http.MethodDelete, "/routes/app.example.com", nil); r.Code != http.StatusNoContent {
		t.Fatalf("removed with %d %s", r.Code, r.Body.String())
	}

	ctx := context.Background()
	if gone, _ := server.routes.FindByDomain(ctx, "app.example.com"); gone != nil {
		t.Errorf("app.example.com is still served: %+v", gone)
	}
	promoted, _ := server.routes.FindByDomain(ctx, "old.customer.com")
	if promoted == nil || !promoted.SSL || promoted.Target != "10.146.38.200" ||
		promoted.CertDir() != "/var/lib/croft/certificates/old.customer.com" || len(promoted.Aliases) != 0 {
		t.Errorf("old.customer.com became %+v", promoted)
	}
}
