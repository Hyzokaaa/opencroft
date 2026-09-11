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
	runner := NewRunner(counter())
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
	runner := NewRunner(counter())

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
	runner := NewRunner(counter())
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
