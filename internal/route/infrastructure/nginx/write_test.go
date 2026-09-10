package nginx

import (
	"context"
	"errors"
	"testing"

	"github.com/Hyzokaaa/opencroft/internal/route/domain/entities"
	"github.com/Hyzokaaa/opencroft/internal/shared/host"
)

func aRoute() *entities.Route {
	return entities.NewRoute(entities.RouteProps{
		Domain: "app.example.com", Target: "10.0.0.5", Port: 3000,
	})
}

func TestWritingARouteRunsExactlyThePlanItShowed(t *testing.T) {
	fake := host.NewFake()
	repository := NewNginxRouteRepository(fake, "/etc/nginx/croft.d")
	route := aRoute()

	shown := repository.WritePlan(route)
	if err := repository.Write(context.Background(), route); err != nil {
		t.Fatalf("write: %v", err)
	}

	if len(fake.Commands) != len(shown.Steps) {
		t.Fatalf("showed %d steps, ran %d\n%s", len(shown.Steps), len(fake.Commands), fake)
	}
}

// nginx keeps running on its old configuration when a new file is rejected,
// so nothing looks broken — until the next reload, by anybody, fails and takes
// every site with it.
func TestARejectedVhostIsRemovedAgain(t *testing.T) {
	fake := host.NewFake()
	fake.Failures["nginx -t"] = errors.New("invalid configuration")

	repository := NewNginxRouteRepository(fake, "/etc/nginx/croft.d")

	if err := repository.Write(context.Background(), aRoute()); err == nil {
		t.Fatal("nginx rejected the file and Write reported success")
	}

	if _, still := fake.Files["/etc/nginx/croft.d/app.example.com.conf"]; still {
		t.Fatal("the rejected file was left on disk for the next reload to trip over")
	}
}

func TestAnAcceptedVhostStays(t *testing.T) {
	fake := host.NewFake()
	repository := NewNginxRouteRepository(fake, "/etc/nginx/croft.d")

	if err := repository.Write(context.Background(), aRoute()); err != nil {
		t.Fatal(err)
	}

	content, ok := fake.Files["/etc/nginx/croft.d/app.example.com.conf"]
	if !ok {
		t.Fatal("nothing was written")
	}
	if !contains(content, "proxy_pass http://10.0.0.5:3000") {
		t.Errorf("the vhost does not proxy where it should:\n%s", content)
	}
	if !contains(content, "acme-challenge") {
		t.Error("the vhost cannot answer an ACME challenge")
	}
}

// A generated file carries a hash of its own body, and that is the only reason
// a hand edit can be detected later.
func TestAWrittenVhostCarriesItsOwnHash(t *testing.T) {
	fake := host.NewFake()
	repository := NewNginxRouteRepository(fake, "/etc/nginx/croft.d")

	if err := repository.Write(context.Background(), aRoute()); err != nil {
		t.Fatal(err)
	}

	content := fake.Files["/etc/nginx/croft.d/app.example.com.conf"]
	if stateOf(content) != "managed" {
		t.Fatalf("a file we just wrote does not read as ours: %q", stateOf(content))
	}

	edited := content + "\n# a human was here\n"
	if stateOf(edited) != "adopted" {
		t.Fatalf("an edited file still reads as %q", stateOf(edited))
	}
}

func contains(haystack, needle string) bool {
	return len(haystack) >= len(needle) && indexOf(haystack, needle) >= 0
}

func indexOf(haystack, needle string) int {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return i
		}
	}
	return -1
}
