package services

import (
	"context"
	"errors"
	"strings"
	"testing"

	instanceEntities "github.com/Hyzokaaa/opencroft/internal/instance/domain/entities"
	"github.com/Hyzokaaa/opencroft/internal/instance/infrastructure/runtime"
	"github.com/Hyzokaaa/opencroft/internal/project/domain/entities"
	"github.com/Hyzokaaa/opencroft/internal/project/infrastructure/files"
	"github.com/Hyzokaaa/opencroft/internal/shared/host"
)

// aHost has one declared project, open-helpdesk, holding the web; the
// backend is labelled with a project nobody declared here — it came from
// another host.
func aHost(t *testing.T) (*Projects, *host.Fake) {
	t.Helper()
	ctx := context.Background()

	fake := host.NewFake()
	fake.Dirs[files.Dir] = []string{"open-helpdesk.conf", "README"}
	fake.Files[files.Dir+"/open-helpdesk.conf"] = "# comment\ndescription = The helpdesk and its API\n"

	instances := runtime.NewMemoryInstanceRepository()
	for _, props := range []instanceEntities.InstanceProps{
		{Name: "web", Project: "open-helpdesk"},
		{Name: "api", Project: "migrated"},
		{Name: "loose"},
	} {
		if err := instances.Create(ctx, instanceEntities.NewInstance(props)); err != nil {
			t.Fatal(err)
		}
	}
	return NewProjects(files.NewFileProjectRepository(fake, "lxc"), instances), fake
}

// A project is what is declared and what containers are labelled with — each
// on its own is enough, and the listing says which it is.
func TestAProjectIsDeclaredOrFoundOnItsContainers(t *testing.T) {
	projects, _ := aHost(t)

	all, err := projects.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 2 {
		t.Fatalf("got %+v", all)
	}
	migrated, helpdesk := all[0], all[1]
	if helpdesk.Name != "open-helpdesk" || !helpdesk.Declared ||
		helpdesk.Description != "The helpdesk and its API" || strings.Join(helpdesk.Instances, ",") != "web" {
		t.Errorf("declared: %+v", helpdesk)
	}
	if migrated.Name != "migrated" || migrated.Declared || strings.Join(migrated.Instances, ",") != "api" {
		t.Errorf("found: %+v", migrated)
	}
}

// Declaring writes a file, not a row: a project survives losing the panel.
func TestDeclaringAProjectWritesAFileOnTheHost(t *testing.T) {
	projects, _ := aHost(t)

	p, err := projects.PrepareDeclare(context.Background(), entities.Declaration{Name: "billing", Description: " Invoices "})
	if err != nil {
		t.Fatal(err)
	}
	shell := p.Shell()
	if !strings.Contains(shell, files.Dir+"/billing.conf") || !strings.Contains(shell, "description = Invoices") {
		t.Errorf("plan:\n%s", shell)
	}

	for _, bad := range []entities.Declaration{
		{Name: "Billing"}, {Name: "../etc"}, {Name: "-x"}, {Name: "a b"},
		{Name: "ok", Description: "two\nlines"},
	} {
		if _, err := projects.PrepareDeclare(context.Background(), bad); err == nil {
			t.Errorf("accepted %+v", bad)
		}
	}
}

// A project found only on a container is declared as it is, in one step.
func TestAFoundProjectIsDeclaredWithoutMovingAnything(t *testing.T) {
	projects, _ := aHost(t)

	p, err := projects.PrepareDeclare(context.Background(), entities.Declaration{Name: "migrated"})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(p.Shell(), "config set") {
		t.Errorf("declaring touched a container:\n%s", p.Shell())
	}
}

// A project with containers is not removed: their labels would name nothing.
func TestAProjectWithContainersIsNotRemoved(t *testing.T) {
	projects, _ := aHost(t)

	_, err := projects.PrepareRemove(context.Background(), "open-helpdesk")
	if !errors.Is(err, ErrProjectNotEmpty) || !strings.Contains(err.Error(), "web") {
		t.Errorf("got %v", err)
	}
	if _, err := projects.PrepareRemove(context.Background(), "migrated"); !errors.Is(err, ErrProjectUnknown) {
		t.Errorf("an undeclared project: %v", err)
	}
}

// Moving a container is its label and nothing else; only into a declared
// project, and out of any into none.
func TestAContainerMovesByItsLabel(t *testing.T) {
	projects, _ := aHost(t)
	ctx := context.Background()

	p, err := projects.PrepareAssign(ctx, "loose", "open-helpdesk")
	if err != nil {
		t.Fatal(err)
	}
	if p.Shell() != "lxc config set loose user.croft.project open-helpdesk" {
		t.Errorf("plan: %s", p.Shell())
	}

	p, err = projects.PrepareAssign(ctx, "web", "")
	if err != nil {
		t.Fatal(err)
	}
	if p.Shell() != "lxc config unset web user.croft.project" {
		t.Errorf("plan: %s", p.Shell())
	}

	if _, err := projects.PrepareAssign(ctx, "loose", "typo"); !errors.Is(err, ErrUndeclared) {
		t.Errorf("into a name nobody declared: %v", err)
	}
	if _, err := projects.PrepareAssign(ctx, "web", "open-helpdesk"); !errors.Is(err, ErrNothingToChange) {
		t.Errorf("into the one it is in: %v", err)
	}
	if _, err := projects.PrepareAssign(ctx, "ghost", ""); !errors.Is(err, ErrInstanceUnknown) {
		t.Errorf("a container that is not there: %v", err)
	}
}
