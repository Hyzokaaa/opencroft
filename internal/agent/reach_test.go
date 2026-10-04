package agent

import (
	"context"
	"errors"
	"net/http"
	"path/filepath"
	"testing"
)

// A panel started before its agent is told the agent is not answering — not
// some other error — and starts working the moment the agent is there, with
// nothing restarted in between.
func TestAPanelRecoversWhenItsAgentAppears(t *testing.T) {
	socket := filepath.Join(t.TempDir(), "agent.sock")
	client := Connect(socket, "test")

	if _, err := client.FindAll(context.Background()); !errors.Is(err, ErrUnreachable) {
		t.Fatalf("with no agent: %v", err)
	}

	server, _ := testServer()
	listener, err := Listen(socket, "")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	go http.Serve(listener, server.Handler())

	found, err := client.FindAll(context.Background())
	if err != nil || len(found) == 0 {
		t.Fatalf("once the agent is there: %v, %d containers", err, len(found))
	}
	if client.Flavor() != "lxd" {
		t.Errorf("the runtime was not learned: %q", client.Flavor())
	}
}

// Two halves of different versions are refused, and waiting does not change
// that — it is not the agent being down.
func TestAnAgentOfAnotherVersionIsRefused(t *testing.T) {
	socket := filepath.Join(t.TempDir(), "agent.sock")
	server, _ := testServer()
	listener, err := Listen(socket, "")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	go http.Serve(listener, server.Handler())

	_, err = Connect(socket, "9.9.9").FindAll(context.Background())
	if err == nil || errors.Is(err, ErrUnreachable) {
		t.Errorf("a version mismatch passed as %v", err)
	}
}
