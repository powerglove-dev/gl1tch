package pipeline

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/8op-org/gl1tch/internal/executor"
)

// StepThroughSession is a stateful wrapper around a pipeline Run that pauses
// after each step so a UI (the Wails desktop app) can present the output,
// collect an action (accept / edit the output / abort / save-as), and resume.
//
// The session runs the pipeline in a goroutine with WithStepInterceptor and
// WithMaxParallel(1). The interceptor blocks on a per-session decision
// channel; the caller drives the session by calling Accept, EditOutput, or
// Abort after it has received a StepPausedEvent on the Events channel.
//
// Lifecycle:
//
//  1. NewStepThroughSession(pipeline, mgr) — creates the session, ID is
//     generated but execution has not started.
//  2. session.Start(ctx, userInput, opts...) — spawns the runner goroutine.
//  3. loop: read from session.Events; on StepPausedEvent, call Accept /
//     EditOutput / Abort; on StepFinalEvent, the run is done.
//  4. session.SaveAs(dir, name) — serialize the pipeline to
//     <dir>/<name>.workflow.yaml. The router's on-demand discover+embed path
//     picks it up automatically on the next ask.
//
// v1 scope: supports linear-chain pipelines (one ready root plus a chain of
// dependents). Parallel sibling steps run serially under forceMaxParallel=1
// but will complete without per-sibling pause. Rewind and prompt re-run are
// intentionally deferred — they require WithResumeFrom + checkpoint overrides
// that add meaningful complexity and are not in the minimum bar the user
// signed off on.
type StepThroughSession struct {
	ID       string
	pipeline *Pipeline
	mgr      *executor.Manager

	// Events is how the UI learns about step progress. The session closes
	// this channel when the run terminates (success, error, or abort).
	Events chan StepThroughEvent

	mu         sync.Mutex
	state      stepThroughState
	currentStep string
	outputs    map[string]string          // step ID → final (possibly edited) value
	edits      map[string]StepEditMark    // step ID → provenance for hand-edits
	runErr     error

	// decisionCh carries the caller's response out of the interceptor so the
	// runner can resume. Exactly one decision per pause.
	decisionCh chan StepDecision

	cancelRun context.CancelFunc
	done      chan struct{}
}

type stepThroughState int

const (
	stateIdle stepThroughState = iota
	stateRunning
	statePaused
	stateFinished
	stateAborted
	stateFailed
)

// StepEditMark records that a step's captured output was hand-edited in a
// step-through session. It is stored in the session (not in the saved YAML)
// so the UI can surface provenance for a given run.
type StepEditMark struct {
	Kind           string // "hand_edit"
	OriginalOutput string
	EditedOutput   string
	EditedAt       time.Time
}

// StepThroughEvent is the union of messages the session sends to the UI. It
// is intentionally a struct with an explicit Kind so Wails can marshal it
// cleanly to the React frontend via runtime.EventsEmit.
type StepThroughEvent struct {
	Kind      string `json:"kind"` // "step_started" | "step_paused" | "step_committed" | "final" | "error"
	SessionID string `json:"session_id"`
	StepID    string `json:"step_id,omitempty"`
	Output    string `json:"output,omitempty"`
	// StepIndex / StepTotal are 0-based / total step counts populated on
	// step_paused events so the UI can render "step N of M" and decide
	// whether the next click is "Continue" (more steps remain) or
	// "Finish" (this is the last pause). Without these, users see the
	// pause panel after the chain has visibly produced output and
	// reasonably assume the run is done — then accepting kicks off
	// another step they didn't expect. The screenshot that flagged this
	// was a 2-step chain paused on step-1 with both steps' output
	// already streamed into the chat surface.
	StepIndex   int    `json:"step_index"`
	StepTotal   int    `json:"step_total"`
	FinalOutput string `json:"final_output,omitempty"`
	Error       string `json:"error,omitempty"`
}

// NewStepThroughSession constructs an idle session bound to the given
// pipeline and executor manager. Call Start to begin execution.
func NewStepThroughSession(p *Pipeline, mgr *executor.Manager) *StepThroughSession {
	return &StepThroughSession{
		ID:         newSessionID(),
		pipeline:   p,
		mgr:        mgr,
		Events:     make(chan StepThroughEvent, 16),
		state:      stateIdle,
		outputs:    make(map[string]string),
		edits:      make(map[string]StepEditMark),
		decisionCh: make(chan StepDecision, 1),
		done:       make(chan struct{}),
	}
}

// Start kicks off the pipeline run in a background goroutine. Additional
// RunOptions may be passed for store / event publisher / brain injection
// wiring; WithStepInterceptor and WithMaxParallel(1) are always applied and
// will override any conflicting options the caller passes.
func (s *StepThroughSession) Start(ctx context.Context, userInput string, opts ...RunOption) error {
	s.mu.Lock()
	if s.state != stateIdle {
		s.mu.Unlock()
		return errors.New("step-through: session already started")
	}
	s.state = stateRunning
	runCtx, cancel := context.WithCancel(ctx)
	s.cancelRun = cancel
	s.mu.Unlock()

	// Append the interceptor + force-serial options last so they win.
	opts = append(opts,
		WithStepInterceptor(s.intercept),
		WithMaxParallel(1),
	)

	go func() {
		defer close(s.done)
		defer close(s.Events)

		final, err := Run(runCtx, s.pipeline, s.mgr, userInput, opts...)

		s.mu.Lock()
		s.runErr = err
		switch {
		case err != nil && s.state == stateAborted:
			// Aborted by user: already emitted, just return.
		case err != nil:
			s.state = stateFailed
			s.emitLocked(StepThroughEvent{
				Kind:      "error",
				SessionID: s.ID,
				Error:     err.Error(),
			})
		default:
			s.state = stateFinished
			s.emitLocked(StepThroughEvent{
				Kind:        "final",
				SessionID:   s.ID,
				FinalOutput: final,
			})
		}
		s.mu.Unlock()
	}()
	return nil
}

// intercept is the StepInterceptor callback passed to the runner. It runs on
// the runner's main drain loop goroutine. It publishes a step_paused event
// and blocks on decisionCh until the caller responds.
func (s *StepThroughSession) intercept(ctx context.Context, stepID string, output map[string]any) (StepDecision, error) {
	outStr := ""
	if output != nil {
		if v, ok := output["value"]; ok {
			outStr = fmt.Sprint(v)
		}
	}

	// Look up where this step sits in the pipeline so the UI can render
	// progress + decide whether the next button should say "Continue" or
	// "Finish". 0-based index; total counts every declared step including
	// non-plugin ones because that matches what the user sees in the
	// chain bar. If we can't find the step (shouldn't happen — the runner
	// only invokes the interceptor for steps it owns) we fall back to
	// 0/total so the UI just shows progress conservatively.
	stepIndex := 0
	stepTotal := 0
	if s.pipeline != nil {
		stepTotal = len(s.pipeline.Steps)
		for i, st := range s.pipeline.Steps {
			if st.ID == stepID {
				stepIndex = i
				break
			}
		}
	}

	s.mu.Lock()
	s.state = statePaused
	s.currentStep = stepID
	s.outputs[stepID] = outStr
	s.emitLocked(StepThroughEvent{
		Kind:      "step_paused",
		SessionID: s.ID,
		StepID:    stepID,
		Output:    outStr,
		StepIndex: stepIndex,
		StepTotal: stepTotal,
	})
	s.mu.Unlock()

	// Block until caller decides. Context cancellation unblocks as an abort.
	select {
	case decision := <-s.decisionCh:
		s.mu.Lock()
		if decision.Action == "edit_output" {
			s.outputs[stepID] = decision.EditedOutput
			s.edits[stepID] = StepEditMark{
				Kind:           "hand_edit",
				OriginalOutput: outStr,
				EditedOutput:   decision.EditedOutput,
				EditedAt:       time.Now(),
			}
		}
		// Don't transition out of stateAborted: Abort() owns that state and
		// the Start goroutine relies on observing it to distinguish a
		// user-driven abort from a runner-side error.
		if s.state != stateAborted && decision.Action != "abort" {
			s.state = stateRunning
			s.emitLocked(StepThroughEvent{
				Kind:      "step_committed",
				SessionID: s.ID,
				StepID:    stepID,
			})
		}
		s.mu.Unlock()
		return decision, nil
	case <-ctx.Done():
		return StepDecision{Action: "abort"}, nil
	}
}

// Accept tells the session to continue past the current paused step,
// leaving the step's output unchanged.
func (s *StepThroughSession) Accept() error {
	return s.sendDecision(StepDecision{Action: "continue"})
}

// EditOutput replaces the paused step's "value" output with editedValue and
// continues. The edit is recorded in the session's provenance map.
func (s *StepThroughSession) EditOutput(editedValue string) error {
	return s.sendDecision(StepDecision{Action: "edit_output", EditedOutput: editedValue})
}

// Abort cancels the run. The runner will cancel its context and the Start
// goroutine will emit a final error event and close the Events channel.
func (s *StepThroughSession) Abort() error {
	s.mu.Lock()
	if s.state != stateRunning && s.state != statePaused {
		s.mu.Unlock()
		return errors.New("step-through: session is not active")
	}
	s.state = stateAborted
	cancel := s.cancelRun
	s.mu.Unlock()

	// Unblock any pending interceptor pause with an abort decision. Best
	// effort — if no pause is pending the buffered channel still holds it.
	select {
	case s.decisionCh <- StepDecision{Action: "abort"}:
	default:
	}
	if cancel != nil {
		cancel()
	}
	return nil
}

// Wait blocks until the session has fully terminated. Useful for tests.
func (s *StepThroughSession) Wait() {
	<-s.done
}

// Snapshot returns a copy of the session's current state for the UI.
func (s *StepThroughSession) Snapshot() StepThroughSnapshot {
	s.mu.Lock()
	defer s.mu.Unlock()
	outputs := make(map[string]string, len(s.outputs))
	for k, v := range s.outputs {
		outputs[k] = v
	}
	editedSteps := make([]string, 0, len(s.edits))
	for k := range s.edits {
		editedSteps = append(editedSteps, k)
	}
	return StepThroughSnapshot{
		ID:          s.ID,
		State:       stateName(s.state),
		CurrentStep: s.currentStep,
		Outputs:     outputs,
		EditedSteps: editedSteps,
	}
}

// StepThroughSnapshot is the externally visible state of a session — what the
// Wails app surfaces to the React frontend on demand.
type StepThroughSnapshot struct {
	ID          string            `json:"id"`
	State       string            `json:"state"`
	CurrentStep string            `json:"current_step,omitempty"`
	Outputs     map[string]string `json:"outputs"`
	EditedSteps []string          `json:"edited_steps"`
}

func (s *StepThroughSession) sendDecision(d StepDecision) error {
	s.mu.Lock()
	if s.state != statePaused {
		s.mu.Unlock()
		return errors.New("step-through: session is not paused")
	}
	s.mu.Unlock()
	select {
	case s.decisionCh <- d:
		return nil
	default:
		return errors.New("step-through: decision channel full (duplicate resume?)")
	}
}

// emitLocked assumes s.mu is held by the caller. It publishes an event to
// the Events channel without blocking — if the UI has fallen behind and the
// buffer is full the event is dropped. The UI should drain quickly; dropped
// events are considered a UI bug, not a runner concern.
func (s *StepThroughSession) emitLocked(ev StepThroughEvent) {
	select {
	case s.Events <- ev:
	default:
	}
}

func stateName(st stepThroughState) string {
	switch st {
	case stateIdle:
		return "idle"
	case stateRunning:
		return "running"
	case statePaused:
		return "paused"
	case stateFinished:
		return "finished"
	case stateAborted:
		return "aborted"
	case stateFailed:
		return "failed"
	default:
		return "unknown"
	}
}

func newSessionID() string {
	var b [8]byte
	_, _ = rand.Read(b[:])
	return "st_" + hex.EncodeToString(b[:])
}
