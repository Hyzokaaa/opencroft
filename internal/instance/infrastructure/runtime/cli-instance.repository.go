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

func (r *CLIInstanceRepository) Create(ctx context.Context, instance *entities.Instance) error {
	if _, err := r.host.Run(ctx, r.bin, "init", instance.Image, instance.Name); err != nil {
		return err
	}

	settings := [][2]string{
		{"limits.cpu", strconv.Itoa(instance.CPULimit)},
		{"limits.memory", instance.MemLimit},
	}
	for _, s := range settings {
		if _, err := r.host.Run(ctx, r.bin, "config", "set", instance.Name, s[0], s[1]); err != nil {
			return err
		}
	}

	// A DHCP lease changes when the container restarts, which silently breaks
	// the proxy pointing at it. Pin the address before the first boot.
	_, _ = r.host.Run(ctx, r.bin, "config", "device", "override", instance.Name, "eth0")
	if _, err := r.host.Run(ctx, r.bin, "config", "device", "set", instance.Name, "eth0", "ipv4.address", instance.Address); err != nil {
		return err
	}

	annotations := [][2]string{
		{"id", instance.GetId()},
		{"managed", "true"},
		{"image", instance.Image},
		{"port", strconv.Itoa(instance.Port)},
		{"created", instance.Created},
	}
	for _, a := range annotations {
		if err := r.Annotate(ctx, instance.Name, a[0], a[1]); err != nil {
			return err
		}
	}

	_, err := r.host.Run(ctx, r.bin, "start", instance.Name)
	return err
}

func (r *CLIInstanceRepository) Delete(ctx context.Context, name string) error {
	_, err := r.host.Run(ctx, r.bin, "delete", name, "--force")
	return err
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
