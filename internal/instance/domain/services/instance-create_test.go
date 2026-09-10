package services_test

import (
	"context"
	"errors"
	"testing"

	"github.com/Hyzokaaa/opencroft/internal/instance/domain/services"
	"github.com/Hyzokaaa/opencroft/internal/instance/infrastructure/runtime"
	"github.com/Hyzokaaa/opencroft/internal/shared/id"
)

func creator() (*services.CreateInstance, *runtime.MemoryInstanceRepository) {
	repository := runtime.NewMemoryInstanceRepository()
	return services.NewCreateInstance(id.NewFake("test-"), repository), repository
}

func TestPreparingDoesNotCreateAnything(t *testing.T) {
	create, repository := creator()

	if _, _, err := create.Prepare(context.Background(), services.CreateInstanceProps{Name: "helpdesk"}); err != nil {
		t.Fatalf("prepare: %v", err)
	}

	found, _ := repository.FindAll(context.Background())
	if len(found) != 0 {
		t.Fatalf("asking what would happen created %d containers", len(found))
	}
}

func TestCreateFillsInSensibleDefaults(t *testing.T) {
	create, _ := creator()

	instance, _, err := create.Prepare(context.Background(), services.CreateInstanceProps{Name: "helpdesk"})
	if err != nil {
		t.Fatal(err)
	}

	switch {
	case instance.Port != 80:
		t.Errorf("port defaulted to %d", instance.Port)
	case instance.CPULimit != 4:
		t.Errorf("cpu defaulted to %d", instance.CPULimit)
	case instance.MemLimit != "4GB":
		t.Errorf("memory defaulted to %q", instance.MemLimit)
	case instance.Image == "":
		t.Error("no image was chosen")
	case instance.Address == "":
		t.Error("no address was allocated")
	case !instance.Managed:
		t.Error("a container we created is not marked as ours")
	}
}

func TestNamesAreRefusedBeforeAnythingHappens(t *testing.T) {
	cases := map[string]struct {
		name string
		want error
	}{
		"empty":                {"", services.ErrNameRequired},
		"with a space":         {"two words", services.ErrNameInvalid},
		"with a separator":     {"app;id", services.ErrNameInvalid},
		"with a slash":         {"app/other", services.ErrNameInvalid},
		"starting with a dash": {"-app", services.ErrNameInvalid},
	}

	for label, test := range cases {
		t.Run(label, func(t *testing.T) {
			create, repository := creator()

			_, _, err := create.Prepare(context.Background(), services.CreateInstanceProps{Name: test.name})
			if !errors.Is(err, test.want) {
				t.Fatalf("got %v, wanted %v", err, test.want)
			}

			found, _ := repository.FindAll(context.Background())
			if len(found) != 0 {
				t.Error("something was created anyway")
			}
		})
	}
}

func TestTwoContainersCannotShareAName(t *testing.T) {
	create, _ := creator()
	ctx := context.Background()

	if _, err := create.Execute(ctx, services.CreateInstanceProps{Name: "helpdesk"}); err != nil {
		t.Fatal(err)
	}
	if _, err := create.Execute(ctx, services.CreateInstanceProps{Name: "helpdesk"}); !errors.Is(err, services.ErrAlreadyExists) {
		t.Fatalf("the second one was allowed: %v", err)
	}
}

// Two containers on one address means one of them is unreachable and the proxy
// is pointing at whichever answers first.
func TestEachContainerGetsItsOwnAddress(t *testing.T) {
	create, _ := creator()
	ctx := context.Background()

	seen := map[string]bool{}
	for _, name := range []string{"one", "two", "three"} {
		instance, err := create.Execute(ctx, services.CreateInstanceProps{Name: name})
		if err != nil {
			t.Fatal(err)
		}
		if seen[instance.Address] {
			t.Fatalf("%s was given %s, which is already taken", name, instance.Address)
		}
		seen[instance.Address] = true
	}
}
