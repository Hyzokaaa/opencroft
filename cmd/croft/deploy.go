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
	branch := fs.String("branch", "main", "branch to deploy")
	path := fs.String("path", "/srv/app", "where the code lands inside the container")
	start := fs.String("start", "", "override the start command")
	port := fs.Int("port", 0, "override the port it listens on")
	yes := fs.Bool("yes", false, "do not ask; run both plans")
	socket := fs.String("agent", agent.SocketPath, "socket of the privileged agent")
	_ = fs.Parse(reorder(fs, args))

	if fs.NArg() < 1 || *repo == "" {
		fmt.Fprintln(os.Stderr, "[ERROR] Usage: croft deploy <container> --repo https://github.com/you/app.git")
		os.Exit(1)
	}
	name := fs.Arg(0)

	client, err := agent.Dial(*socket, version)
	if err != nil {
		fmt.Fprintln(os.Stderr, "[ERROR]", err)
		fmt.Fprintln(os.Stderr, "        The agent does the work here; start it with: sudo systemctl start croft-agent")
		os.Exit(1)
	}
	apps := client.Apps()
	want := agent.Inspect{Repo: *repo, Branch: *branch, Path: *path}

	// First plan: fetch the code so there is something to look at. It builds
	// nothing and starts nothing, and the snapshot it takes first means even
	// that much is reversible.
	inspectPlan, err := apps.InspectPlan(ctx, name, want)
	if err != nil {
		fail(err)
	}
	fmt.Printf("\n  Look at %s inside %s. Nothing is built and nothing is started.\n\n", *repo, name)
	printPlan(inspectPlan.Steps)

	if !*yes && !confirm("  Fetch it?") {
		fmt.Println("\n  Nothing happened.")
		return
	}

	detection, err := apps.Inspect(ctx, name, want)
	if err != nil {
		fail(err)
	}

	fmt.Println()
	if detection.Found {
		fmt.Printf("  Detected %s — %s\n", detection.Runtime, detection.Why)
	} else {
		fmt.Printf("  %s\n", detection.Why)
	}

	deploy := agent.Deploy{
		Inspect:  want,
		Install:  detection.Install,
		Build:    detection.Build,
		Start:    detection.Start,
		Runtime:  detection.Runtime,
		Port:     detection.Port,
		Packages: detection.Packages,
	}
	// A command given on the command line always wins over one we guessed.
	if *start != "" {
		deploy.Start = *start
	}
	if *port != 0 {
		deploy.Port = *port
	}

	if strings.TrimSpace(deploy.Start) == "" {
		fmt.Fprintln(os.Stderr, "\n[ERROR] Nothing says how to start it.")
		fmt.Fprintln(os.Stderr, "        Pass one: croft deploy "+name+" --repo "+*repo+" --start 'node server.js'")
		os.Exit(1)
	}

	// Second plan: everything that building and running it means, in full.
	deployPlan, err := apps.DeployPlan(ctx, name, deploy)
	if err != nil {
		fail(err)
	}
	fmt.Printf("\n  Deploy it. A snapshot is taken first, so this can be undone.\n\n")
	printPlan(deployPlan.Steps)

	if !*yes && !confirm("  Run these?") {
		fmt.Println("\n  Nothing else happened. The code is fetched, and that is all.")
		return
	}

	fmt.Println()
	if err := apps.Deploy(ctx, name, deploy, func(_ int, text string) {
		fmt.Println("  " + text)
	}); err != nil {
		fmt.Fprintln(os.Stderr, "\n[ERROR]", err)
		fmt.Fprintln(os.Stderr, "        What it said on the way down:")
		if logs, logErr := apps.Logs(ctx, name, 30); logErr == nil {
			for _, line := range strings.Split(strings.TrimSpace(logs), "\n") {
				fmt.Fprintln(os.Stderr, "          "+line)
			}
		}
		os.Exit(1)
	}

	fmt.Printf("\n[OK] %s is running in %s", *repo, name)
	if deploy.Port > 0 {
		fmt.Printf(" on port %d", deploy.Port)
	}
	fmt.Println(".")
	fmt.Println("\n  Give it a domain with:  croft serve  →  Add domain")
	fmt.Println()
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
