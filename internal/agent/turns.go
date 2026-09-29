package agent

import (
	"context"
	"net/http"
	"sync"
)

// turns lets one job at a time write to a container.
//
// Two at once collide in ways that look like somebody else's fault. Taking a
// snapshot freezes the container, and whatever the other job runs in that
// moment fails with "Instance is frozen". Even without that, both would fight
// over apt's lock, and each would snapshot the other halfway through. So the
// second one waits for the first, and says that is what it is doing — a queue
// the person can see, rather than a failure they have to interpret.
type turns struct {
	mu   sync.Mutex
	held map[string]chan struct{}
}

func newTurns() *turns {
	return &turns{held: map[string]chan struct{}{}}
}

func (t *turns) of(container string) chan struct{} {
	t.mu.Lock()
	defer t.mu.Unlock()
	turn, ok := t.held[container]
	if !ok {
		turn = make(chan struct{}, 1)
		t.held[container] = turn
	}
	return turn
}

// take waits for the container's turn, telling waiting when it has to. The
// returned function gives the turn back.
func (t *turns) take(ctx context.Context, container string, waiting func()) (func(), error) {
	turn := t.of(container)

	select {
	case turn <- struct{}{}:
		return func() { <-turn }, nil
	default:
	}

	waiting()
	select {
	case turn <- struct{}{}:
		return func() { <-turn }, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// streamOn is stream for work that writes to one container: it waits for the
// container's turn before any step runs.
func (s *Server) streamOn(w http.ResponseWriter, r *http.Request, container string, run func(report func(int, string)) error) {
	s.stream(w, func(report func(int, string)) error {
		release, err := s.turns.take(r.Context(), container, func() {
			report(0, "Waiting for the other job on "+container+" to finish — two at once would collide")
		})
		if err != nil {
			return err
		}
		defer release()
		return run(report)
	})
}
