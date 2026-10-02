package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/Hyzokaaa/opencroft/internal/agent"
	certificateEntities "github.com/Hyzokaaa/opencroft/internal/certificate/domain/entities"
	instanceEntities "github.com/Hyzokaaa/opencroft/internal/instance/domain/entities"
	"github.com/Hyzokaaa/opencroft/internal/instance/domain/enums"
	"github.com/Hyzokaaa/opencroft/internal/instance/infrastructure/runtime"
	routeEntities "github.com/Hyzokaaa/opencroft/internal/route/domain/entities"
	routeEnums "github.com/Hyzokaaa/opencroft/internal/route/domain/enums"
	routeMemory "github.com/Hyzokaaa/opencroft/internal/route/infrastructure/memory"
	"github.com/Hyzokaaa/opencroft/internal/shared/host"
)

// A demo agent is the real agent on a host that runs nothing.
//
// Every request takes the same path it takes on a server — panel, socket,
// agent, plan — and every command is answered by rehearsalHost instead of a
// shell. What a command would change and the panel would read back (a label
// on a container, a declaration in /etc) is applied to the in-memory state,
// so a flow can be walked end to end on a laptop: create a project, move a
// container into it, add a database, connect another container to it.
//
// It is for trying the interface, not for testing croft against a system:
// nothing it says about nginx or systemd is true.

func demoAgent(ctx context.Context, socket string, pause time.Duration) {
	instances := runtime.NewMemoryInstanceRepository()
	routes := routeMemory.NewMemoryRouteRepository()
	h := newRehearsalHost(instances, pause)
	seedDemo(ctx, instances, routes, h)

	server := agent.NewServer(instances, routes, demoCertificates{},
		h, "lxd", version, "lxc")

	listener, err := agent.Listen(socket, "")
	if err != nil {
		fmt.Fprintln(os.Stderr, "[ERROR]", err)
		os.Exit(1)
	}
	defer listener.Close()

	fmt.Printf("croft agent %s — demo: nothing on this machine is touched\n", version)
	fmt.Printf("Listening on %s\n", socket)
	if err := http.Serve(listener, server.Handler()); err != nil {
		fmt.Fprintln(os.Stderr, "[ERROR]", err)
		os.Exit(1)
	}
}

// ── The host that runs nothing ───────────────────────────────────────────────

type rehearsalHost struct {
	mu        sync.Mutex
	files     map[string]string
	instances *runtime.MemoryInstanceRepository
	// pause is how long each command pretends to take, so progress can be
	// watched rather than only read.
	pause time.Duration
}

func newRehearsalHost(instances *runtime.MemoryInstanceRepository, pause time.Duration) *rehearsalHost {
	return &rehearsalHost{files: map[string]string{}, instances: instances, pause: pause}
}

func (h *rehearsalHost) Run(ctx context.Context, name string, args ...string) (host.Output, error) {
	select {
	case <-time.After(h.pause):
	case <-ctx.Done():
		return host.Output{}, ctx.Err()
	}

	switch {
	// What the panel reads back about a container is its configuration, so
	// that is what a rehearsed `lxc config set` changes.
	case (name == "lxc" || name == "incus") && len(args) >= 4 && args[0] == "config" &&
		(args[1] == "set" || args[1] == "unset") && strings.HasPrefix(args[3], "user.croft."):
		value := ""
		if args[1] == "set" && len(args) >= 5 {
			value = args[4]
		}
		return host.Output{}, h.instances.Annotate(ctx, args[2], strings.TrimPrefix(args[3], "user.croft."), value)

	case (name == "lxc" || name == "incus") && len(args) >= 5 && args[0] == "exec" &&
		args[2] == "--" && args[3] == "systemctl" && args[4] == "is-active":
		answers := make([]string, 0, len(args)-5)
		for range args[5:] {
			answers = append(answers, "active")
		}
		return host.Output{Stdout: strings.Join(answers, "\n")}, nil

	case (name == "lxc" || name == "incus") && len(args) >= 4 && args[0] == "network" && args[1] == "get":
		if args[3] == "dns.domain" {
			return host.Output{Stdout: "lxd\n"}, nil
		}
		return host.Output{}, nil

	case name == "rm" && len(args) > 0:
		h.mu.Lock()
		delete(h.files, args[len(args)-1])
		h.mu.Unlock()

	case name == "mv" && len(args) == 2:
		h.mu.Lock()
		if content, ok := h.files[args[0]]; ok {
			h.files[args[1]] = content
			delete(h.files, args[0])
		}
		h.mu.Unlock()
	}
	return host.Output{}, nil
}

func (h *rehearsalHost) ReadFile(_ context.Context, file string) ([]byte, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	content, ok := h.files[file]
	if !ok {
		return nil, os.ErrNotExist
	}
	return []byte(content), nil
}

func (h *rehearsalHost) WriteFile(_ context.Context, file string, content []byte, _ os.FileMode) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.files[file] = string(content)
	return nil
}

func (h *rehearsalHost) RemoveFile(_ context.Context, file string) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.files, file)
	return nil
}

// ListDir answers from the files written, so a directory exists once
// something is in it — which is all the agent ever asks.
func (h *rehearsalHost) ListDir(_ context.Context, dir string) ([]string, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	found := []string{}
	for file := range h.files {
		if path.Dir(file) == dir {
			found = append(found, path.Base(file))
		}
	}
	if len(found) == 0 {
		return nil, os.ErrNotExist
	}
	sort.Strings(found)
	return found, nil
}

func (h *rehearsalHost) Lookup(_ context.Context, name string) (string, bool) {
	return "/usr/bin/" + name, true
}

// ── Something to look at ─────────────────────────────────────────────────────

// seedDemo is a host a few weeks in: one project with a web and an API that
// holds its database, a blog on its own, a container nobody has grouped, and
// one croft did not create.
func seedDemo(ctx context.Context, instances *runtime.MemoryInstanceRepository,
	routes *routeMemory.MemoryRouteRepository, h *rehearsalHost) {
	today := time.Now().Format("2006-01-02")

	for _, props := range []instanceEntities.InstanceProps{
		{Name: "shop-web", Image: "images:ubuntu/24.04", Address: "10.146.38.200", Port: 80,
			CPULimit: 2, MemLimit: "2GB", Status: enums.StatusRunning, Created: today, Managed: true},
		{Name: "shop-api", Image: "images:ubuntu/24.04", Address: "10.146.38.201", Port: 3000,
			CPULimit: 4, MemLimit: "4GB", Status: enums.StatusRunning, Created: today, Managed: true},
		{Name: "blog", Image: "images:debian/12", Address: "10.146.38.202", Port: 8080,
			CPULimit: 1, MemLimit: "1GB", Status: enums.StatusRunning, Created: today, Managed: true},
		{Name: "sandbox", Image: "images:ubuntu/24.04", Address: "10.146.38.203", Port: 80,
			CPULimit: 1, MemLimit: "1GB", Status: enums.StatusStopped, Created: today, Managed: true},
		{Name: "legacy-box", Address: "10.146.38.51", CPULimit: 1, MemLimit: "1GB",
			Status: enums.StatusRunning},
	} {
		_ = instances.Create(ctx, instanceEntities.NewInstance(props))
	}

	h.files["/etc/croft/projects/shop.conf"] = "description = The storefront and its API\n"

	annotate := func(container string, pairs ...string) {
		for i := 0; i+1 < len(pairs); i += 2 {
			_ = instances.Annotate(ctx, container, pairs[i], pairs[i+1])
		}
	}
	annotate("shop-web", "project", "shop",
		"services", "storefront",
		"service.storefront.repo", "https://github.com/example/storefront.git",
		"service.storefront.branch", "main", "service.storefront.commit", "4f2a9c1",
		"service.storefront.runtime", "node", "service.storefront.build", "npm ci && npm run build",
		"service.storefront.start", "npm run start", "service.storefront.port", "80")
	annotate("shop-api", "project", "shop",
		"services", "api",
		"service.api.repo", "https://github.com/example/shop-api.git",
		"service.api.branch", "main", "service.api.commit", "b81e07d",
		"service.api.runtime", "node", "service.api.install", "npm ci",
		"service.api.start", "node dist/main.js", "service.api.port", "3000",
		"service.api.health", "/health",
		"databases", "shop",
		"database.shop.engine", "postgres", "database.shop.db", "shop",
		"database.shop.user", "shop", "database.shop.port", "5432")
	annotate("blog", "services", "blog",
		"service.blog.repo", "https://github.com/example/blog.git",
		"service.blog.branch", "main", "service.blog.commit", "0c3d5e8",
		"service.blog.runtime", "static", "service.blog.build", "npm ci && npm run build",
		"service.blog.output", "dist")

	for _, route := range []routeEntities.RouteProps{
		{Domain: "shop.example.com", Target: "10.146.38.200", Port: 80, SSL: true,
			Certificates: "/var/lib/croft/certificates/_.example.com",
			Paths:        []routeEntities.PathRoute{{Prefix: "/api/", Target: "10.146.38.201", Port: 3000, Strip: true}},
			State:        routeEnums.StateManaged},
		{Domain: "blog.example.com", Target: "10.146.38.202", Port: 8080, State: routeEnums.StateManaged},
		{Domain: "old.example.com", Target: "10.146.38.51", Port: 80, State: routeEnums.StateUnmanaged,
			File: "/etc/nginx/sites-enabled/old.example.com"},
	} {
		_ = routes.Write(ctx, routeEntities.NewRoute(route))
	}
}

// demoCertificates are the demo host's: croft's wildcard for the shop, and a
// certbot certificate about to run out, which is what the panel warns about.
type demoCertificates struct{}

func (demoCertificates) FindAll(context.Context) ([]*certificateEntities.Certificate, error) {
	now := time.Now()
	return []*certificateEntities.Certificate{
		certificateEntities.NewCertificate(certificateEntities.CertificateProps{
			Domain: "*.example.com", Names: []string{"*.example.com", "example.com"},
			Issuer: "Let's Encrypt", NotAfter: now.AddDate(0, 0, 74),
			Path: "/var/lib/croft/certificates/_.example.com/fullchain.pem", Managed: true,
		}),
		certificateEntities.NewCertificate(certificateEntities.CertificateProps{
			Domain: "old.example.com", Names: []string{"old.example.com"},
			Issuer: "Let's Encrypt", NotAfter: now.AddDate(0, 0, 9),
			Path: "/etc/letsencrypt/live/old.example.com/fullchain.pem",
		}),
	}, nil
}
