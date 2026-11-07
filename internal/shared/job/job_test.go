package job

import (
	"context"
	"testing"
	"time"

	"github.com/Hyzokaaa/opencroft/internal/shared/plan"
)

func counter() func() string {
	n := 0
	return func() string {
		n++
		return "job-" + string(rune('0'+n))
	}
}

func onestep() plan.Plan {
	return plan.New(plan.Command("do the thing", "true"))
}

// Closing the window and stopping the work are different things. Until there
// was a way to say the second, a command that was never going to finish held
// everything until something far away gave up.
func TestCancellingReachesTheWorkItself(t *testing.T) {
	runner := NewRunner(counter(), nil)
	started, stopped := make(chan struct{}), make(chan struct{})

	j := runner.Start("deploy", "app", onestep(), func(ctx context.Context, _ func(int, string)) error {
		close(started)
		<-ctx.Done()
		close(stopped)
		return ctx.Err()
	})

	<-started
	if !runner.Cancel(j.Id) {
		t.Fatal("cancelling a running job was refused")
	}

	select {
	case <-stopped:
	case <-time.After(2 * time.Second):
		t.Fatal("the work was never told to stop")
	}
}

// A job that already finished has nothing to stop, and saying so is better
// than reporting success for doing nothing.
func TestCancellingSomethingThatIsNotRunningSaysSo(t *testing.T) {
	runner := NewRunner(counter(), nil)

	done := make(chan struct{})
	j := runner.Start("deploy", "app", onestep(), func(context.Context, func(int, string)) error {
		return nil
	})

	go func() {
		for {
			if snapshot, _ := runner.Find(j.Id); snapshot.Status != StatusRunning {
				close(done)
				return
			}
			time.Sleep(time.Millisecond)
		}
	}()
	<-done

	if runner.Cancel(j.Id) {
		t.Error("claimed to stop a job that had already finished")
	}
	if runner.Cancel("nothing-like-this") {
		t.Error("claimed to stop a job that does not exist")
	}
}

// A client that arrives after the work started still needs the whole story —
// which is what makes a lost connection cost the live narration and nothing
// else.
func TestSubscribingLateStillSeesWhatHappened(t *testing.T) {
	runner := NewRunner(counter(), nil)
	ready := make(chan struct{})

	j := runner.Start("deploy", "app", onestep(), func(ctx context.Context, report func(int, string)) error {
		report(1, "the first thing")
		report(2, "the second thing")
		close(ready)
		<-ctx.Done()
		return nil
	})

	<-ready
	events, unsubscribe, ok := runner.Subscribe(j.Id)
	if !ok {
		t.Fatal("the job was not there")
	}
	defer unsubscribe()

	seen := []string{}
	for i := 0; i < 2; i++ {
		select {
		case e := <-events:
			seen = append(seen, e.Text)
		case <-time.After(time.Second):
			t.Fatalf("only saw %v", seen)
		}
	}

	if seen[0] != "the first thing" || seen[1] != "the second thing" {
		t.Errorf("got %v", seen)
	}
	runner.Cancel(j.Id)
}

// "✓ worked" is a claim about the service, not about the steps. A deployment
// with no readiness check finished every step and could still be serving
// errors, so only a job that passed a step meant to prove it says so.
func TestOnlyAJobThatPassedItsCheckWorked(t *testing.T) {
	check := plan.Command("wait until it answers", "true")
	check.Proves = true

	cases := []struct {
		name string
		job  Snapshot
		want bool
	}{
		{"checked and done", Snapshot{Status: StatusDone, Plan: plan.New(check)}, true},
		{"done with nothing to check", Snapshot{Status: StatusDone, Plan: onestep()}, false},
		{"failed at its check", Snapshot{Status: StatusFailed, Plan: plan.New(check)}, false},
		{"interrupted before it was seen", Snapshot{Status: StatusInterrupted, Plan: plan.New(check)}, false},
	}
	for _, c := range cases {
		if got := c.job.Verified(); got != c.want {
			t.Errorf("%s: verified=%v, want %v", c.name, got, c.want)
		}
	}
}
