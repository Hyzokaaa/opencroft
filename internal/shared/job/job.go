// Package job runs a plan in the background and streams what happens.
//
// Creating a container takes minutes, which does not fit in an HTTP request.
// The work belongs to the daemon, not to the browser: closing the tab must not
// abandon a half-created container.
package job

import (
	"context"
	"fmt"
	"log"
	"sort"
	"sync"
	"time"

	"github.com/Hyzokaaa/opencroft/internal/shared/plan"
)

type Status string

const (
	StatusRunning Status = "running"
	StatusDone    Status = "done"
	StatusFailed  Status = "failed"
	// StatusInterrupted is a job the daemon stopped being around for. It may
	// have finished, failed or stopped halfway — what it did is on the machine,
	// and the snapshot it took first is still there.
	StatusInterrupted Status = "interrupted"
)

// Journal is where work is written down, so that what was done outlives the
// daemon that did it. A job is recorded when it starts and again when it ends.
type Journal interface {
	Record(Snapshot) error
	// Recent is the newest first.
	Recent(limit int) ([]Snapshot, error)
	Find(id string) (Snapshot, bool, error)
}

type Event struct {
	At      time.Time `json:"at"`
	Step    int       `json:"step"`
	Total   int       `json:"total"`
	Text    string    `json:"text"`
	Command string    `json:"command,omitempty"`
	Failed  bool      `json:"failed,omitempty"`
	// End marks the last event, which sums the job up rather than being a
	// step of it — so the panel does not list it as one.
	End bool `json:"end,omitempty"`
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

	// stop cancels the context the work runs under. Stopping a plan halfway
	// leaves the machine halfway, which is why it is the person watching who
	// decides — not a timeout, and not us.
	stop context.CancelFunc
}

// Cancel stops the work where it stands.
//
// What it cannot promise is tidiness: a plan abandoned halfway has applied some
// of its steps and not the rest, and a command already running inside a
// container may outlive the one that launched it. Saying so is better than a
// button that pretends otherwise — and the snapshot taken at step one is still
// there.
func (r *Runner) Cancel(id string) bool {
	j, ok := r.job(id)
	if !ok {
		return false
	}

	j.mu.Lock()
	stop, running := j.stop, j.Status == StatusRunning
	j.mu.Unlock()

	if !running || stop == nil {
		return false
	}
	stop()
	return true
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
	mu      sync.Mutex
	jobs    map[string]*Job
	next    func() string
	journal Journal
}

// NewRunner keeps its jobs in memory, and in the journal when there is one. A
// nil journal keeps nothing beyond this process.
func NewRunner(idGenerator func() string, journal Journal) *Runner {
	return &Runner{jobs: map[string]*Job{}, next: idGenerator, journal: journal}
}

// record writes a job down. A journal that cannot be written to must not stop
// the work it was meant to remember, so the failure is logged and no more.
func (r *Runner) record(j *Job) {
	if r.journal == nil {
		return
	}
	j.mu.Lock()
	snapshot := j.snapshotLocked()
	j.mu.Unlock()

	if err := r.journal.Record(snapshot.Kept()); err != nil {
		log.Printf("job %s could not be written down: %v", snapshot.Id, err)
	}
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
	r.forget()
	r.mu.Unlock()
	r.record(j)

	go func() {
		// Detached from the request on purpose: closing the browser must not
		// abandon a container halfway through being created.
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
		defer cancel()

		j.mu.Lock()
		j.stop = cancel
		j.mu.Unlock()

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
			End:    true,
		})
		r.record(j)
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

// Keep is how many finished jobs are remembered. Enough that the panel can
// still open the one that just failed, and few enough that a daemon left
// running for months does not hold every event of every deployment it ever
// ran. Anything still running is never dropped.
const Keep = 50

// forget drops the oldest finished jobs. The caller holds the lock.
func (r *Runner) forget() {
	finished := make([]*Job, 0, len(r.jobs))
	for _, j := range r.jobs {
		j.mu.Lock()
		done := j.Status != StatusRunning
		j.mu.Unlock()

		if done {
			finished = append(finished, j)
		}
	}
	if len(finished) <= Keep {
		return
	}

	sort.Slice(finished, func(a, b int) bool {
		return finished[a].Ended.Before(finished[b].Ended)
	})
	for _, j := range finished[:len(finished)-Keep] {
		delete(r.jobs, j.Id)
	}
}

// Find looks in memory first — a running job's events are only there — and in
// the journal for anything older than this process or than Keep.
func (r *Runner) Find(id string) (Snapshot, bool) {
	r.mu.Lock()
	j, ok := r.jobs[id]
	r.mu.Unlock()
	if ok {
		j.mu.Lock()
		defer j.mu.Unlock()
		return j.snapshotLocked(), true
	}

	if r.journal == nil {
		return Snapshot{}, false
	}
	found, ok, err := r.journal.Find(id)
	if err != nil {
		log.Printf("reading job %s: %v", id, err)
		return Snapshot{}, false
	}
	return found, ok
}

// Recent is the newest jobs first: from the journal when there is one, since
// it holds everything including what this process ran, and from memory when
// there is not.
func (r *Runner) Recent(limit int) ([]Snapshot, error) {
	if r.journal != nil {
		return r.journal.Recent(limit)
	}

	r.mu.Lock()
	all := make([]Snapshot, 0, len(r.jobs))
	for _, j := range r.jobs {
		j.mu.Lock()
		all = append(all, j.snapshotLocked())
		j.mu.Unlock()
	}
	r.mu.Unlock()

	sort.Slice(all, func(a, b int) bool { return all[a].Started.After(all[b].Started) })
	if len(all) > limit {
		all = all[:limit]
	}
	return all, nil
}

// Kept is what is written down about a job: all of it, except what a step
// marked secret carries — its command, and the error it failed with, which
// quotes the command.
func (s Snapshot) Kept() Snapshot {
	kept := s
	secret := map[int]bool{}

	kept.Plan = plan.Plan{Steps: make([]plan.Step, len(s.Plan.Steps))}
	for i, step := range s.Plan.Steps {
		if step.Secret {
			secret[i+1] = true
			step = plan.Step{Describe: step.Describe, Secret: true}
		}
		kept.Plan.Steps[i] = step
	}

	failedAt := 0
	kept.Events = make([]Event, len(s.Events))
	for i, event := range s.Events {
		if secret[event.Step] {
			event.Command = ""
		}
		if event.Step > 0 {
			failedAt = event.Step
		}
		kept.Events[i] = event
	}

	if s.Status == StatusFailed && secret[failedAt] {
		kept.Error = s.Plan.Steps[failedAt-1].Describe +
			" failed. What it said is not kept, because its command carries secrets."
		for i := range kept.Events {
			if kept.Events[i].Failed {
				kept.Events[i].Text = "Stopped: " + kept.Error
			}
		}
	}
	return kept
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
