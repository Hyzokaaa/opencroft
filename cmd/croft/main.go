// Command croft is both the CLI and the daemon. One binary, one code path:
// every HTTP handler builds the same Command the CLI does.
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"text/tabwriter"

	"github.com/Hyzokaaa/opencroft/internal/agent"
	authServices "github.com/Hyzokaaa/opencroft/internal/auth/domain/services"
	"github.com/Hyzokaaa/opencroft/internal/auth/infrastructure/crypto"
	authHttp "github.com/Hyzokaaa/opencroft/internal/auth/infrastructure/http"
	"github.com/Hyzokaaa/opencroft/internal/auth/infrastructure/sqlite"
	certificateRepositories "github.com/Hyzokaaa/opencroft/internal/certificate/domain/repositories"
	certificateServices "github.com/Hyzokaaa/opencroft/internal/certificate/domain/services"
	pemCertificates "github.com/Hyzokaaa/opencroft/internal/certificate/infrastructure/pem"
	"golang.org/x/term"

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
	"github.com/Hyzokaaa/opencroft/internal/shared/job"
	"github.com/Hyzokaaa/opencroft/internal/update"

	instanceRepositories "github.com/Hyzokaaa/opencroft/internal/instance/domain/repositories"
)

// version is stamped at build time with -ldflags "-X main.version=…".
// A panel that misreports its own version is a problem the day something
// needs debugging.
var version = "dev"

const defaultDBPath = "/var/lib/croft/croft.db"

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
	case "user":
		user(ctx, os.Args[2:])
	case "update":
		updateCommand(os.Args[2:])
	case "agent":
		agentCommand(ctx, os.Args[2:])
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
  croft update [--check]                install the latest release
  croft agent                           the privileged half, over a unix socket
  croft version

  croft user add <name>                 create a user who can sign in
  croft user list
  croft user passwd <name>              change a password
  croft user bootstrap                  create admin with a random password, once
  croft user rm <name>

Every command works without a terminal: pass flags and read --json.
`)
}

// deps wires concrete implementations. The CLI and the HTTP API share it, so
// there is only ever one path into the domain.
type deps struct {
	instances    instanceRepositories.InstanceRepository
	routes       routeRepositories.RouteRepository
	certificates certificateRepositories.CertificateRepository
	runtime      string
	version      string
	demo         bool
}

func wire(ctx context.Context, demo bool, nginxDir, socket string) deps {
	if demo {
		return deps{
			instances:    runtime.NewDemoInstanceRepository(),
			routes:       routeMemory.NewDemoRouteRepository(),
			certificates: pemCertificates.NewDemoCertificateRepository(),
			runtime:      "demo",
			version:      version,
			demo:         true,
		}
	}

	// Prefer the agent: it is the half that holds the privileges, and this
	// process then needs none of them.
	if socket != "" {
		if _, err := os.Stat(socket); err == nil {
			client, err := agent.Dial(socket, version)
			if err != nil {
				fmt.Fprintln(os.Stderr, "[ERROR]", err)
				os.Exit(1)
			}
			return deps{
				instances:    client,
				routes:       client.Routing(),
				certificates: client.Certificates(),
				runtime:      client.Flavor(),
				version:      version,
			}
		}
	}

	h := host.NewLocal()
	flavor, bin := runtime.Detect(ctx, h)
	if flavor == runtime.FlavorNone {
		fmt.Fprintln(os.Stderr, "[ERROR] No container runtime found. Install Incus or LXD, or run with --demo.")
		os.Exit(1)
	}

	if os.Geteuid() == 0 {
		fmt.Fprintln(os.Stderr, "[WARN] No agent on "+socket+", so this process talks to the runtime itself,")
		fmt.Fprintln(os.Stderr, "       as root. Start croft-agent to keep the half that serves HTTP unprivileged.")
	}

	return deps{
		instances:    runtime.NewCLIInstanceRepository(h, bin, flavor),
		routes:       nginx.NewNginxRouteRepository(h, nginxDir),
		certificates: pemCertificates.NewPEMCertificateRepository(h),
		runtime:      string(flavor),
		version:      version,
	}
}

func serve(ctx context.Context, args []string) {
	fs := flag.NewFlagSet("serve", flag.ExitOnError)
	addr := fs.String("addr", ":8080", "address to listen on")
	demo := fs.Bool("demo", false, "serve sample data, with no container runtime")
	nginxDir := fs.String("nginx-dir", "/etc/nginx/croft.d", "directory holding generated vhosts")
	readOnly := fs.Bool("read-only", false, "refuse every write through the API")
	dbPath := fs.String("db", defaultDBPath, "where users and sessions are kept")
	agentSocket := fs.String("agent", agent.SocketPath, "socket of the privileged agent")
	_ = fs.Parse(reorder(fs, args))

	d := wire(ctx, *demo, *nginxDir, *agentSocket)
	auth := wireAuth(*dbPath)
	jobs := job.NewRunner(id.NewULIDGenerator().Create)

	handler := server.Handler(server.Deps{
		Overview: overviewQueries.NewOverviewQuery(
			instanceServices.NewListInstances(d.instances),
			routeServices.NewListRoutes(d.routes),
			certificateServices.NewListCertificates(d.certificates),
			d.runtime,
			d.version,
			d.demo,
		),
		CreateInstance:  instanceServices.NewCreateInstance(id.NewULIDGenerator(), d.instances),
		DestroyInstance: instanceServices.NewDestroyInstance(d.instances),
		ReadOnly:        *readOnly,
		Auth:            auth.handler,
		Instances:       d.instances,
		Routes:          d.routes,
		AddRoute:        routeServices.NewAddRoute(d.routes, d.instances),
		Host:            host.NewLocal(),
		Jobs:            jobs,
		Simulated:       d.demo,
	})

	total, err := auth.users.Count(ctx)
	if err != nil {
		fmt.Fprintln(os.Stderr, "[ERROR]", err)
		os.Exit(1)
	}

	fmt.Printf("croft %s — runtime: %s\n", version, d.runtime)
	if d.demo {
		fmt.Println("Demo mode: sample data, nothing on this machine is touched.")
	}
	if total == 0 {
		fmt.Println("No users yet. Create the first one with:  croft user add <name>")
	}
	fmt.Printf("Listening on %s\n", browsableURL(*addr))

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
	agentSocket := fs.String("agent", agent.SocketPath, "socket of the privileged agent")
	_ = fs.Parse(reorder(fs, args))

	d := wire(ctx, *demo, *nginxDir, *agentSocket)

	query := overviewQueries.NewOverviewQuery(
		instanceServices.NewListInstances(d.instances),
		routeServices.NewListRoutes(d.routes),
		certificateServices.NewListCertificates(d.certificates),
		d.runtime,
		d.version,
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
	agentSocket := fs.String("agent", agent.SocketPath, "socket of the privileged agent")
	_ = fs.Parse(reorder(fs, args))

	if fs.NArg() < 1 {
		fmt.Fprintln(os.Stderr, "[ERROR] A name is required: croft create <name>")
		os.Exit(1)
	}

	d := wire(ctx, *demo, *nginxDir, *agentSocket)
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
	agentSocket := fs.String("agent", agent.SocketPath, "socket of the privileged agent")
	_ = fs.Parse(reorder(fs, args))

	if fs.NArg() < 1 {
		fmt.Fprintln(os.Stderr, "[ERROR] A name is required: croft destroy <name>")
		os.Exit(1)
	}

	d := wire(ctx, *demo, *nginxDir, *agentSocket)
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

// ── Authentication ────────────────────────────────────────────────────────────

type authDeps struct {
	handler *authHttp.Handler
	users   *sqlite.SQLiteUserRepository
	create  *authServices.CreateUser
	store   *sqlite.Store
}

func wireAuth(dbPath string) authDeps {
	store, err := sqlite.Open(dbPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "[ERROR] Opening", dbPath+":", err)
		os.Exit(1)
	}

	users := sqlite.NewSQLiteUserRepository(store)
	sessions := sqlite.NewSQLiteSessionRepository(store)
	hasher := crypto.NewBcryptHasher()
	tokens := crypto.NewRandomTokens()

	return authDeps{
		handler: authHttp.NewHandler(
			users,
			authServices.NewOpenSession(users, sessions, hasher, tokens),
			authServices.NewVerifySession(sessions, tokens),
			authServices.NewCloseSession(sessions, tokens),
		),
		users:  users,
		create: authServices.NewCreateUser(id.NewULIDGenerator(), users, hasher),
		store:  store,
	}
}

func user(ctx context.Context, args []string) {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "Usage: croft user add|list|passwd|rm <name>")
		os.Exit(1)
	}

	fs := flag.NewFlagSet("user", flag.ExitOnError)
	dbPath := fs.String("db", defaultDBPath, "where users and sessions are kept")
	password := fs.String("password", "", "read the password from a flag instead of prompting (avoid: it lands in your shell history)")
	_ = fs.Parse(reorder(fs, args[1:]))

	auth := wireAuth(*dbPath)
	defer auth.store.Close()

	switch args[0] {
	case "add":
		if fs.NArg() < 1 {
			fmt.Fprintln(os.Stderr, "[ERROR] A name is required: croft user add <name>")
			os.Exit(1)
		}

		// Check the name before asking for a password twice.
		if taken, _ := auth.users.FindByUsername(ctx, fs.Arg(0)); taken != nil {
			fmt.Fprintln(os.Stderr, "[ERROR]", authServices.ErrUsernameTaken)
			os.Exit(1)
		}

		secret := *password
		if secret == "" {
			secret = askForPassword()
		}

		created, err := auth.create.Execute(ctx, authServices.CreateUserProps{
			Username: fs.Arg(0),
			Password: secret,
		})
		if err != nil {
			fmt.Fprintln(os.Stderr, "[ERROR]", err)
			os.Exit(1)
		}
		fmt.Printf("[OK] %s can now sign in\n", created.Username)

	case "list":
		found, err := auth.users.FindAll(ctx)
		if err != nil {
			fmt.Fprintln(os.Stderr, "[ERROR]", err)
			os.Exit(1)
		}
		if len(found) == 0 {
			fmt.Println("No users yet. Create one with: croft user add <name>")
			return
		}
		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "\nUSERNAME\tCREATED")
		for _, u := range found {
			fmt.Fprintf(w, "%s\t%s\n", u.Username, u.Created)
		}
		_ = w.Flush()
		fmt.Println()

	case "bootstrap":
		// For unattended installs. Creates nothing if somebody already can
		// sign in, so re-running the installer never resets access.
		total, err := auth.users.Count(ctx)
		if err != nil {
			fmt.Fprintln(os.Stderr, "[ERROR]", err)
			os.Exit(1)
		}
		if total > 0 {
			fmt.Println("[OK] Users already exist; nothing to do.")
			return
		}

		generated, err := authServices.GeneratePassword()
		if err != nil {
			fmt.Fprintln(os.Stderr, "[ERROR]", err)
			os.Exit(1)
		}

		if _, err := auth.create.Execute(ctx, authServices.CreateUserProps{
			Username: "admin",
			Password: generated,
		}); err != nil {
			fmt.Fprintln(os.Stderr, "[ERROR]", err)
			os.Exit(1)
		}

		fmt.Println()
		fmt.Println("  Sign in as:  admin")
		fmt.Println("  Password:    " + generated)
		fmt.Println()
		fmt.Println("  This is shown once and is not stored anywhere in readable form.")
		fmt.Println("  Change it with:  croft user passwd admin")
		fmt.Println()

	case "passwd":
		if fs.NArg() < 1 {
			fmt.Fprintln(os.Stderr, "[ERROR] A name is required: croft user passwd <name>")
			os.Exit(1)
		}

		secret := *password
		if secret == "" {
			secret = askForPassword()
		}

		change := authServices.NewChangePassword(auth.users, crypto.NewBcryptHasher())
		if err := change.Execute(ctx, fs.Arg(0), secret); err != nil {
			fmt.Fprintln(os.Stderr, "[ERROR]", err)
			os.Exit(1)
		}
		fmt.Printf("[OK] password changed for %s\n", fs.Arg(0))

	case "rm":
		if fs.NArg() < 1 {
			fmt.Fprintln(os.Stderr, "[ERROR] A name is required: croft user rm <name>")
			os.Exit(1)
		}
		if err := auth.users.Delete(ctx, fs.Arg(0)); err != nil {
			fmt.Fprintln(os.Stderr, "[ERROR]", err)
			os.Exit(1)
		}
		fmt.Printf("[OK] %s removed\n", fs.Arg(0))

	default:
		fmt.Fprintln(os.Stderr, "Usage: croft user add|list|passwd|rm <name>")
		os.Exit(1)
	}
}

// askForPassword reads without echoing when there is a terminal, and falls
// back to a plain read so the command still works from a script.
func askForPassword() string {
	if term.IsTerminal(int(syscall.Stdin)) {
		fmt.Print("  Password: ")
		first, err := term.ReadPassword(int(syscall.Stdin))
		fmt.Println()
		if err != nil {
			fmt.Fprintln(os.Stderr, "[ERROR]", err)
			os.Exit(1)
		}

		fmt.Print("  Again:    ")
		second, _ := term.ReadPassword(int(syscall.Stdin))
		fmt.Println()

		if string(first) != string(second) {
			fmt.Fprintln(os.Stderr, "[ERROR] The passwords do not match.")
			os.Exit(1)
		}
		return string(first)
	}

	reader := bufio.NewReader(os.Stdin)
	line, _ := reader.ReadString('\n')
	return strings.TrimSpace(line)
}

// reorder moves flags ahead of positional arguments.
//
// Go's flag package stops parsing at the first non-flag argument, so
// `croft create app1 --port 3000` would silently ignore the port. Nobody
// writes commands in the order the parser wants, so the parser adapts.
func reorder(fs *flag.FlagSet, args []string) []string {
	flags := []string{}
	positional := []string{}

	for i := 0; i < len(args); i++ {
		arg := args[i]

		if arg == "--" {
			positional = append(positional, args[i+1:]...)
			break
		}

		if !strings.HasPrefix(arg, "-") || arg == "-" {
			positional = append(positional, arg)
			continue
		}

		flags = append(flags, arg)
		if strings.Contains(arg, "=") {
			continue
		}

		// A boolean flag takes no value; anything else consumes the next token.
		name := strings.TrimLeft(arg, "-")
		if f := fs.Lookup(name); f != nil && !isBoolFlag(f) && i+1 < len(args) {
			i++
			flags = append(flags, args[i])
		}
	}

	return append(flags, positional...)
}

func isBoolFlag(f *flag.Flag) bool {
	boolean, ok := f.Value.(interface{ IsBoolFlag() bool })
	return ok && boolean.IsBoolFlag()
}

// ── Updating ──────────────────────────────────────────────────────────────────

func updateCommand(args []string) {
	fs := flag.NewFlagSet("update", flag.ExitOnError)
	checkOnly := fs.Bool("check", false, "report what is available and exit")
	restart := fs.Bool("restart", true, "restart the croft service afterwards")
	_ = fs.Parse(reorder(fs, args))

	release, err := update.Latest()
	if err != nil {
		fmt.Fprintln(os.Stderr, "[ERROR]", err)
		os.Exit(1)
	}

	if !update.Newer(version, release) {
		fmt.Printf("croft %s is already the latest release.\n", version)
		return
	}

	fmt.Printf("  Running:   %s\n", version)
	fmt.Printf("  Available: %s\n", release.Tag)

	if *checkOnly {
		fmt.Println("\n  Update with:  sudo croft update")
		return
	}

	path, err := os.Executable()
	if err != nil {
		fmt.Fprintln(os.Stderr, "[ERROR]", err)
		os.Exit(1)
	}
	// Follow a symlink so the real file is replaced, not the link.
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		path = resolved
	}

	fmt.Printf("\n── Replacing %s\n", path)
	if err := update.Apply(release.Tag, path); err != nil {
		fmt.Fprintln(os.Stderr, "[ERROR]", err)
		if os.IsPermission(err) {
			fmt.Fprintln(os.Stderr, "        Updating the binary needs root. Try: sudo croft update")
		}
		os.Exit(1)
	}
	fmt.Printf("[OK] croft %s installed\n", release.Tag)

	if *restart {
		if _, err := exec.LookPath("systemctl"); err == nil {
			// The agent first. Both halves are the same binary, and a pair
			// left half-updated speaks two different protocols to each other.
			for _, unit := range []string{"croft-agent", "croft"} {
				out, err := exec.Command("systemctl", "restart", unit).CombinedOutput()
				switch {
				case err == nil:
					fmt.Printf("[OK] %s.service restarted\n", unit)
				case strings.Contains(string(out), "not found"),
					strings.Contains(string(out), "could not be found"):
					// Not installed on this host; nothing to restart.
				default:
					fmt.Fprintf(os.Stderr, "[WARN] Could not restart %s: %s\n", unit, strings.TrimSpace(string(out)))
					fmt.Fprintf(os.Stderr, "       Restart it yourself with: sudo systemctl restart %s\n", unit)
				}
			}
		}
	}
}

// ── The privileged half ───────────────────────────────────────────────────────

func agentCommand(ctx context.Context, args []string) {
	fs := flag.NewFlagSet("agent", flag.ExitOnError)
	socket := fs.String("socket", agent.SocketPath, "unix socket to listen on")
	group := fs.String("group", "croft", "group allowed to reach the socket")
	nginxDir := fs.String("nginx-dir", "/etc/nginx/croft.d", "directory holding generated vhosts")
	_ = fs.Parse(reorder(fs, args))

	if os.Geteuid() != 0 {
		fmt.Fprintln(os.Stderr, "[ERROR] The agent is the privileged half; it must run as root.")
		os.Exit(1)
	}

	h := host.NewLocal()
	flavor, bin := runtime.Detect(ctx, h)
	if flavor == runtime.FlavorNone {
		fmt.Fprintln(os.Stderr, "[ERROR] No container runtime found. Install Incus or LXD.")
		os.Exit(1)
	}

	server := agent.NewServer(
		runtime.NewCLIInstanceRepository(h, bin, flavor),
		nginx.NewNginxRouteRepository(h, *nginxDir),
		pemCertificates.NewPEMCertificateRepository(h),
		h,
		string(flavor),
		version,
	)

	listener, err := agent.Listen(*socket, *group)
	if err != nil {
		fmt.Fprintln(os.Stderr, "[ERROR]", err)
		if strings.Contains(err.Error(), "no group") {
			fmt.Fprintf(os.Stderr, "        Create it with:  groupadd --system %s\n", *group)
		}
		os.Exit(1)
	}
	defer listener.Close()

	fmt.Printf("croft agent %s — runtime: %s\n", version, flavor)
	fmt.Printf("Listening on %s, reachable by group %s\n", *socket, *group)

	if err := http.Serve(listener, server.Handler()); err != nil {
		fmt.Fprintln(os.Stderr, "[ERROR]", err)
		os.Exit(1)
	}
}

// browsableURL turns a listen address into something you can paste into a
// browser. ":8080" means every interface; "127.0.0.1:8080" means itself.
func browsableURL(addr string) string {
	if strings.HasPrefix(addr, ":") {
		return "http://localhost" + addr
	}
	return "http://" + addr
}
