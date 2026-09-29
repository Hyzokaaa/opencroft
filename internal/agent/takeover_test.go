package agent

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"

	routeEntities "github.com/Hyzokaaa/opencroft/internal/route/domain/entities"
	routeEnums "github.com/Hyzokaaa/opencroft/internal/route/domain/enums"
	"github.com/Hyzokaaa/opencroft/internal/shared/host"
)

const handWritten = "/etc/nginx/sites-enabled/dev.example.com"

// aHandWrittenDomain is a vhost set up before croft, the way certbot leaves
// one: on https, its certificate in certbot's directory.
func aHandWrittenDomain(t *testing.T) (*Server, *host.Fake) {
	t.Helper()
	server, fake := testServer()
	if err := server.routes.Write(context.Background(), routeEntities.NewRoute(routeEntities.RouteProps{
		Domain: "dev.example.com", Target: "10.146.38.200", Port: 80, SSL: true,
		Certificates: "/etc/letsencrypt/live/dev.example.com",
		State:        routeEnums.StateUnmanaged, File: handWritten,
	})); err != nil {
		t.Fatal(err)
	}
	return server, fake
}

// Taking over carries everything a visitor depends on — where it goes, the
// certificate — so that nothing they see changes, and moves the original aside
// rather than deleting it.
func TestTakingOverADomainKeepsWhatVisitorsSee(t *testing.T) {
	server, _ := aHandWrittenDomain(t)

	t1, err := server.takeOverOf(context.Background(), "dev.example.com")
	if err != nil {
		t.Fatal(err)
	}
	if t1.route.Target != "10.146.38.200" || t1.route.Port != 80 || !t1.route.SSL ||
		t1.route.Certificates != "/etc/letsencrypt/live/dev.example.com" {
		t.Errorf("taken over as %+v", t1.route)
	}
	if t1.route.State != routeEnums.StateManaged {
		t.Errorf("croft does not manage it afterwards: %s", t1.route.State)
	}

	body := ""
	for _, step := range server.takeOverPlan(t1).Steps {
		body += step.Shell() + "\n"
	}
	if !strings.Contains(body, "mv "+handWritten+" "+takenOverDir+"/") {
		t.Errorf("the original is not moved aside:\n%s", body)
	}
	if strings.Contains(body, "rm ") {
		t.Errorf("taking over deletes something:\n%s", body)
	}
}

// A file that also serves other domains would take them down with it.
func TestAFileServingOtherDomainsIsNotTakenOver(t *testing.T) {
	server, _ := aHandWrittenDomain(t)
	_ = server.routes.Write(context.Background(), routeEntities.NewRoute(routeEntities.RouteProps{
		Domain: "other.example.com", Target: "10.146.38.201", Port: 80,
		State: routeEnums.StateUnmanaged, File: handWritten,
	}))

	if _, err := server.takeOverOf(context.Background(), "dev.example.com"); err == nil ||
		!strings.Contains(err.Error(), "other.example.com") {
		t.Errorf("got %v", err)
	}
}

// If nginx refuses the new file, the original goes back where it was and
// croft's file goes — nginx is left as it was found, not with neither.
func TestARefusedTakeOverPutsTheOriginalBack(t *testing.T) {
	server, fake := aHandWrittenDomain(t)
	fake.Failures["nginx -t"] = errors.New("invalid configuration")

	recorder := serve(server, http.MethodPost, "/routes/dev.example.com/takeover", nil)
	if !strings.Contains(recorder.Body.String(), "is back where it was") {
		t.Fatalf("answered %s", recorder.Body.String())
	}

	putBack := false
	for _, command := range fake.Commands {
		if strings.HasPrefix(command, "mv "+takenOverDir) && strings.HasSuffix(command, handWritten) {
			putBack = true
		}
	}
	if !putBack {
		t.Error("the original was left out of nginx's way")
	}
}
