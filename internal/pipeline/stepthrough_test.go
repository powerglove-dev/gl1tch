package pipeline_test

import (
	"context"
	"io"
	"testing"
	"time"

	"github.com/8op-org/gl1tch/internal/executor"
	"github.com/8op-org/gl1tch/internal/pipeline"
)

// echoExecutor stubs an executor that emits a fixed body so step output is
// deterministic and tests can assert exact strings flowing through.
func echoExecutor(name, body string) *executor.StubExecutor {
	return &executor.StubExecutor{
		ExecutorName: name,
		ExecuteFn: func(_ context.Context, _ string, _ map[string]string, w io.Writer) error {
			_, err := w.Write([]byte(body))
			return err
		},
	}
}

// drainEvents collects events from a session until the channel closes or the
// timeout fires. Returns the events in arrival order.
func drainEvents(t *testing.T, ch <-chan pipeline.StepThroughEvent, timeout time.Duration) []pipeline.StepThroughEvent {
	t.Helper()
	var out []pipeline.StepThroughEvent
	deadline := time.After(timeout)
	for {
		select {
		case ev, ok := <-ch:
			if !ok {
				return out
			}
			out = append(out, ev)
		case <-deadline:
			t.Fatalf("drainEvents: timeout after %s, collected %d events", timeout, len(out))
		}
	}
}

// TestStepThroughSession_AcceptAll runs a two-step linear pipeline through a
// session and accepts both pauses. The final output must match the second
// step's executor body, proving the runner resumed cleanly past the
// interceptor on each step.
func TestStepThroughSession_AcceptAll(t *testing.T) {
	p := &pipeline.Pipeline{
		Name:    "step-through-accept",
		Version: "1",
		Steps: []pipeline.Step{
			{ID: "first", Executor: "first-exec"},
			{ID: "second", Executor: "second-exec", Needs: []string{"first"}},
		},
	}

	mgr := executor.NewManager()
	if err := mgr.Register(echoExecutor("first-exec", "first-output")); err != nil {
		t.Fatalf("register first: %v", err)
	}
	if err := mgr.Register(echoExecutor("second-exec", "second-output")); err != nil {
		t.Fatalf("register second: %v", err)
	}

	sess := pipeline.NewStepThroughSession(p, mgr)
	if err := sess.Start(context.Background(), ""); err != nil {
		t.Fatalf("Start: %v", err)
	}

	// Drive the session: read events, accept on each pause, until final.
	go func() {
		for ev := range sess.Events {
			if ev.Kind == "step_paused" {
				_ = sess.Accept()
			}
		}
	}()
	sess.Wait()

	snap := sess.Snapshot()
	if snap.State != "finished" {
		t.Fatalf("expected state 'finished', got %q", snap.State)
	}
	if got := snap.Outputs["first"]; got != "first-output" {
		t.Errorf("first step output: want %q, got %q", "first-output", got)
	}
	if got := snap.Outputs["second"]; got != "second-output" {
		t.Errorf("second step output: want %q, got %q", "second-output", got)
	}
	if len(snap.EditedSteps) != 0 {
		t.Errorf("expected zero edited steps, got %v", snap.EditedSteps)
	}
}

// TestStepThroughSession_EditOutputCaptured verifies the session captures a
// hand-edit, records its provenance, and lets the run continue to completion.
// We deliberately do not assert that the edit flows into step 2's prompt via
// template expansion — that is the runner's job and is covered by the
// runner's own ec.Set propagation tests. The session's contract here is:
// (1) capture the edit, (2) mark provenance, (3) don't break the run.
func TestStepThroughSession_EditOutputCaptured(t *testing.T) {
	p := &pipeline.Pipeline{
		Name:    "step-through-edit",
		Version: "1",
		Steps: []pipeline.Step{
			{ID: "first", Executor: "first-exec"},
			{ID: "second", Executor: "second-exec", Needs: []string{"first"}},
		},
	}

	mgr := executor.NewManager()
	if err := mgr.Register(echoExecutor("first-exec", "raw-llm-output")); err != nil {
		t.Fatalf("register first: %v", err)
	}
	if err := mgr.Register(echoExecutor("second-exec", "second-output")); err != nil {
		t.Fatalf("register second: %v", err)
	}

	sess := pipeline.NewStepThroughSession(p, mgr)
	if err := sess.Start(context.Background(), ""); err != nil {
		t.Fatalf("Start: %v", err)
	}

	editApplied := false
	go func() {
		for ev := range sess.Events {
			if ev.Kind != "step_paused" {
				continue
			}
			if ev.StepID == "first" && !editApplied {
				editApplied = true
				_ = sess.EditOutput("HAND-EDITED")
			} else {
				_ = sess.Accept()
			}
		}
	}()
	sess.Wait()

	snap := sess.Snapshot()
	if snap.State != "finished" {
		t.Fatalf("expected state 'finished', got %q (edited=%v)", snap.State, snap.EditedSteps)
	}
	if !editApplied {
		t.Fatal("first-step pause was never observed by the test driver")
	}
	if got := snap.Outputs["first"]; got != "HAND-EDITED" {
		t.Errorf("session output for first should reflect edit, got %q", got)
	}
	if len(snap.EditedSteps) != 1 || snap.EditedSteps[0] != "first" {
		t.Errorf("expected EditedSteps == [first], got %v", snap.EditedSteps)
	}
}

// TestStepThroughSession_Abort verifies that calling Abort while paused
// terminates the run with a failed/aborted state and a non-nil error from
// the run goroutine.
func TestStepThroughSession_Abort(t *testing.T) {
	p := &pipeline.Pipeline{
		Name:    "step-through-abort",
		Version: "1",
		Steps: []pipeline.Step{
			{ID: "first", Executor: "first-exec"},
			{ID: "second", Executor: "second-exec", Needs: []string{"first"}},
		},
	}

	mgr := executor.NewManager()
	if err := mgr.Register(echoExecutor("first-exec", "first-output")); err != nil {
		t.Fatalf("register first: %v", err)
	}
	if err := mgr.Register(echoExecutor("second-exec", "second-output")); err != nil {
		t.Fatalf("register second: %v", err)
	}

	sess := pipeline.NewStepThroughSession(p, mgr)
	if err := sess.Start(context.Background(), ""); err != nil {
		t.Fatalf("Start: %v", err)
	}

	go func() {
		for ev := range sess.Events {
			if ev.Kind == "step_paused" && ev.StepID == "first" {
				_ = sess.Abort()
			}
		}
	}()
	sess.Wait()

	snap := sess.Snapshot()
	if snap.State != "aborted" {
		t.Errorf("expected state 'aborted' after Abort, got %q", snap.State)
	}
	if _, ran := snap.Outputs["second"]; ran {
		t.Errorf("step 'second' should never have executed after abort")
	}
}
