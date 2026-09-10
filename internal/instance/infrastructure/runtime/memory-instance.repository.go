package runtime

import (
	"context"
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/Hyzokaaa/opencroft/internal/instance/domain/entities"
	"github.com/Hyzokaaa/opencroft/internal/instance/domain/enums"
	"github.com/Hyzokaaa/opencroft/internal/shared/plan"
)

// MemoryInstanceRepository implements the same interface without a runtime.
// It backs `croft serve --demo` and the test suite.
type MemoryInstanceRepository struct {
	mu        sync.RWMutex
	instances map[string]*entities.Instance
}

func NewMemoryInstanceRepository() *MemoryInstanceRepository {
	return &MemoryInstanceRepository{instances: map[string]*entities.Instance{}}
}

// NewDemoInstanceRepository is pre-populated so the interface has something to
// show on a machine with no container runtime.
func NewDemoInstanceRepository() *MemoryInstanceRepository {
	r := NewMemoryInstanceRepository()
	today := time.Now().Format("2006-01-02")

	seed := []entities.InstanceProps{
		{Id: "01JDEMO000000000000000001", Name: "helpdesk", Image: "images:ubuntu/24.04",
			Address: "10.146.38.200", Port: 3000, Domain: "soporte.example.com",
			CPULimit: 4, MemLimit: "4GB", Status: enums.StatusRunning, Created: today, Managed: true},
		{Id: "01JDEMO000000000000000002", Name: "landing", Image: "images:ubuntu/24.04",
			Address: "10.146.38.201", Port: 80, Domain: "www.example.com",
			CPULimit: 2, MemLimit: "2GB", Status: enums.StatusRunning, Created: today, Managed: true},
		{Id: "01JDEMO000000000000000003", Name: "staging", Image: "images:debian/12",
			Address: "10.146.38.202", Port: 8080, Domain: "staging.example.com",
			CPULimit: 2, MemLimit: "2GB", Status: enums.StatusStopped, Created: today, Managed: true},
		{Name: "legacy-box", Address: "10.146.38.51", CPULimit: 1, MemLimit: "1GB",
			Status: enums.StatusRunning, Managed: false},
	}

	for _, props := range seed {
		instance := entities.NewInstance(props)
		r.instances[instance.Name] = instance
	}
	return r
}

func (r *MemoryInstanceRepository) DefaultImage() string { return "images:ubuntu/24.04" }

// The demo plan says what the real driver would run, so the plan screen can be
// shown and reviewed without a container runtime present.
func (r *MemoryInstanceRepository) CreatePlan(instance *entities.Instance) plan.Plan {
	return plan.New(
		plan.Command("Create the container, stopped", "incus", "init", instance.Image, instance.Name),
		plan.Command("Limit its CPU", "incus", "config", "set", instance.Name, "limits.cpu", strconv.Itoa(instance.CPULimit)),
		plan.Command("Limit its memory", "incus", "config", "set", instance.Name, "limits.memory", instance.MemLimit),
		plan.Command("Pin the address so a restart cannot move it", "incus", "config", "device", "set", instance.Name, "eth0", "ipv4.address", instance.Address),
		plan.Command("Record it as managed", "incus", "config", "set", instance.Name, "user.croft.managed", "true"),
		plan.Command("Start it", "incus", "start", instance.Name),
	)
}

func (r *MemoryInstanceRepository) DeletePlan(name string) plan.Plan {
	return plan.New(plan.Command("Delete the container and its disk", "incus", "delete", name, "--force"))
}

func (r *MemoryInstanceRepository) FindAll(_ context.Context) ([]*entities.Instance, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	all := make([]*entities.Instance, 0, len(r.instances))
	for _, i := range r.instances {
		all = append(all, i)
	}
	return all, nil
}

func (r *MemoryInstanceRepository) FindByName(_ context.Context, name string) (*entities.Instance, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if found, ok := r.instances[name]; ok {
		return found, nil
	}
	return nil, nil
}

func (r *MemoryInstanceRepository) Create(_ context.Context, instance *entities.Instance) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if instance.Created == "" {
		instance.Created = time.Now().Format("2006-01-02")
	}
	r.instances[instance.Name] = instance
	return nil
}

func (r *MemoryInstanceRepository) Delete(_ context.Context, name string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	delete(r.instances, name)
	return nil
}

func (r *MemoryInstanceRepository) Start(_ context.Context, name string) error {
	return r.setStatus(name, enums.StatusRunning)
}

func (r *MemoryInstanceRepository) Stop(_ context.Context, name string) error {
	return r.setStatus(name, enums.StatusStopped)
}

func (r *MemoryInstanceRepository) setStatus(name string, status enums.InstanceStatus) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	found, ok := r.instances[name]
	if !ok {
		return fmt.Errorf("instance %q not found", name)
	}
	found.Status = status
	return nil
}

func (r *MemoryInstanceRepository) Annotate(_ context.Context, name, key, value string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	found, ok := r.instances[name]
	if !ok {
		return fmt.Errorf("instance %q not found", name)
	}
	switch key {
	case "domain":
		found.Domain = value
	case "managed":
		found.Managed = value == "true"
	}
	return nil
}

func (r *MemoryInstanceRepository) AllocateAddress(_ context.Context) (string, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	used := map[string]bool{}
	for _, i := range r.instances {
		used[i.Address] = true
	}

	for n := staticFirst; n <= staticLast; n++ {
		candidate := fmt.Sprintf("10.146.38.%d", n)
		if !used[candidate] {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("no free address in the static range")
}

func (r *MemoryInstanceRepository) StartPlan(name string) plan.Plan {
	return plan.New(plan.Command("Start the container", "incus", "start", name))
}

func (r *MemoryInstanceRepository) StopPlan(name string) plan.Plan {
	return plan.New(plan.Command("Stop the container", "incus", "stop", name))
}
