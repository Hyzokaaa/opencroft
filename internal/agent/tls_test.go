package agent

import (
	"context"
	"strings"
	"testing"

	certificateServices "github.com/Hyzokaaa/opencroft/internal/certificate/domain/services"
	routeEntities "github.com/Hyzokaaa/opencroft/internal/route/domain/entities"
	routeEnums "github.com/Hyzokaaa/opencroft/internal/route/domain/enums"
	"github.com/Hyzokaaa/opencroft/internal/shared/host"
)

// aDomainCertbotRenews is what taking over a certbot vhost leaves: croft's
// vhost, certbot's certificate, certbot's renewal.
func aDomainCertbotRenews(t *testing.T) (*Server, *host.Fake) {
	t.Helper()
	server, fake := testServer()
	if err := server.routes.Write(context.Background(), routeEntities.NewRoute(routeEntities.RouteProps{
		Domain: "dev.example.com", Target: "10.146.38.200", Port: 80, SSL: true,
		Certificates: "/etc/letsencrypt/live/dev.example.com",
		Paths:        []routeEntities.PathRoute{{Prefix: "/api/", Target: "10.146.38.200", Port: 3000, Strip: true}},
		State:        routeEnums.StateManaged,
	})); err != nil {
		t.Fatal(err)
	}
	fake.Files["/etc/letsencrypt/renewal/dev.example.com.conf"] = "authenticator = dns-ovh\n"
	return server, fake
}

func shellOf(server *Server, change tlsChange) string {
	body := ""
	for _, step := range server.tlsPlan(change, certificateServices.ChallengeDNS).Steps {
		body += step.Shell() + "\n"
	}
	return body
}

// Moving a domain off certbot serves croft's certificate, keeps its paths, and
// stops certbot renewing the one nothing serves — by moving its renewal file
// aside, after the vhost, never by deleting anything.
func TestADomainMovesOffCertbotWithoutLosingAnything(t *testing.T) {
	server, _ := aDomainCertbotRenews(t)

	change, err := server.tlsChangeFor(context.Background(), "dev.example.com")
	if err != nil {
		t.Fatal(err)
	}
	if change.route.CertDir() != "/var/lib/croft/certificates/dev.example.com" {
		t.Errorf("serves %s", change.route.CertDir())
	}
	if len(change.route.Paths) != 1 {
		t.Errorf("the paths were lost: %+v", change.route.Paths)
	}

	body := shellOf(server, change)
	moved := strings.Index(body, "mv /etc/letsencrypt/renewal/dev.example.com.conf "+takenOverDir+"/")
	written := strings.Index(body, "/etc/nginx/croft.d/dev.example.com.conf")
	if moved < 0 || written < 0 || moved < written {
		t.Errorf("certbot's renewal is not moved aside after the vhost:\n%s", body)
	}
	if strings.Contains(body, "rm ") || strings.Contains(body, "certbot delete") {
		t.Errorf("something is deleted:\n%s", body)
	}
}

// A certificate another vhost still reads has to go on being renewed.
func TestACertificateSomethingElseServesKeepsRenewing(t *testing.T) {
	server, _ := aDomainCertbotRenews(t)
	_ = server.routes.Write(context.Background(), routeEntities.NewRoute(routeEntities.RouteProps{
		Domain: "www.example.com", Target: "10.146.38.201", Port: 80, SSL: true,
		Certificates: "/etc/letsencrypt/live/dev.example.com",
		State:        routeEnums.StateUnmanaged, File: "/etc/nginx/sites-enabled/www",
	}))

	change, err := server.tlsChangeFor(context.Background(), "dev.example.com")
	if err != nil {
		t.Fatal(err)
	}
	if change.retire != "" || strings.Contains(shellOf(server, change), "renewal") {
		t.Errorf("certbot's renewal is stopped while www.example.com still reads it")
	}
}

// A domain already on croft's certificate has nothing to change.
func TestACroftCertificateIsNotIssuedAgain(t *testing.T) {
	server, _ := testServer()
	_ = server.routes.Write(context.Background(), routeEntities.NewRoute(routeEntities.RouteProps{
		Domain: "app.example.com", Target: "10.146.38.200", Port: 80, SSL: true,
		Certificates: "/var/lib/croft/certificates/app.example.com",
		State:        routeEnums.StateManaged,
	}))

	if _, err := server.tlsChangeFor(context.Background(), "app.example.com"); err == nil {
		t.Error("issued again")
	}
}

// A wildcard answers for its own name and one label below, as the authority
// reads it — not two labels below, and not a name that only ends the same.
func TestAWildcardCoversOneLabelBelow(t *testing.T) {
	for domain, want := range map[string]bool{
		"example.com":         true,
		"app.example.com":     true,
		"api.app.example.com": false,
		"badexample.com":      false,
		"example.org":         false,
	} {
		if got := covers("example.com", domain); got != want {
			t.Errorf("*.example.com covers %s: %v, want %v", domain, got, want)
		}
	}
}

// A wildcard is renewed as a wildcard, over DNS — asking for "*.example.com"
// as if it were a name would be refused.
func TestAWildcardIsRenewedAsOne(t *testing.T) {
	request := renewalOf("*.example.com")
	if !request.Wildcard || request.Domain != "example.com" || request.Challenge != certificateServices.ChallengeDNS {
		t.Errorf("renewed as %+v", request)
	}
	if renewalOf("app.example.com").Wildcard {
		t.Error("a single name is renewed as a wildcard")
	}
}

// Without DNS credentials there is no way to prove a wildcard, and saying so
// before anything runs is better than a failure a minute in.
func TestAWildcardNeedsDNSCredentials(t *testing.T) {
	server, _ := testServer()
	recorder := serve(server, "GET", "/certificates/wildcard/plan?domain=example.com", nil)
	if recorder.Code != 400 || !strings.Contains(recorder.Body.String(), "DNS") {
		t.Errorf("answered %d %s", recorder.Code, recorder.Body.String())
	}
	recorder = serve(server, "GET", "/certificates/wildcard/plan?domain=*.example.com", nil)
	if recorder.Code != 400 {
		t.Errorf("a name with a star was taken: %d", recorder.Code)
	}
}
