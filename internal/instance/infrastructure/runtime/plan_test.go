package runtime

import (
	"context"
	"strings"
	"testing"

	"github.com/Hyzokaaa/opencroft/internal/instance/domain/entities"
	"github.com/Hyzokaaa/opencroft/internal/shared/host"
)

func sample() *entities.Instance {
	return entities.NewInstance(entities.InstanceProps{
		Id: "01ABC", Name: "helpdesk", Image: "ubuntu:24.04",
		Address: "10.0.0.200", Port: 3000, CPULimit: 4, MemLimit: "4GB",
		Created: "2026-09-10", Managed: true,
	})
}

// The promise this product makes is that the plan you approved is the work
// that runs. Not a preview generated alongside it — the same list. This test
// is that promise, and it is the one that must never be deleted.
func TestCreateRunsExactlyThePlanItShowed(t *testing.T) {
	fake := host.NewFake()
	repository := NewCLIInstanceRepository(fake, "lxc", FlavorLXD)
	instance := sample()

	shown := repository.CreatePlan(instance)

	if err := repository.Create(context.Background(), instance); err != nil {
		t.Fatalf("create: %v", err)
	}

	if len(fake.Commands) != len(shown.Steps) {
		t.Fatalf("showed %d steps and ran %d commands\n%s", len(shown.Steps), len(fake.Commands), fake)
	}

	for i, step := range shown.Steps {
		if fake.Commands[i] != step.Shell() {
			t.Errorf("step %d\n  shown: %s\n  ran:   %s", i+1, step.Shell(), fake.Commands[i])
		}
	}
}

func TestDeleteRunsExactlyThePlanItShowed(t *testing.T) {
	fake := host.NewFake()
	repository := NewCLIInstanceRepository(fake, "lxc", FlavorLXD)

	shown := repository.DeletePlan("helpdesk")
	if err := repository.Delete(context.Background(), "helpdesk"); err != nil {
		t.Fatalf("delete: %v", err)
	}

	if len(fake.Commands) != 1 || fake.Commands[0] != shown.Steps[0].Shell() {
		t.Fatalf("shown %q, ran %v", shown.Steps[0].Shell(), fake.Commands)
	}
}

// A plan with an empty target produced `proxy_pass http://:0` on a live host.
// Nothing should be describable as a command with a blank in it.
func TestPlanNeverContainsAnEmptyArgument(t *testing.T) {
	fake := host.NewFake()
	repository := NewCLIInstanceRepository(fake, "lxc", FlavorLXD)

	for _, step := range repository.CreatePlan(sample()).Steps {
		for i, argument := range step.Argv {
			if strings.TrimSpace(argument) == "" {
				t.Errorf("%q has an empty argument at %d", step.Describe, i)
			}
		}
	}
}

// The address is pinned before the container ever boots. A DHCP lease that
// changes on restart silently breaks the proxy pointing at it, and that was
// the first real bug this project had.
func TestCreatePinsTheAddressBeforeStarting(t *testing.T) {
	fake := host.NewFake()
	repository := NewCLIInstanceRepository(fake, "lxc", FlavorLXD)

	steps := repository.CreatePlan(sample()).Steps

	pinned, started := -1, -1
	for i, step := range steps {
		switch {
		case strings.Contains(step.Shell(), "ipv4.address"):
			pinned = i
		case strings.HasSuffix(step.Shell(), "start helpdesk"):
			started = i
		}
	}

	if pinned < 0 {
		t.Fatal("the plan never pins an address")
	}
	if started < 0 {
		t.Fatal("the plan never starts the container")
	}
	if pinned > started {
		t.Error("the address is pinned after the container starts, which is too late")
	}
}

// Overriding an inherited device fails when the device is already local. That
// is expected, so the step is marked optional and execution carries on.
func TestAnOptionalStepThatFailsDoesNotStopTheWork(t *testing.T) {
	fake := host.NewFake()
	fake.Failures["device override"] = errAlready

	repository := NewCLIInstanceRepository(fake, "lxc", FlavorLXD)
	if err := repository.Create(context.Background(), sample()); err != nil {
		t.Fatalf("an optional failure stopped everything: %v", err)
	}

	if !fake.Ran("start helpdesk") {
		t.Error("the work stopped early:\n" + fake.String())
	}
}

func TestARequiredStepThatFailsStopsTheWork(t *testing.T) {
	fake := host.NewFake()
	fake.Failures["limits.memory"] = errAlready

	repository := NewCLIInstanceRepository(fake, "lxc", FlavorLXD)
	if err := repository.Create(context.Background(), sample()); err == nil {
		t.Fatal("a required step failed and Create reported success")
	}

	if fake.Ran("start helpdesk") {
		t.Error("it carried on after a required step failed:\n" + fake.String())
	}
}

// LXD and Incus do not share image remotes. Getting this wrong means the very
// first container a user creates fails to launch.
func TestEachRuntimeUsesItsOwnImageRemote(t *testing.T) {
	fake := host.NewFake()

	if got := NewCLIInstanceRepository(fake, "lxc", FlavorLXD).DefaultImage(); got != "ubuntu:24.04" {
		t.Errorf("LXD default image is %q", got)
	}
	if got := NewCLIInstanceRepository(fake, "incus", FlavorIncus).DefaultImage(); got != "images:ubuntu/24.04" {
		t.Errorf("Incus default image is %q", got)
	}
}

var errAlready = &staticError{"already local"}

type staticError struct{ text string }

func (e *staticError) Error() string { return e.text }
