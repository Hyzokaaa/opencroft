// Package job runs a plan in the background and streams what happens.
//
// Creating a container takes minutes, which does not fit in an HTTP request.
// The work belongs to the daemon, not to the browser: closing the tab must not
// abandon a half-created container.
package job

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/Hyzokaaa/opencroft/internal/shared/plan"
)

type Status string

const (
	StatusRunning Status = "running"
	StatusDone    Status = "done"
	StatusFailed  Status = "failed"
)

type Event struct {
	At      time.Time `json:"at"`
	Step    int       `json:"step"`
	Total   int       `json:"total"`
	Text    string    `json:"text"`
	Command string    `json:"command,omitempty"`
	Failed  bool      `json:"failed,omitempty"`
}

// Snapshot is the serialisable view. Job itself holds a mutex, so it must
// never be copied — including by an encoder.
type Snapshot struct {
	Id      string    `json:"id"`
	Kind    string    `json:"kind"`
	Subject string    `json:"subject"`
	Status  Status    `json:"status"`
	Plan    plan.Plan `json:"plan"`
	Events  []Event   `json:"events"`
	Error   string    `json:"error,omitempty"`
	Started time.Time `json:"started"`
	Ended   time.Time `json:"ended,omitzero"`
}

type Job struct {
	Snapshot

	mu        sync.Mutex
	listeners map[chan Event]struct{}
}

func (j *Job) snapshotLocked() Snapshot {
	copied := j.Snapshot
	copied.Events = append([]Event{}, j.Events...)
	return copied
}

// Runner keeps jobs in memory. They outlive the request that started them, but
// not a restart of the daemon — which is honest: a restart is exactly when you
// want to look at the system itself rather than at our record of it.
type Runner struct {
	mu   sync.Mutex
	jobs map[string]*Job
	next func() string
}

func NewRunner(idGenerator func() string) *Runner {
	return &Runner{jobs: map[string]*Job{}, next: idGenerator}
}

// Start runs the work in the background and returns immediately. The step
// function reports progress; the runner turns it into events.
func (r *Runner) Start(kind, subject string, p plan.Plan, work func(ctx context.Context, report func(step int, text string)) error) *Job {
	j := &Job{
		Snapshot: Snapshot{
			Id:      r.next(),
			Kind:    kind,
			Subject: subject,
			Status:  StatusRunning,
			Plan:    p,
			Events:  []Event{},
			Started: time.Now().UTC(),
		},
		listeners: map[chan Event]struct{}{},
	}

	r.mu.Lock()
	r.jobs[j.Id] = j
	r.mu.Unlock()

	go func() {
		// Detached from the request on purpose: closing the browser must not
		// abandon a container halfway through being created.
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
		defer cancel()

		err := work(ctx, func(step int, text string) {
			command := ""
			if step > 0 && step <= len(p.Steps) {
				command = p.Steps[step-1].Shell()
			}
			j.emit(Event{At: time.Now().UTC(), Step: step, Total: len(p.Steps), Text: text, Command: command})
		})

		j.mu.Lock()
		j.Ended = time.Now().UTC()
		if err != nil {
			j.Status = StatusFailed
			j.Error = err.Error()
		} else {
			j.Status = StatusDone
		}
		j.mu.Unlock()

		j.emit(Event{
			At:     time.Now().UTC(),
			Total:  len(p.Steps),
			Text:   finalText(err),
			Failed: err != nil,
		})
		j.closeListeners()
	}()

	return j
}

func finalText(err error) string {
	if err != nil {
		return fmt.Sprintf("Stopped: %v", err)
	}
	return "Done."
}

func (r *Runner) Find(id string) (Snapshot, bool) {
	r.mu.Lock()
	j, ok := r.jobs[id]
	r.mu.Unlock()
	if !ok {
		return Snapshot{}, false
	}

	j.mu.Lock()
	defer j.mu.Unlock()
	return j.snapshotLocked(), true
}

func (r *Runner) job(id string) (*Job, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	j, ok := r.jobs[id]
	return j, ok
}

// Subscribe replays what already happened, then follows. A client that arrives
// late still sees the whole story.
func (r *Runner) Subscribe(id string) (<-chan Event, func(), bool) {
	j, ok := r.job(id)
	if !ok {
		return nil, nil, false
	}

	ch := make(chan Event, 64)

	j.mu.Lock()
	for _, e := range j.Events {
		select {
		case ch <- e:
		default:
		}
	}
	finished := j.Status != StatusRunning
	if !finished {
		j.listeners[ch] = struct{}{}
	}
	j.mu.Unlock()

	if finished {
		close(ch)
		return ch, func() {}, true
	}

	return ch, func() {
		j.mu.Lock()
		delete(j.listeners, ch)
		j.mu.Unlock()
	}, true
}

func (j *Job) emit(e Event) {
	j.mu.Lock()
	j.Events = append(j.Events, e)
	for ch := range j.listeners {
		select {
		case ch <- e:
		default: // a slow reader must not stall the work
		}
	}
	j.mu.Unlock()
}

func (j *Job) closeListeners() {
	j.mu.Lock()
	for ch := range j.listeners {
		close(ch)
		delete(j.listeners, ch)
	}
	j.mu.Unlock()
}
