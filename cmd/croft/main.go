// Command croft is both the CLI and the daemon. One binary, one code path:
// every HTTP handler builds the same Command the CLI does.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"text/tabwriter"

	instanceServices "github.com/Hyzokaaa/opencroft/internal/instance/domain/services"
	"github.com/Hyzokaaa/opencroft/internal/instance/infrastructure/runtime"
	overviewQueries "github.com/Hyzokaaa/opencroft/internal/overview/application/queries"
	routeRepositories "github.com/Hyzokaaa/opencroft/internal/route/domain/repositories"
	routeServices "github.com/Hyzokaaa/opencroft/internal/route/domain/services"
	routeMemory "github.com/Hyzokaaa/opencroft/internal/route/infrastructure/memory"
	"github.com/Hyzokaaa/opencroft/internal/route/infrastructure/nginx"
	"github.com/Hyzokaaa/opencroft/internal/server"
	"github.com/Hyzokaaa/opencroft/internal/shared/host"
	"github.com/Hyzokaaa/opencroft/internal/shared/id"

	instanceRepositories "github.com/Hyzokaaa/opencroft/internal/instance/domain/repositories"
)

const version = "0.1.0-dev"

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(1)
	}

	ctx := context.Background()
	switch os.Args[1] {
	case "serve":
		serve(ctx, os.Args[2:])
	case "list":
		list(ctx, os.Args[2:])
	case "create":
		create(ctx, os.Args[2:])
	case "destroy":
		destroy(ctx, os.Args[2:])
	case "version", "--version", "-v":
		fmt.Println("croft " + version)
	default:
		usage()
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprint(os.Stderr, `croft — a PaaS without Docker, that doesn't hijack your server.

Usage:
  croft serve [--addr :8080] [--demo]   run the daemon and the web interface
  croft list [--json]                   show every container and how it is routed
  croft create <name> [flags]           create a container
  croft destroy <name>                  remove a container
  croft version

Every command works without a terminal: pass flags and read --json.
`)
}

// deps wires concrete implementations. The CLI and the HTTP API share it, so
// there is only ever one path into the domain.
type deps struct {
	instances instanceRepositories.InstanceRepository
	routes    routeRepositories.RouteRepository
	runtime   string
	demo      bool
}

func wire(ctx context.Context, demo bool, nginxDir string) deps {
	if demo {
		return deps{
			instances: runtime.NewDemoInstanceRepository(),
			routes:    routeMemory.NewDemoRouteRepository(),
			runtime:   "demo",
			demo:      true,
		}
	}

	h := host.NewLocal()
	flavor, bin := runtime.Detect(ctx, h)
	if flavor == runtime.FlavorNone {
		fmt.Fprintln(os.Stderr, "[ERROR] No container runtime found. Install Incus or LXD, or run with --demo.")
		os.Exit(1)
	}

	return deps{
		instances: runtime.NewCLIInstanceRepository(h, bin, flavor),
		routes:    nginx.NewNginxRouteRepository(h, nginxDir),
		runtime:   string(flavor),
	}
}

func serve(ctx context.Context, args []string) {
	fs := flag.NewFlagSet("serve", flag.ExitOnError)
	addr := fs.String("addr", ":8080", "address to listen on")
	demo := fs.Bool("demo", false, "serve sample data, with no container runtime")
	nginxDir := fs.String("nginx-dir", "/etc/nginx/croft.d", "directory holding generated vhosts")
	readOnly := fs.Bool("read-only", false, "refuse every write through the API")
	_ = fs.Parse(args)

	d := wire(ctx, *demo, *nginxDir)

	handler := server.Handler(server.Deps{
		Overview: overviewQueries.NewOverviewQuery(
			instanceServices.NewListInstances(d.instances),
			routeServices.NewListRoutes(d.routes),
			d.runtime,
			d.demo,
		),
		CreateInstance:  instanceServices.NewCreateInstance(id.NewULIDGenerator(), d.instances),
		DestroyInstance: instanceServices.NewDestroyInstance(d.instances),
		ReadOnly:        *readOnly,
	})

	fmt.Printf("croft %s — runtime: %s\n", version, d.runtime)
	if d.demo {
		fmt.Println("Demo mode: sample data, nothing on this machine is touched.")
	}
	fmt.Printf("Listening on http://localhost%s\n", *addr)

	if err := http.ListenAndServe(*addr, handler); err != nil {
		fmt.Fprintln(os.Stderr, "[ERROR]", err)
		os.Exit(1)
	}
}

func list(ctx context.Context, args []string) {
	fs := flag.NewFlagSet("list", flag.ExitOnError)
	asJSON := fs.Bool("json", false, "print machine-readable output")
	demo := fs.Bool("demo", false, "use sample data")
	nginxDir := fs.String("nginx-dir", "/etc/nginx/croft.d", "directory holding generated vhosts")
	_ = fs.Parse(args)

	d := wire(ctx, *demo, *nginxDir)

	query := overviewQueries.NewOverviewQuery(
		instanceServices.NewListInstances(d.instances),
		routeServices.NewListRoutes(d.routes),
		d.runtime,
		d.demo,
	)

	response, err := query.Execute(ctx)
	if err != nil {
		fmt.Fprintln(os.Stderr, "[ERROR]", err)
		os.Exit(1)
	}

	if *asJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(response)
		return
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "\nNAME\tSTATUS\tADDRESS\tDOMAIN\tMANAGED")
	for _, i := range response.Instances {
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
			i.Name, i.Status, dash(i.Address), dash(i.Domain), yesNo(i.Managed))
	}
	_ = w.Flush()

	if len(response.Findings) > 0 {
		fmt.Println("\nFindings:")
		for _, f := range response.Findings {
			fmt.Printf("  [%s] %s — %s\n", f.Severity, f.Subject, f.Message)
		}
	}
	fmt.Println()
}

func create(ctx context.Context, args []string) {
	fs := flag.NewFlagSet("create", flag.ExitOnError)
	image := fs.String("image", "", "image to launch (defaults to the runtime's Ubuntu LTS)")
	port := fs.Int("port", 80, "port the application listens on inside the container")
	cpu := fs.Int("cpu", 4, "CPU limit")
	mem := fs.String("memory", "4GB", "memory limit")
	demo := fs.Bool("demo", false, "use sample data")
	nginxDir := fs.String("nginx-dir", "/etc/nginx/croft.d", "directory holding generated vhosts")
	_ = fs.Parse(args)

	if fs.NArg() < 1 {
		fmt.Fprintln(os.Stderr, "[ERROR] A name is required: croft create <name>")
		os.Exit(1)
	}

	d := wire(ctx, *demo, *nginxDir)
	service := instanceServices.NewCreateInstance(id.NewULIDGenerator(), d.instances)

	instance, err := service.Execute(ctx, instanceServices.CreateInstanceProps{
		Name: fs.Arg(0), Image: *image, Port: *port, CPULimit: *cpu, MemLimit: *mem,
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "[ERROR]", err)
		os.Exit(1)
	}
	fmt.Printf("[OK] %s created at %s\n", instance.Name, instance.Address)
}

func destroy(ctx context.Context, args []string) {
	fs := flag.NewFlagSet("destroy", flag.ExitOnError)
	demo := fs.Bool("demo", false, "use sample data")
	nginxDir := fs.String("nginx-dir", "/etc/nginx/croft.d", "directory holding generated vhosts")
	_ = fs.Parse(args)

	if fs.NArg() < 1 {
		fmt.Fprintln(os.Stderr, "[ERROR] A name is required: croft destroy <name>")
		os.Exit(1)
	}

	d := wire(ctx, *demo, *nginxDir)
	if err := instanceServices.NewDestroyInstance(d.instances).Execute(ctx, fs.Arg(0)); err != nil {
		fmt.Fprintln(os.Stderr, "[ERROR]", err)
		os.Exit(1)
	}
	fmt.Printf("[OK] %s destroyed\n", fs.Arg(0))
}

func dash(v string) string {
	if v == "" {
		return "—"
	}
	return v
}

func yesNo(v bool) string {
	if v {
		return "yes"
	}
	return "no"
}

