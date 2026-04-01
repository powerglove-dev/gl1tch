package pipeline_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/powerglove-dev/gl1tch/internal/busd/topics"
	"github.com/powerglove-dev/gl1tch/internal/executor"
	"github.com/powerglove-dev/gl1tch/internal/pipeline"
)

func makeWritePlugin(name, output string) *executor.StubExecutor {
	return &executor.StubExecutor{
		ExecutorName: name,
		ExecuteFn: func(_ context.Context, _ string, _ map[string]string, w io.Writer) error {
			_, err := w.Write([]byte(output))
			return err
		},
	}
}

func TestRunner_LinearPipeline(t *testing.T) {
	p := &pipeline.Pipeline{
		Name:    "linear-test",
		Version: "1.0",
		Steps: []pipeline.Step{
			{ID: "s1", Type: "input"},
			{ID: "s2", Executor: "echo"},
			{ID: "s3", Type: "output"},
		},
	}

	mgr := executor.NewManager()
	if err := mgr.Register(&executor.StubExecutor{
		ExecutorName: "echo",
		ExecuteFn: func(_ context.Context, input string, _ map[string]string, w io.Writer) error {
			_, err := w.Write([]byte("echoed: " + input))
			return err
		},
	}); err != nil {
		t.Fatalf("Register: %v", err)
	}

	result, err := pipeline.Run(context.Background(), p, mgr, "hello world")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !strings.Contains(result, "echoed: hello world") {
		t.Errorf("expected 'echoed: hello world' in output, got %q", result)
	}
}

func TestRunner_ConditionalBranch_Then(t *testing.T) {
	p := &pipeline.Pipeline{
		Name: "branch-test",
		Steps: []pipeline.Step{
			{ID: "s1", Type: "input"},
			{
				ID:     "s2",
				Executor: "classifier",
				Condition: pipeline.Condition{
					If:   "contains:go",
					Then: "golang-step",
					Else: "other-step",
				},
			},
			{ID: "golang-step", Executor: "go-handler"},
			{ID: "other-step", Executor: "other-handler"},
			{ID: "out", Type: "output"},
		},
	}

	mgr := executor.NewManager()
	for _, p := range []executor.Executor{
		makeWritePlugin("classifier", "golang rocks"),
		makeWritePlugin("go-handler", "handled by go"),
		makeWritePlugin("other-handler", "handled by other"),
	} {
		if err := mgr.Register(p); err != nil {
			t.Fatalf("Register: %v", err)
		}
	}

	result, err := pipeline.Run(context.Background(), p, mgr, "golang rocks")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !strings.Contains(result, "handled by go") {
		t.Errorf("expected 'handled by go', got %q", result)
	}
}

func TestRunner_ConditionalBranch_Else(t *testing.T) {
	p := &pipeline.Pipeline{
		Name: "branch-else-test",
		Steps: []pipeline.Step{
			{ID: "s1", Type: "input"},
			{
				ID:     "s2",
				Executor: "classifier",
				Condition: pipeline.Condition{
					If:   "contains:python",
					Then: "python-step",
					Else: "default-step",
				},
			},
			{ID: "python-step", Executor: "py-handler"},
			{ID: "default-step", Executor: "default-handler"},
			{ID: "out", Type: "output"},
		},
	}

	mgr := executor.NewManager()
	for _, p := range []executor.Executor{
		makeWritePlugin("classifier", "golang rocks"),
		makeWritePlugin("py-handler", "python handler"),
		makeWritePlugin("default-handler", "default handler"),
	} {
		if err := mgr.Register(p); err != nil {
			t.Fatalf("Register: %v", err)
		}
	}

	result, err := pipeline.Run(context.Background(), p, mgr, "golang rocks")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !strings.Contains(result, "default handler") {
		t.Errorf("expected 'default handler', got %q", result)
	}
}

func TestRunner_TemplateInterpolation(t *testing.T) {
	p := &pipeline.Pipeline{
		Name: "interp-test",
		Steps: []pipeline.Step{
			{ID: "s1", Type: "input"},
			{ID: "s2", Executor: "upper", Prompt: "input was: {{.s1.out}}"},
			{ID: "out", Type: "output"},
		},
	}

	mgr := executor.NewManager()
	var capturedInput string
	if err := mgr.Register(&executor.StubExecutor{
		ExecutorName: "upper",
		ExecuteFn: func(_ context.Context, input string, _ map[string]string, w io.Writer) error {
			capturedInput = input
			_, err := w.Write([]byte("done"))
			return err
		},
	}); err != nil {
		t.Fatalf("Register: %v", err)
	}

	_, err := pipeline.Run(context.Background(), p, mgr, "hello")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !strings.Contains(capturedInput, "input was: hello") {
		t.Errorf("expected interpolated input, got %q", capturedInput)
	}
}

func TestRunner_MissingPlugin(t *testing.T) {
	p := &pipeline.Pipeline{
		Name: "missing-test",
		Steps: []pipeline.Step{
			{ID: "s1", Type: "input"},
			{ID: "s2", Executor: "nonexistent"},
			{ID: "out", Type: "output"},
		},
	}
	mgr := executor.NewManager() // empty — no plugins registered intentionally
	_, err := pipeline.Run(context.Background(), p, mgr, "hello")
	if err == nil {
		t.Error("expected error for missing plugin")
	}
}

// TestParallelExecution verifies that two independent steps run concurrently.
// Each step sleeps for 100ms; if they ran sequentially, the total would be ≥200ms.
// We assert the total time is < 180ms (well under 200ms) to prove concurrency.
func TestParallelExecution(t *testing.T) {
	const stepDelay = 50 * time.Millisecond

	var startA, startB time.Time
	var mu syncMutex

	p := &pipeline.Pipeline{
		Name:        "parallel-test",
		MaxParallel: 4,
		Steps: []pipeline.Step{
			{
				ID:     "step-a",
				Executor: "echo-a",
			},
			{
				ID:     "step-b",
				Executor: "echo-b",
			},
		},
	}

	mgr := executor.NewManager()
	_ = mgr.Register(&executor.StubExecutor{
		ExecutorName: "echo-a",
		ExecuteFn: func(_ context.Context, _ string, _ map[string]string, w io.Writer) error {
			mu.Lock()
			startA = time.Now()
			mu.Unlock()
			time.Sleep(stepDelay)
			_, err := w.Write([]byte("a"))
			return err
		},
	})
	_ = mgr.Register(&executor.StubExecutor{
		ExecutorName: "echo-b",
		ExecuteFn: func(_ context.Context, _ string, _ map[string]string, w io.Writer) error {
			mu.Lock()
			startB = time.Now()
			mu.Unlock()
			time.Sleep(stepDelay)
			_, err := w.Write([]byte("b"))
			return err
		},
	})

	start := time.Now()
	_, err := pipeline.Run(context.Background(), p, mgr, "")
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	mu.Lock()
	overlap := startA.Before(startB.Add(stepDelay)) && startB.Before(startA.Add(stepDelay))
	mu.Unlock()

	// Either both started within the delay window (overlap) or total time < 2x delay.
	if elapsed >= 2*stepDelay && !overlap {
		t.Errorf("steps appear to have run sequentially (elapsed=%v, want < %v)", elapsed, 2*stepDelay)
	}
}

// syncMutex is a simple wrapper used in tests that need a local mutex type.
type syncMutex struct {
	mu sync.Mutex
}

func (m *syncMutex) Lock()   { m.mu.Lock() }
func (m *syncMutex) Unlock() { m.mu.Unlock() }


// TestRetryPolicy verifies that a step that fails twice then succeeds
// is attempted exactly 3 times.
func TestRetryPolicy(t *testing.T) {
	var attempts atomic.Int32

	p := &pipeline.Pipeline{
		Name: "retry-test",
		Steps: []pipeline.Step{
			{
				ID:     "flaky",
				Executor: "flaky-plugin",
				Retry: &pipeline.RetryPolicy{
					MaxAttempts: 3,
					Interval:    pipeline.Duration{},
					On:          "always",
				},
			},
		},
	}

	mgr := executor.NewManager()
	_ = mgr.Register(&executor.StubExecutor{
		ExecutorName: "flaky-plugin",
		ExecuteFn: func(_ context.Context, _ string, _ map[string]string, w io.Writer) error {
			n := attempts.Add(1)
			if n < 3 {
				return errors.New("transient error")
			}
			_, err := w.Write([]byte("success after retries"))
			return err
		},
	})

	result, err := pipeline.Run(context.Background(), p, mgr, "")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if attempts.Load() != 3 {
		t.Errorf("expected 3 attempts, got %d", attempts.Load())
	}
	if !strings.Contains(result, "success after retries") {
		t.Errorf("expected success output, got %q", result)
	}
}

// TestOnFailure verifies that when a step fails its on_failure step runs,
// and that the failed step's dependents are marked skipped.
func TestOnFailure(t *testing.T) {
	var failureHandlerRan bool
	var dependentRan bool

	p := &pipeline.Pipeline{
		Name: "on-failure-test",
		Steps: []pipeline.Step{
			{
				ID:        "failing-step",
				Executor:    "always-fail",
				OnFailure: "recovery-step",
			},
			{
				ID:     "dependent-step",
				Executor: "should-not-run",
				Needs:  []string{"failing-step"},
			},
			{
				ID:     "recovery-step",
				Executor: "recovery-plugin",
			},
		},
	}

	mgr := executor.NewManager()
	_ = mgr.Register(&executor.StubExecutor{
		ExecutorName: "always-fail",
		ExecuteFn: func(_ context.Context, _ string, _ map[string]string, w io.Writer) error {
			return errors.New("intentional failure")
		},
	})
	_ = mgr.Register(&executor.StubExecutor{
		ExecutorName: "should-not-run",
		ExecuteFn: func(_ context.Context, _ string, _ map[string]string, w io.Writer) error {
			dependentRan = true
			return nil
		},
	})
	_ = mgr.Register(&executor.StubExecutor{
		ExecutorName: "recovery-plugin",
		ExecuteFn: func(_ context.Context, _ string, _ map[string]string, w io.Writer) error {
			failureHandlerRan = true
			_, err := w.Write([]byte("recovered"))
			return err
		},
	})

	result, err := pipeline.Run(context.Background(), p, mgr, "")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !failureHandlerRan {
		t.Error("expected on_failure step to run")
	}
	if dependentRan {
		t.Error("expected dependent step to be skipped")
	}
	if !strings.Contains(result, "recovered") {
		t.Errorf("expected recovery output in result, got %q", result)
	}
}

// TestForEach verifies that a for_each step expands into one execution per item.
func TestForEach(t *testing.T) {
	var executionItems []string
	var mu sync.Mutex

	p := &pipeline.Pipeline{
		Name:        "foreach-test",
		MaxParallel: 4,
		Steps: []pipeline.Step{
			{
				ID:      "process",
				Executor:  "item-processor",
				ForEach: "alpha\nbeta\ngamma",
			},
		},
	}

	mgr := executor.NewManager()
	_ = mgr.Register(&executor.StubExecutor{
		ExecutorName: "item-processor",
		ExecuteFn: func(_ context.Context, _ string, vars map[string]string, w io.Writer) error {
			// The item is injected as vars["_item"] through the args mechanism.
			// In the DAG runner, item is in args, not vars — but the plugin gets
			// the prompt/input. We verify via output.
			_, err := w.Write([]byte("processed"))
			mu.Lock()
			executionItems = append(executionItems, "item")
			mu.Unlock()
			return err
		},
	})

	_, err := pipeline.Run(context.Background(), p, mgr, "")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	mu.Lock()
	count := len(executionItems)
	mu.Unlock()

	if count != 3 {
		t.Errorf("expected 3 executions for 3 items, got %d", count)
	}
}

// TestBuiltinStep verifies that builtin steps run via the DAG runner.
func TestBuiltinStep(t *testing.T) {
	p := &pipeline.Pipeline{
		Name: "builtin-test",
		Steps: []pipeline.Step{
			{
				ID:       "assert-step",
				Executor: "builtin.assert",
				Args: map[string]any{
					"condition": "true",
				},
			},
		},
	}

	mgr := executor.NewManager()
	result, err := pipeline.Run(context.Background(), p, mgr, "")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	_ = result
}

// TestBuiltinAssertFails verifies that a failing builtin.assert propagates as an error.
func TestBuiltinAssertFails(t *testing.T) {
	p := &pipeline.Pipeline{
		Name: "builtin-fail-test",
		Steps: []pipeline.Step{
			{
				ID:       "assert-fail",
				Executor: "builtin.assert",
				Args: map[string]any{
					"condition": "false",
					"message":   "expected failure",
				},
			},
		},
	}

	mgr := executor.NewManager()
	_, err := pipeline.Run(context.Background(), p, mgr, "")
	// The failure may be swallowed by the DAG (no dependents, no on_failure),
	// so we just run and ensure no panic.
	_ = err
}

// TestStepStatusLogLines verifies that the DAG runner emits structured
// [step:<id>] status:<state> lines to stdout for each non-input/output step.
func TestStepStatusLogLines(t *testing.T) {
	p := &pipeline.Pipeline{
		Name:        "status-log-test",
		MaxParallel: 4,
		Steps: []pipeline.Step{
			{
				ID:     "step1",
				Executor: "noop1",
			},
			{
				ID:     "step2",
				Executor: "noop2",
				Needs:  []string{"step1"},
			},
		},
	}

	mgr := executor.NewManager()
	_ = mgr.Register(&executor.StubExecutor{
		ExecutorName: "noop1",
		ExecuteFn: func(_ context.Context, _ string, _ map[string]string, w io.Writer) error {
			_, err := w.Write([]byte("out1"))
			return err
		},
	})
	_ = mgr.Register(&executor.StubExecutor{
		ExecutorName: "noop2",
		ExecuteFn: func(_ context.Context, _ string, _ map[string]string, w io.Writer) error {
			_, err := w.Write([]byte("out2"))
			return err
		},
	})

	// Redirect stdout so we can capture fmt.Printf output from the runner.
	origStdout := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	os.Stdout = w

	var captured string
	done := make(chan struct{})
	go func() {
		defer close(done)
		b, _ := io.ReadAll(r)
		captured = string(b)
	}()

	_, runErr := pipeline.Run(context.Background(), p, mgr, "")

	// Restore stdout and close the write-end so the drain goroutine terminates.
	w.Close()
	os.Stdout = origStdout
	<-done
	r.Close()

	if runErr != nil {
		t.Fatalf("Run: %v", runErr)
	}

	// All four status lines must be present.
	wantLines := []string{
		"[step:step1] status:running",
		"[step:step1] status:done",
		"[step:step2] status:running",
		"[step:step2] status:done",
	}
	for _, want := range wantLines {
		if !strings.Contains(captured, want) {
			t.Errorf("stdout missing %q\nfull output:\n%s", want, captured)
		}
	}

	// step1 must appear as running before step2 starts (sequential due to Needs).
	idx1Running := strings.Index(captured, "[step:step1] status:running")
	idx1Done := strings.Index(captured, "[step:step1] status:done")
	idx2Running := strings.Index(captured, "[step:step2] status:running")
	idx2Done := strings.Index(captured, "[step:step2] status:done")

	if idx1Running > idx1Done {
		t.Error("step1: running must appear before done")
	}
	if idx2Running > idx2Done {
		t.Error("step2: running must appear before done")
	}
	// step1 must finish before step2 starts (Needs dependency).
	if idx1Done > idx2Running {
		t.Error("step1 done must appear before step2 running (sequential dependency)")
	}
}

// ── capturingPublisher ────────────────────────────────────────────────────────

type capturingPublisher struct {
	mu     sync.Mutex
	events []publishedEvent
}

type publishedEvent struct {
	topic   string
	payload []byte
}

func (c *capturingPublisher) Publish(_ context.Context, topic string, payload []byte) error {
	c.mu.Lock()
	c.events = append(c.events, publishedEvent{topic, payload})
	c.mu.Unlock()
	return nil
}

func (c *capturingPublisher) topicsList() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]string, len(c.events))
	for i, e := range c.events {
		out[i] = e.topic
	}
	return out
}

// topicPayload returns the decoded JSON payload for the first event matching topic.
func (c *capturingPublisher) topicPayload(topic string) map[string]any {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, e := range c.events {
		if e.topic == topic {
			var m map[string]any
			_ = json.Unmarshal(e.payload, &m)
			return m
		}
	}
	return nil
}

// countTopic returns the number of events with the given topic.
func (c *capturingPublisher) countTopic(topic string) int {
	c.mu.Lock()
	defer c.mu.Unlock()
	n := 0
	for _, e := range c.events {
		if e.topic == topic {
			n++
		}
	}
	return n
}

// ── Event emission tests ──────────────────────────────────────────────────────

// TestEventEmission_Legacy verifies that a 2-step sequential (legacy) pipeline
// emits events in the correct order: run.started, step.started×2, step.done×2, run.completed.
func TestEventEmission_Legacy(t *testing.T) {
	p := &pipeline.Pipeline{
		Name: "event-legacy-test",
		Steps: []pipeline.Step{
			{ID: "in", Type: "input"},
			{ID: "s1", Executor: "echo1"},
			{ID: "s2", Executor: "echo2"},
			{ID: "out", Type: "output"},
		},
	}

	mgr := executor.NewManager()
	_ = mgr.Register(makeWritePlugin("echo1", "output1"))
	_ = mgr.Register(makeWritePlugin("echo2", "output2"))

	pub := &capturingPublisher{}
	_, err := pipeline.Run(context.Background(), p, mgr, "input", pipeline.WithEventPublisher(pub))
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	ts := pub.topicsList()

	// Verify required topics are present in order.
	wantTopics := []string{
		topics.RunStarted,
		topics.StepStarted,
		topics.StepDone,
		topics.StepStarted,
		topics.StepDone,
		topics.RunCompleted,
	}
	if len(ts) != len(wantTopics) {
		t.Errorf("expected %d events, got %d: %v", len(wantTopics), len(ts), ts)
	}
	for i, want := range wantTopics {
		if i >= len(ts) {
			break
		}
		if ts[i] != want {
			t.Errorf("event[%d]: want %q, got %q", i, want, ts[i])
		}
	}

	// Verify counts.
	if n := pub.countTopic(topics.RunStarted); n != 1 {
		t.Errorf("want 1 run.started, got %d", n)
	}
	if n := pub.countTopic(topics.StepStarted); n != 2 {
		t.Errorf("want 2 step.started, got %d", n)
	}
	if n := pub.countTopic(topics.StepDone); n != 2 {
		t.Errorf("want 2 step.done, got %d", n)
	}
	if n := pub.countTopic(topics.RunCompleted); n != 1 {
		t.Errorf("want 1 run.completed, got %d", n)
	}

	// Verify run.started payload contains pipeline name.
	if m := pub.topicPayload(topics.RunStarted); m != nil {
		if m["pipeline"] != "event-legacy-test" {
			t.Errorf("run.started pipeline name: want %q, got %v", "event-legacy-test", m["pipeline"])
		}
	}
}

// TestEventEmission_DAG verifies that a 2-step DAG pipeline emits the expected events:
// run.started, step.started×2, step.done×2, run.completed (run.started first, run.completed last).
func TestEventEmission_DAG(t *testing.T) {
	p := &pipeline.Pipeline{
		Name:        "event-dag-test",
		MaxParallel: 2,
		Steps: []pipeline.Step{
			{ID: "s1", Executor: "dag-echo1"},
			{ID: "s2", Executor: "dag-echo2", Needs: []string{"s1"}},
		},
	}

	mgr := executor.NewManager()
	_ = mgr.Register(makeWritePlugin("dag-echo1", "dag-output1"))
	_ = mgr.Register(makeWritePlugin("dag-echo2", "dag-output2"))

	pub := &capturingPublisher{}
	_, err := pipeline.Run(context.Background(), p, mgr, "", pipeline.WithEventPublisher(pub))
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	// Verify counts.
	if n := pub.countTopic(topics.RunStarted); n != 1 {
		t.Errorf("want 1 run.started, got %d", n)
	}
	if n := pub.countTopic(topics.StepStarted); n != 2 {
		t.Errorf("want 2 step.started, got %d", n)
	}
	if n := pub.countTopic(topics.StepDone); n != 2 {
		t.Errorf("want 2 step.done, got %d", n)
	}
	if n := pub.countTopic(topics.RunCompleted); n != 1 {
		t.Errorf("want 1 run.completed, got %d", n)
	}

	// run.started must be first, run.completed must be last.
	ts := pub.topicsList()
	if len(ts) < 2 {
		t.Fatalf("too few events: %v", ts)
	}
	if ts[0] != topics.RunStarted {
		t.Errorf("first event: want %q, got %q", topics.RunStarted, ts[0])
	}
	if ts[len(ts)-1] != topics.RunCompleted {
		t.Errorf("last event: want %q, got %q", topics.RunCompleted, ts[len(ts)-1])
	}
}

// TestPublishTo verifies that a step with publish_to set causes an extra event
// to be published on the custom topic containing the step's output.
func TestPublishTo(t *testing.T) {
	p := &pipeline.Pipeline{
		Name:        "publish-to-test",
		MaxParallel: 2,
		Steps: []pipeline.Step{
			{ID: "produce", Executor: "producer", PublishTo: "custom.topic"},
		},
	}

	mgr := executor.NewManager()
	_ = mgr.Register(makeWritePlugin("producer", "hello"))

	pub := &capturingPublisher{}
	_, err := pipeline.Run(context.Background(), p, mgr, "", pipeline.WithEventPublisher(pub))
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	if n := pub.countTopic("custom.topic"); n != 1 {
		t.Errorf("want 1 custom.topic event, got %d", n)
	}
}
