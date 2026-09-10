package services_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	instanceServices "github.com/Hyzokaaa/opencroft/internal/instance/domain/services"
	"github.com/Hyzokaaa/opencroft/internal/instance/infrastructure/runtime"
	"github.com/Hyzokaaa/opencroft/internal/route/domain/entities"
	"github.com/Hyzokaaa/opencroft/internal/route/domain/enums"
	"github.com/Hyzokaaa/opencroft/internal/route/domain/services"
	routeMemory "github.com/Hyzokaaa/opencroft/internal/route/infrastructure/memory"
	"github.com/Hyzokaaa/opencroft/internal/shared/id"
)

func routing(t *testing.T) (*services.AddRoute, *services.EditRoute, *routeMemory.MemoryRouteRepository) {
	t.Helper()

	instances := runtime.NewMemoryInstanceRepository()
	routes := routeMemory.NewMemoryRouteRepository()

	create := instanceServices.NewCreateInstance(id.NewFake("test-"), instances)
	if _, err := create.Execute(context.Background(), instanceServices.CreateInstanceProps{
		Name: "helpdesk", Port: 3000,
	}); err != nil {
		t.Fatal(err)
	}

	return services.NewAddRoute(routes, instances), services.NewEditRoute(routes, instances), routes
}

func TestAddingARouteResolvesTheContainerToItsAddress(t *testing.T) {
	add, _, _ := routing(t)

	route, _, err := add.Prepare(context.Background(), services.AddRouteProps{
		Domain: "app.example.com", Target: "helpdesk",
	})
	if err != nil {
		t.Fatal(err)
	}

	if !strings.HasPrefix(route.Target, "10.") {
		t.Errorf("target is %q, which is not an address", route.Target)
	}
	// No port given: the container already said which one it listens on.
	if route.Port != 3000 {
		t.Errorf("port is %d, not the container's own", route.Port)
	}
}

func TestARouteIsRefusedForReasonsWorthReading(t *testing.T) {
	cases := map[string]struct {
		props services.AddRouteProps
		want  error
	}{
		"no domain":         {services.AddRouteProps{Target: "helpdesk"}, services.ErrDomainRequired},
		"not a domain":      {services.AddRouteProps{Domain: "not a domain", Target: "helpdesk"}, services.ErrDomainInvalid},
		"unknown container": {services.AddRouteProps{Domain: "a.example.com", Target: "nope"}, services.ErrTargetUnknown},
	}

	for label, test := range cases {
		t.Run(label, func(t *testing.T) {
			add, _, _ := routing(t)
			if _, _, err := add.Prepare(context.Background(), test.props); !errors.Is(err, test.want) {
				t.Fatalf("got %v, wanted %v", err, test.want)
			}
		})
	}
}

// Two vhosts claiming one name is a configuration nginx accepts and a person
// cannot reason about.
func TestADomainCannotBeServedTwice(t *testing.T) {
	add, _, routes := routing(t)
	ctx := context.Background()

	if _, err := add.Execute(ctx, services.AddRouteProps{Domain: "app.example.com", Target: "helpdesk"}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := add.Prepare(ctx, services.AddRouteProps{Domain: "app.example.com", Target: "helpdesk"}); !errors.Is(err, services.ErrDomainTaken) {
		t.Fatalf("the second one was allowed: %v", err)
	}

	all, _ := routes.FindAll(ctx)
	if len(all) != 1 {
		t.Fatalf("ended up with %d routes for one domain", len(all))
	}
}

// A domain on https must not silently drop to http because somebody changed
// which container serves it.
func TestEditingARouteKeepsItsCertificate(t *testing.T) {
	_, edit, routes := routing(t)
	ctx := context.Background()

	if err := routes.Write(ctx, entities.NewRoute(entities.RouteProps{
		Domain: "app.example.com", Target: "10.146.38.200", Port: 80,
		SSL: true, State: enums.StateManaged,
	})); err != nil {
		t.Fatal(err)
	}

	route, _, err := edit.Prepare(ctx, services.EditRouteProps{Domain: "app.example.com", Port: 8080})
	if err != nil {
		t.Fatal(err)
	}

	if !route.SSL {
		t.Error("editing the port turned https off")
	}
	if route.Target != "10.146.38.200" {
		t.Errorf("editing the port also moved the target to %q", route.Target)
	}
	if route.Port != 8080 {
		t.Errorf("the port did not change: %d", route.Port)
	}
}

// Writing our own file for a domain served from somebody else's block leaves
// nginx with two claiming the same name.
func TestAForeignVhostIsNeverRewritten(t *testing.T) {
	_, edit, routes := routing(t)
	ctx := context.Background()

	if err := routes.Write(ctx, entities.NewRoute(entities.RouteProps{
		Domain: "theirs.example.com", Target: "10.0.0.9", Port: 80,
		State: enums.StateUnmanaged, File: "/etc/nginx/sites-enabled/theirs",
	})); err != nil {
		t.Fatal(err)
	}

	_, _, err := edit.Prepare(ctx, services.EditRouteProps{Domain: "theirs.example.com", Port: 99})
	if !errors.Is(err, services.ErrRouteForeign) {
		t.Fatalf("got %v, wanted a refusal", err)
	}
}

func TestAnEditThatChangesNothingIsRefused(t *testing.T) {
	_, edit, routes := routing(t)
	ctx := context.Background()

	if err := routes.Write(ctx, entities.NewRoute(entities.RouteProps{
		Domain: "app.example.com", Target: "10.0.0.5", Port: 80, State: enums.StateManaged,
	})); err != nil {
		t.Fatal(err)
	}

	if _, _, err := edit.Prepare(ctx, services.EditRouteProps{Domain: "app.example.com", Port: 80}); err == nil {
		t.Fatal("an edit with nothing to change was accepted")
	}
}
