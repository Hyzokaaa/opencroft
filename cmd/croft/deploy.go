package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/Hyzokaaa/opencroft/internal/agent"
	"github.com/Hyzokaaa/opencroft/internal/shared/plan"
)

// deployCommand is the same two plans the panel shows, in a terminal: look at
// the repository, then say what deploying it would do. Each is printed and
// confirmed before it runs. Nothing here can be approved unseen.
func deployCommand(ctx context.Context, args []string) {
	fs := flag.NewFlagSet("deploy", flag.ExitOnError)
	repo := fs.String("repo", "", "repository to deploy (a public https URL)")
	name := fs.String("name", "", "what to call the service (default: the repository name)")
	branch := fs.String("branch", "main", "branch to deploy")
	path := fs.String("path", "", "where the code lands inside the container")
	start := fs.String("start", "", "override the start command")
	port := fs.Int("port", 0, "override the port it listens on")
	health := fs.String("health", "", "path to ask for once it is up, e.g. /health")
	envFile := fs.String("env", "", "file of KEY=value lines to install beside the code")
	yes := fs.Bool("yes", false, "do not ask; run both plans")
	socket := fs.String("agent", agent.SocketPath, "socket of the privileged agent")
	_ = fs.Parse(reorder(fs, args))

	if fs.NArg() < 1 || *repo == "" {
		fmt.Fprintln(os.Stderr,
			"[ERROR] Usage: croft deploy <container> --repo https://github.com/you/app.git")
		os.Exit(1)
	}
	container := fs.Arg(0)

	env, err := readEnv(*envFile)
	if err != nil {
		fail(err)
	}

	client, err := agent.Dial(*socket, version)
	if err != nil {
		fmt.Fprintln(os.Stderr, "[ERROR]", err)
		fmt.Fprintln(os.Stderr,
			"        The agent does the work here; start it with: sudo systemctl start croft-agent")
		os.Exit(1)
	}

	services := client.Services()
	want := agent.ServiceDTO{
		Name: serviceName(*name, *repo), Repo: *repo, Branch: *branch, Path: *path,
		Env: env, Health: agent.HealthDTO{Path: *health},
	}

	// First plan: fetch the code so there is something to look at. It builds
	// nothing and starts nothing, and the snapshot it takes first means even
	// that much is reversible.
	inspectPlan, err := services.InspectPlan(ctx, container, want)
	if err != nil {
		fail(err)
	}
	fmt.Printf("\n  Look at %s inside %s as %s. Nothing is built and nothing is started.\n\n",
		*repo, container, want.Name)
	printPlan(inspectPlan.Steps)

	if !*yes && !confirm("  Fetch it?") {
		fmt.Println("\n  Nothing happened.")
		return
	}

	detection, err := services.Inspect(ctx, container, want)
	if err != nil {
		fail(err)
	}

	fmt.Println()
	if detection.Found {
		fmt.Printf("  Detected %s — %s\n", detection.Runtime, detection.Why)
	} else {
		fmt.Printf("  %s\n", detection.Why)
	}

	want.Install, want.Build = detection.Install, detection.Build
	want.Start, want.Runtime = detection.Start, detection.Runtime
	want.Port, want.Packages = detection.Port, detection.Packages

	// Anything given on the command line always wins over what we guessed.
	if *start != "" {
		want.Start = *start
	}
	if *port != 0 {
		want.Port = *port
	}

	if strings.TrimSpace(want.Start) == "" {
		fmt.Fprintln(os.Stderr, "\n[ERROR] Nothing says how to start it.")
		fmt.Fprintln(os.Stderr, "        Pass one:  --start 'node server.js'")
		os.Exit(1)
	}

	// Second plan: everything that building and running it means, in full.
	deployPlan, err := services.DeployPlan(ctx, container, want)
	if err != nil {
		fail(err)
	}
	fmt.Print("\n  Deploy it. A snapshot is taken first, so this can be undone.\n\n")
	printPlan(deployPlan.Steps)

	if !*yes && !confirm("  Run these?") {
		fmt.Println("\n  Nothing else happened. The code is fetched, and that is all.")
		return
	}

	fmt.Println()
	if err := services.Deploy(ctx, container, want, func(_ int, text string) {
		fmt.Println("  " + text)
	}); err != nil {
		fmt.Fprintln(os.Stderr, "\n[ERROR]", err)
		fmt.Fprintln(os.Stderr, "        What it said on the way down:")

		if logs, logErr := services.Logs(ctx, container, want.Name, 30); logErr == nil {
			for _, line := range strings.Split(strings.TrimSpace(logs), "\n") {
				fmt.Fprintln(os.Stderr, "          "+line)
			}
		}
		fmt.Fprintf(os.Stderr, "\n        Go back with:  croft rollback %s\n", container)
		os.Exit(1)
	}

	fmt.Printf("\n[OK] %s is running in %s", want.Name, container)
	if want.Port > 0 {
		fmt.Printf(" on port %d", want.Port)
	}
	fmt.Println(".")
	fmt.Println()
}

// serviceName defaults to what the repository is called, because that is what
// a person would have typed anyway.
func serviceName(given, repo string) string {
	if given != "" {
		return given
	}

	name := repo
	if at := strings.LastIndex(name, "/"); at >= 0 {
		name = name[at+1:]
	}
	return strings.ToLower(strings.TrimSuffix(name, ".git"))
}

// readEnv parses KEY=value lines. It is parsed, never sourced: sourcing it as
// a shell would run whatever is in it, as root.
func readEnv(path string) (map[string]string, error) {
	if path == "" {
		return nil, nil
	}

	body, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	env := map[string]string{}
	for _, line := range strings.Split(string(body), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		key, value, found := strings.Cut(line, "=")
		if !found {
			return nil, fmt.Errorf("%s is not a KEY=value line: %q", path, line)
		}
		env[strings.TrimSpace(key)] = strings.Trim(strings.TrimSpace(value), `"'`)
	}
	return env, nil
}

// rollbackCommand restores a snapshot. It takes the whole container back, and
// the plan says what that reaches before anything happens.
func rollbackCommand(ctx context.Context, args []string) {
	fs := flag.NewFlagSet("rollback", flag.ExitOnError)
	snapshot := fs.String("snapshot", "", "which snapshot to restore (default: the last that worked)")
	yes := fs.Bool("yes", false, "do not ask")
	socket := fs.String("agent", agent.SocketPath, "socket of the privileged agent")
	_ = fs.Parse(reorder(fs, args))

	if fs.NArg() < 1 {
		fmt.Fprintln(os.Stderr, "[ERROR] Usage: croft rollback <container> [--snapshot <name>]")
		os.Exit(1)
	}
	container := fs.Arg(0)

	client, err := agent.Dial(*socket, version)
	if err != nil {
		fail(err)
	}
	services := client.Services()

	found, err := services.FindAll(ctx, container)
	if err != nil {
		fail(err)
	}

	target := *snapshot
	if target == "" {
		target = lastHealthy(found)
	}
	if target == "" {
		fmt.Fprintf(os.Stderr,
			"[ERROR] No version of %s is recorded as having worked, so there is nothing to go back to.\n",
			container)
		if len(found.Snapshots) > 0 {
			fmt.Fprintln(os.Stderr, "        Snapshots on this container:")
			for _, name := range found.Snapshots {
				fmt.Fprintln(os.Stderr, "          "+name)
			}
			fmt.Fprintln(os.Stderr, "\n        Pick one with --snapshot.")
		}
		os.Exit(1)
	}

	p, warning, err := services.RollbackPlan(ctx, container, target)
	if err != nil {
		fail(err)
	}

	fmt.Printf("\n  Restore %s onto %s.\n", target, container)
	if warning != "" {
		fmt.Printf("\n  %s\n", warning)
	}
	fmt.Println()
	printPlan(p.Steps)

	if !*yes && !confirm("  Restore it?") {
		fmt.Println("\n  Nothing happened.")
		return
	}

	fmt.Println()
	if err := services.Rollback(ctx, container, target, func(_ int, text string) {
		fmt.Println("  " + text)
	}); err != nil {
		fail(err)
	}
	fmt.Printf("\n[OK] %s is back at %s\n\n", container, target)
}

// lastHealthy is the newest snapshot any service recorded as having passed its
// check. Names sort by time, so the largest is the most recent.
func lastHealthy(found agent.ServicesResponse) string {
	best := ""
	for _, service := range found.Services {
		if service.Healthy > best {
			best = service.Healthy
		}
	}
	return best
}

func printPlan(steps []plan.Step) {
	for i, step := range steps {
		fmt.Printf("    %d. %s\n", i+1, step.Describe)
		fmt.Printf("       %s\n", step.Shell())
	}
	fmt.Println()
}

func confirm(question string) bool {
	fmt.Print(question + " [y/N] ")

	answer, err := bufio.NewReader(os.Stdin).ReadString('\n')
	if err != nil {
		return false
	}
	answer = strings.ToLower(strings.TrimSpace(answer))
	return answer == "y" || answer == "yes"
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "[ERROR]", err)
	os.Exit(1)
}
