package main

import (
	"flag"
	"testing"
)

// Go's flag package stops at the first non-flag argument, so
// `croft create app1 --port 3000` silently ignored the port. Nobody writes
// commands in the order the parser wants.
func TestFlagsWorkAfterAPositionalArgument(t *testing.T) {
	set := flag.NewFlagSet("create", flag.ContinueOnError)
	port := set.Int("port", 80, "")
	demo := set.Bool("demo", false, "")
	memory := set.String("memory", "4GB", "")

	args := []string{"app1", "--port", "3000", "--demo", "--memory", "8GB"}
	if err := set.Parse(reorder(set, args)); err != nil {
		t.Fatal(err)
	}

	switch {
	case *port != 3000:
		t.Errorf("port is %d", *port)
	case !*demo:
		t.Error("the boolean flag was lost")
	case *memory != "8GB":
		t.Errorf("memory is %q", *memory)
	case set.NArg() != 1 || set.Arg(0) != "app1":
		t.Errorf("the name was lost: %v", set.Args())
	}
}

func TestFlagsStillWorkBeforeAPositionalArgument(t *testing.T) {
	set := flag.NewFlagSet("create", flag.ContinueOnError)
	port := set.Int("port", 80, "")

	if err := set.Parse(reorder(set, []string{"--port", "3000", "app1"})); err != nil {
		t.Fatal(err)
	}
	if *port != 3000 || set.Arg(0) != "app1" {
		t.Fatalf("port %d, name %q", *port, set.Arg(0))
	}
}

func TestTheEqualsFormIsUnderstood(t *testing.T) {
	set := flag.NewFlagSet("create", flag.ContinueOnError)
	port := set.Int("port", 80, "")

	if err := set.Parse(reorder(set, []string{"app1", "--port=3000"})); err != nil {
		t.Fatal(err)
	}
	if *port != 3000 {
		t.Fatalf("port is %d", *port)
	}
}

// A boolean flag takes no value, so the token after it is a positional
// argument and must not be eaten.
func TestABooleanFlagDoesNotSwallowTheNextArgument(t *testing.T) {
	set := flag.NewFlagSet("create", flag.ContinueOnError)
	demo := set.Bool("demo", false, "")

	if err := set.Parse(reorder(set, []string{"--demo", "app1"})); err != nil {
		t.Fatal(err)
	}
	if !*demo {
		t.Error("the flag was lost")
	}
	if set.Arg(0) != "app1" {
		t.Errorf("the name was eaten: %v", set.Args())
	}
}

// Everything after `--` is a name, even if it looks like a flag.
func TestADoubleDashEndsTheFlags(t *testing.T) {
	set := flag.NewFlagSet("destroy", flag.ContinueOnError)
	set.Bool("demo", false, "")

	if err := set.Parse(reorder(set, []string{"--", "--not-a-flag"})); err != nil {
		t.Fatal(err)
	}
	if set.Arg(0) != "--not-a-flag" {
		t.Fatalf("got %v", set.Args())
	}
}

func TestAListenAddressBecomesSomethingYouCanOpen(t *testing.T) {
	cases := map[string]string{
		":8080":          "http://localhost:8080",
		"127.0.0.1:8080": "http://127.0.0.1:8080",
		"0.0.0.0:9000":   "http://0.0.0.0:9000",
	}

	for addr, want := range cases {
		if got := browsableURL(addr); got != want {
			t.Errorf("%s → %s, wanted %s", addr, got, want)
		}
	}
}
