package agent

import (
	"context"
	"testing"
	"time"
)

// Two jobs on one container at once collide — a snapshot freezes it under the
// other's feet. The second waits, and says so, instead of failing.
func TestASecondJobOnAContainerWaitsItsTurn(t *testing.T) {
	turns := newTurns()
	ctx := context.Background()

	release, _ := turns.take(ctx, "helpdesk", func() { t.Error("the first job was made to wait") })

	waited := make(chan struct{})
	got := make(chan struct{})
	go func() {
		next, _ := turns.take(ctx, "helpdesk", func() { close(waited) })
		close(got)
		next()
	}()

	select {
	case <-waited:
	case <-time.After(time.Second):
		t.Fatal("the second job never said it was waiting")
	}
	select {
	case <-got:
		t.Fatal("the second job ran while the first still had the container")
	case <-time.After(50 * time.Millisecond):
	}

	release()
	select {
	case <-got:
	case <-time.After(time.Second):
		t.Fatal("the second job never got its turn")
	}

	// Another container is another queue.
	other, _ := turns.take(ctx, "landing", func() { t.Error("a job waited on an unrelated container") })
	other()
}
