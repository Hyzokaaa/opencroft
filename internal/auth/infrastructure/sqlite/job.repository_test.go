package sqlite

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/Hyzokaaa/opencroft/internal/shared/job"
)

func aJournal(t *testing.T) *JobJournal {
	t.Helper()
	store, err := Open(filepath.Join(t.TempDir(), "croft.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return NewJobJournal(store)
}

func at(minute int) time.Time { return time.Date(2026, 9, 27, 10, minute, 0, 0, time.UTC) }

// A job is written when it starts and again when it ends: the second write is
// the same job, not another one.
func TestAJobEndsWhereItStarted(t *testing.T) {
	journal := aJournal(t)

	running := job.Snapshot{Id: "01", Kind: "deploy", Subject: "helpdesk/web", Status: job.StatusRunning, Started: at(0)}
	_ = journal.Record(running)
	running.Status, running.Error = job.StatusFailed, "Build: exit status 1"
	_ = journal.Record(running)

	found, _ := journal.Recent(10)
	if len(found) != 1 || found[0].Status != job.StatusFailed || found[0].Error != "Build: exit status 1" {
		t.Fatalf("recorded %+v", found)
	}
}

func TestTheNewestComesFirst(t *testing.T) {
	journal := aJournal(t)
	for i, id := range []string{"old", "new", "middle"} {
		_ = journal.Record(job.Snapshot{Id: id, Kind: "deploy", Status: job.StatusDone, Started: at([]int{1, 9, 5}[i])})
	}

	found, _ := journal.Recent(2)
	if len(found) != 2 || found[0].Id != "new" || found[1].Id != "middle" {
		t.Fatalf("read %v", found)
	}
}

// Whatever a previous run of the daemon left running was walked away from.
// Calling it done would be a lie, and so would calling it failed.
func TestARestartSettlesWhatWasLeftRunning(t *testing.T) {
	journal := aJournal(t)
	_ = journal.Record(job.Snapshot{Id: "left", Kind: "deploy", Status: job.StatusRunning, Started: at(0)})
	_ = journal.Record(job.Snapshot{Id: "finished", Kind: "deploy", Status: job.StatusDone, Started: at(1)})

	if err := journal.Settle(context.Background()); err != nil {
		t.Fatal(err)
	}

	left, _, _ := journal.Find("left")
	finished, _, _ := journal.Find("finished")
	if left.Status != job.StatusInterrupted || finished.Status != job.StatusDone {
		t.Errorf("left is %s, finished is %s", left.Status, finished.Status)
	}
}
