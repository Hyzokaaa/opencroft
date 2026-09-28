package job

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Hyzokaaa/opencroft/internal/shared/plan"
)

// memoryJournal is a journal that forgets with the test.
type memoryJournal struct {
	mu      sync.Mutex
	written []Snapshot
}

func (m *memoryJournal) Record(s Snapshot) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.written = append(m.written, s)
	return nil
}

func (m *memoryJournal) last(id string) (Snapshot, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i := len(m.written) - 1; i >= 0; i-- {
		if m.written[i].Id == id {
			return m.written[i], true
		}
	}
	return Snapshot{}, false
}

func (m *memoryJournal) Recent(int) ([]Snapshot, error) { return nil, nil }

func (m *memoryJournal) Find(id string) (Snapshot, bool, error) {
	s, ok := m.last(id)
	return s, ok, nil
}

func until(t *testing.T, done func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for !done() {
		if time.Now().After(deadline) {
			t.Fatal("never happened")
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// A job is written down when it starts — so a restart halfway leaves a trace —
// and again when it ends, with how it ended.
func TestAJobIsWrittenDownWhenItStartsAndWhenItEnds(t *testing.T) {
	journal := &memoryJournal{}
	runner := NewRunner(counter(), journal)

	release := make(chan struct{})
	j := runner.Start("deploy", "app/web", onestep(), func(context.Context, func(int, string)) error {
		<-release
		return errors.New("npm ci: exit status 1")
	})

	started, ok := journal.last(j.Id)
	if !ok || started.Status != StatusRunning {
		t.Fatalf("not written down as running: %+v", started)
	}

	close(release)
	until(t, func() bool { s, _ := journal.last(j.Id); return s.Status == StatusFailed })

	ended, _ := journal.last(j.Id)
	if !strings.Contains(ended.Error, "npm ci") {
		t.Errorf("why it failed was not kept: %q", ended.Error)
	}
}

// What the daemon no longer holds in memory is still there to be opened.
func TestAnOldJobIsFoundInTheJournal(t *testing.T) {
	journal := &memoryJournal{}
	_ = journal.Record(Snapshot{Id: "from-before", Kind: "deploy", Status: StatusDone})

	found, ok := NewRunner(counter(), journal).Find("from-before")
	if !ok || found.Kind != "deploy" {
		t.Fatalf("found %+v, %v", found, ok)
	}
}

// A step marked secret carries secrets in its command. It is shown before it
// runs and forgotten after: neither its command nor the error that quotes it
// is written down.
func TestASecretStepLeavesNoTraceInTheJournal(t *testing.T) {
	p := plan.New(
		plan.Command("Fetch the code", "git", "fetch"),
		plan.Step{Describe: "Write the .env", Argv: []string{"sh", "-c", "echo JWT_SECRET=hunter2 > .env"}, Secret: true},
	)
	journal := &memoryJournal{}
	j := NewRunner(counter(), journal).Start("deploy", "app/web", p,
		func(_ context.Context, report func(int, string)) error {
			report(1, "Fetch the code")
			report(2, "Write the .env")
			return errors.New("sh -c echo JWT_SECRET=hunter2 > .env: exit status 1")
		})

	until(t, func() bool { s, _ := journal.last(j.Id); return s.Status == StatusFailed })
	kept, _ := journal.last(j.Id)

	written := kept.Error
	for _, step := range kept.Plan.Steps {
		written += step.Shell()
	}
	for _, event := range kept.Events {
		written += event.Text + event.Command
	}
	if strings.Contains(written, "hunter2") {
		t.Errorf("the secret was written down:\n%s", written)
	}
	if !strings.Contains(kept.Error, "Write the .env") {
		t.Errorf("which step failed was lost too: %q", kept.Error)
	}
	if kept.Plan.Steps[0].Shell() != "git fetch" {
		t.Errorf("a step with nothing secret lost its command: %q", kept.Plan.Steps[0].Shell())
	}
}
