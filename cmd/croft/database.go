package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/Hyzokaaa/opencroft/internal/agent"
)

// databaseCommand is the same operations the panel offers, in a terminal.
//
// A database lives inside the container that uses it, which is what makes a
// snapshot of that container a snapshot of the application and its data at the
// same instant. The password is generated in there and never comes back out —
// not to the panel, not to this terminal, not into the plan you are about to
// read.
func databaseCommand(ctx context.Context, args []string) {
	if len(args) == 0 {
		databaseUsage()
		os.Exit(1)
	}

	switch args[0] {
	case "list", "ls":
		databaseList(ctx, args[1:])
	case "add":
		databaseAdd(ctx, args[1:])
	case "rm", "remove":
		databaseRemove(ctx, args[1:])
	default:
		databaseUsage()
		os.Exit(1)
	}
}

func databaseUsage() {
	fmt.Fprintln(os.Stderr, `Usage:
  croft db list <container>
  croft db add <container> --engine postgres|mysql|redis [--name main]
  croft db rm <container> <database>

A database lives inside the container that uses it, so a snapshot of that
container holds the application and its data together.`)
}

func databaseDial(socket string) *agent.DatabaseClient {
	client, err := agent.Dial(socket, version)
	if err != nil {
		fmt.Fprintln(os.Stderr, "[ERROR]", err)
		fmt.Fprintln(os.Stderr,
			"        The agent does the work here; start it with: sudo systemctl start croft-agent")
		os.Exit(1)
	}
	return client.Databases()
}

func databaseList(ctx context.Context, args []string) {
	fs := flag.NewFlagSet("db list", flag.ExitOnError)
	socket := fs.String("agent", agent.SocketPath, "socket of the privileged agent")
	_ = fs.Parse(reorder(fs, args))

	if fs.NArg() < 1 {
		fmt.Fprintln(os.Stderr, "[ERROR] Usage: croft db list <container>")
		os.Exit(1)
	}

	found, err := databaseDial(*socket).FindAll(ctx, fs.Arg(0))
	if err != nil {
		fail(err)
	}

	if len(found.Databases) == 0 {
		fmt.Printf("\n  %s has no database.\n\n", fs.Arg(0))
		fmt.Printf("  Add one with:  croft db add %s --engine postgres\n\n", fs.Arg(0))
		return
	}

	fmt.Println()
	fmt.Printf("  %-12s %-10s %-14s %-10s %s\n", "NAME", "ENGINE", "DATABASE", "PORT", "STATE")
	for _, d := range found.Databases {
		state := d.State
		if d.Location != "" {
			state = "in " + d.Location
		}
		if state == "" {
			state = "unknown"
		}
		fmt.Printf("  %-12s %-10s %-14s %-10d %s\n", d.Name, d.Engine, d.DB, d.Port, state)
	}
	fmt.Println()
}

func databaseAdd(ctx context.Context, args []string) {
	fs := flag.NewFlagSet("db add", flag.ExitOnError)
	engine := fs.String("engine", "", "postgres, mysql or redis")
	name := fs.String("name", "", "what to call it on the container (default: the engine)")
	db := fs.String("db", "", "name of the database to create (default: the name)")
	user := fs.String("user", "", "name of the user to create (default: the name)")
	yes := fs.Bool("yes", false, "do not ask; run the plan")
	socket := fs.String("agent", agent.SocketPath, "socket of the privileged agent")
	_ = fs.Parse(reorder(fs, args))

	if fs.NArg() < 1 || *engine == "" {
		fmt.Fprintln(os.Stderr,
			"[ERROR] Usage: croft db add <container> --engine postgres|mysql|redis")
		os.Exit(1)
	}
	container := fs.Arg(0)

	chosen := *name
	if chosen == "" {
		chosen = *engine
	}

	client := databaseDial(*socket)
	want := agent.DatabaseDTO{
		Name: chosen, Engine: *engine, DB: *db, User: *user,
	}

	p, err := client.ProvisionPlan(ctx, container, want)
	if err != nil {
		fail(err)
	}

	fmt.Printf("\n  Install %s inside %s. The password is generated in there and never\n"+
		"  leaves it — which is why you cannot see one below.\n\n", *engine, container)
	printPlan(p.Steps)

	fmt.Printf("  From here on, a snapshot of %s holds the data as well as the code.\n"+
		"  That is what makes going back work, and it is what to remember when you do it.\n\n",
		container)

	if !*yes && !confirm("  Run these?") {
		fmt.Println("\n  Nothing happened.")
		return
	}

	fmt.Println()
	if err := client.Provision(ctx, container, want, func(_ int, text string) {
		fmt.Println("  " + text)
	}); err != nil {
		fail(err)
	}

	fmt.Printf("\n  %s is ready. The services in %s read it from /etc/croft/db.env.\n\n",
		chosen, container)
}

func databaseRemove(ctx context.Context, args []string) {
	fs := flag.NewFlagSet("db rm", flag.ExitOnError)
	yes := fs.Bool("yes", false, "do not ask; run the plan")
	socket := fs.String("agent", agent.SocketPath, "socket of the privileged agent")
	_ = fs.Parse(reorder(fs, args))

	if fs.NArg() < 2 {
		fmt.Fprintln(os.Stderr, "[ERROR] Usage: croft db rm <container> <database>")
		os.Exit(1)
	}
	container, target := fs.Arg(0), fs.Arg(1)

	client := databaseDial(*socket)

	p, warning, err := client.DestroyPlan(ctx, container, target)
	if err != nil {
		fail(err)
	}

	fmt.Printf("\n  Remove %s from %s.\n\n", target, container)
	printPlan(p.Steps)

	if warning != "" {
		for _, line := range strings.Split(warning, ". ") {
			fmt.Println("  " + strings.TrimSpace(line))
		}
		fmt.Println()
	}

	if !*yes && !confirm("  Run these?") {
		fmt.Println("\n  Nothing happened.")
		return
	}

	fmt.Println()
	if err := client.Destroy(ctx, container, target, func(_ int, text string) {
		fmt.Println("  " + text)
	}); err != nil {
		fail(err)
	}
	fmt.Println()
}
