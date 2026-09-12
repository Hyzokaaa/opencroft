package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/Hyzokaaa/opencroft/internal/instance/domain/entities"
	"github.com/Hyzokaaa/opencroft/internal/instance/domain/enums"
	"github.com/Hyzokaaa/opencroft/internal/shared/host"
	"github.com/Hyzokaaa/opencroft/internal/shared/plan"
)

// CLIInstanceRepository drives LXD or Incus through their command line.
type CLIInstanceRepository struct {
	host   host.Host
	bin    string
	flavor Flavor
}

func NewCLIInstanceRepository(h host.Host, bin string, flavor Flavor) *CLIInstanceRepository {
	return &CLIInstanceRepository{host: h, bin: bin, flavor: flavor}
}

func (r *CLIInstanceRepository) DefaultImage() string {
	if r.flavor == FlavorIncus {
		return "images:ubuntu/24.04"
	}
	return "ubuntu:24.04"
}

// ── Reading ───────────────────────────────────────────────────────────────────

type cliInstance struct {
	Name   string            `json:"name"`
	Status string            `json:"status"`
	Config map[string]string `json:"config"`
	State  *struct {
		Network map[string]struct {
			Addresses []struct {
				Family  string `json:"family"`
				Address string `json:"address"`
			} `json:"addresses"`
		} `json:"network"`
	} `json:"state"`
}

func (r *CLIInstanceRepository) FindAll(ctx context.Context) ([]*entities.Instance, error) {
	out, err := r.host.Run(ctx, r.bin, "list", "--format", "json")
	if err != nil {
		return nil, err
	}

	var raw []cliInstance
	if err := json.Unmarshal([]byte(out.Stdout), &raw); err != nil {
		return nil, fmt.Errorf("parsing the runtime listing: %w", err)
	}

	instances := make([]*entities.Instance, 0, len(raw))
	for _, item := range raw {
		instances = append(instances, toEntity(item))
	}
	return instances, nil
}

func (r *CLIInstanceRepository) FindByName(ctx context.Context, name string) (*entities.Instance, error) {
	all, err := r.FindAll(ctx)
	if err != nil {
		return nil, err
	}
	for _, i := range all {
		if i.Name == name {
			return i, nil
		}
	}
	return nil, nil
}

func toEntity(item cliInstance) *entities.Instance {
	annotation := func(key string) string { return item.Config[AnnotationPrefix+"."+key] }

	port, _ := strconv.Atoi(annotation("port"))
	cpu, _ := strconv.Atoi(item.Config["limits.cpu"])

	return entities.NewInstance(entities.InstanceProps{
		Id:       annotation("id"),
		Name:     item.Name,
		Image:    annotation("image"),
		Address:  firstIPv4(item),
		Port:     port,
		Domain:   annotation("domain"),
		CPULimit: cpu,
		MemLimit: item.Config["limits.memory"],
		Status:   enums.ParseStatus(item.Status),
		Created:  annotation("created"),
		Managed:  annotation("managed") == "true",
	})
}

func firstIPv4(item cliInstance) string {
	if item.State == nil {
		return ""
	}
	for _, iface := range item.State.Network {
		for _, addr := range iface.Addresses {
			if addr.Family == "inet" && !strings.HasPrefix(addr.Address, "127.") {
				return addr.Address
			}
		}
	}
	return ""
}

// ── Writing ───────────────────────────────────────────────────────────────────

// CreatePlan is the single description of what creating a container does.
// Create walks exactly this list, so what you were shown is what runs.
func (r *CLIInstanceRepository) CreatePlan(instance *entities.Instance) plan.Plan {
	steps := []plan.Step{
		plan.Command("Create the container, stopped",
			r.bin, "init", instance.Image, instance.Name),
		plan.Command("Limit its CPU",
			r.bin, "config", "set", instance.Name, "limits.cpu", strconv.Itoa(instance.CPULimit)),
		plan.Command("Limit its memory",
			r.bin, "config", "set", instance.Name, "limits.memory", instance.MemLimit),

		// A DHCP lease changes when a container restarts, which silently
		// breaks the proxy pointing at it. Pin the address before first boot.
		plan.Optional("Take ownership of the network device inherited from the profile",
			r.bin, "config", "device", "override", instance.Name, "eth0"),
		plan.Command("Pin the address so a restart cannot move it",
			r.bin, "config", "device", "set", instance.Name, "eth0", "ipv4.address", instance.Address),
	}

	// The desired state lives on the container itself, so losing our database
	// costs nothing and migrating the container carries it along.
	for _, a := range [][2]string{
		{"id", instance.GetId()},
		{"managed", "true"},
		{"image", instance.Image},
		{"port", strconv.Itoa(instance.Port)},
		{"created", instance.Created},
	} {
		steps = append(steps, plan.Command(
			"Record "+a[0]+" on the container",
			r.bin, "config", "set", instance.Name, AnnotationPrefix+"."+a[0], a[1]))
	}

	return plan.New(append(steps, plan.Command("Start it", r.bin, "start", instance.Name))...)
}

func (r *CLIInstanceRepository) DeletePlan(name string) plan.Plan {
	return plan.New(plan.Command("Delete the container and its disk",
		r.bin, "delete", name, "--force"))
}

func (r *CLIInstanceRepository) Create(ctx context.Context, instance *entities.Instance) error {
	return r.walk(ctx, r.CreatePlan(instance))
}

func (r *CLIInstanceRepository) Delete(ctx context.Context, name string) error {
	return r.walk(ctx, r.DeletePlan(name))
}

func (r *CLIInstanceRepository) walk(ctx context.Context, p plan.Plan) error {
	for _, step := range p.Steps {
		if _, err := r.host.Run(ctx, step.Argv[0], step.Argv[1:]...); err != nil {
			if step.Optional {
				continue
			}
			return err
		}
	}
	return nil
}

func (r *CLIInstanceRepository) Start(ctx context.Context, name string) error {
	_, err := r.host.Run(ctx, r.bin, "start", name)
	return err
}

func (r *CLIInstanceRepository) Stop(ctx context.Context, name string) error {
	_, err := r.host.Run(ctx, r.bin, "stop", name)
	return err
}

func (r *CLIInstanceRepository) Annotate(ctx context.Context, name, key, value string) error {
	_, err := r.host.Run(ctx, r.bin, "config", "set", name, AnnotationPrefix+"."+key, value)
	return err
}

// ── Addressing ────────────────────────────────────────────────────────────────

func (r *CLIInstanceRepository) AllocateAddress(ctx context.Context) (string, error) {
	prefix, err := r.bridgePrefix(ctx)
	if err != nil {
		return "", err
	}

	used := map[string]bool{}
	all, err := r.FindAll(ctx)
	if err != nil {
		return "", err
	}
	for _, i := range all {
		used[i.Address] = true
	}

	for n := staticFirst; n <= staticLast; n++ {
		candidate := fmt.Sprintf("%s.%d", prefix, n)
		if !used[candidate] {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("no free address between %s.%d and %s.%d", prefix, staticFirst, prefix, staticLast)
}

func (r *CLIInstanceRepository) bridgePrefix(ctx context.Context) (string, error) {
	bridge, err := r.host.Run(ctx, r.bin, "profile", "device", "get", "default", "eth0", "network")
	name := strings.TrimSpace(bridge.Stdout)
	if err != nil || name == "" {
		name = "lxdbr0"
		if r.flavor == FlavorIncus {
			name = "incusbr0"
		}
	}

	out, err := r.host.Run(ctx, r.bin, "network", "get", name, "ipv4.address")
	if err != nil {
		return "", fmt.Errorf("reading the bridge subnet: %w", err)
	}

	cidr := strings.TrimSpace(out.Stdout)
	cidr, _, _ = strings.Cut(cidr, "/")
	octets := strings.Split(cidr, ".")
	if len(octets) != 4 {
		return "", fmt.Errorf("unexpected bridge address %q", out.Stdout)
	}
	return strings.Join(octets[:3], "."), nil
}

// CreateWithProgress walks the plan announcing each step before running it.
// Create is the same thing without an audience.
func (r *CLIInstanceRepository) CreateWithProgress(ctx context.Context, instance *entities.Instance, report func(int, string)) error {
	return r.walkReporting(ctx, r.CreatePlan(instance), report)
}

func (r *CLIInstanceRepository) DeleteWithProgress(ctx context.Context, name string, report func(int, string)) error {
	return r.walkReporting(ctx, r.DeletePlan(name), report)
}

func (r *CLIInstanceRepository) walkReporting(ctx context.Context, p plan.Plan, report func(int, string)) error {
	for i, step := range p.Steps {
		if report != nil {
			report(i+1, step.Describe)
		}
		if _, err := r.host.Run(ctx, step.Argv[0], step.Argv[1:]...); err != nil {
			if step.Optional {
				continue
			}
			return err
		}
	}
	return nil
}

func (r *CLIInstanceRepository) StartPlan(name string) plan.Plan {
	return plan.New(plan.Command("Start the container", r.bin, "start", name))
}

// Stopping is graceful: the runtime asks the container's init to shut down.
// Anything running inside gets to close its files.
func (r *CLIInstanceRepository) StopPlan(name string) plan.Plan {
	return plan.New(plan.Command("Stop the container", r.bin, "stop", name))
}

// Annotations reads the whole configuration of one container in a single call.
//
// The listing already carries every key; asking for them one at a time was
// starting a process per annotation, which is what made opening a container
// feel slow for no reason anybody could see.
func (r *CLIInstanceRepository) Annotations(ctx context.Context, name string) (map[string]string, error) {
	out, err := r.host.Run(ctx, r.bin, "list", name, "--format", "json")
	if err != nil {
		return nil, err
	}

	var raw []cliInstance
	if err := json.Unmarshal([]byte(out.Stdout), &raw); err != nil {
		return nil, fmt.Errorf("parsing the runtime listing: %w", err)
	}

	// The runtime matches on a prefix, so asking for "web" can also answer
	// about "web-staging".
	for _, item := range raw {
		if item.Name == name {
			return item.Config, nil
		}
	}
	return map[string]string{}, nil
}
