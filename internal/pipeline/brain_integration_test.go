package pipeline_test

import (
	"context"
	"io"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/powerglove-dev/gl1tch/internal/executor"
	"github.com/powerglove-dev/gl1tch/internal/pipeline"
	"github.com/powerglove-dev/gl1tch/internal/store"
)

// openTestStore creates a fresh SQLite store in a temp directory.
func openTestStore(t *testing.T) *store.Store {
	t.Helper()
	s, err := store.OpenAt(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("openTestStore: %v", err)
	}
	return s
}

// capturePlugin registers a plugin that writes `output` to the writer and
// records its promptOrInput in the provided *string pointer.
func capturePlugin(t *testing.T, mgr *executor.Manager, name, output string, captured *string) {
	t.Helper()
	if err := mgr.Register(&executor.StubExecutor{
		ExecutorName: name,
		ExecuteFn: func(_ context.Context, input string, _ map[string]string, w io.Writer) error {
			if captured != nil {
				*captured = input
			}
			_, err := w.Write([]byte(output))
			return err
		},
	}); err != nil {
		t.Fatalf("Register %q: %v", name, err)
	}
}

// TestBrainReadInjection_PreamblePresent verifies that when a StoreBrainInjector is
// configured, the plugin receives a prompt that starts with the brain preamble.
// Brain is always on — no use_brain flag needed.
func TestBrainReadInjection_PreamblePresent(t *testing.T) {
	s := openTestStore(t)
	mgr := executor.NewManager()
	var captured string
	capturePlugin(t, mgr, "echo", "result", &captured)

	p := &pipeline.Pipeline{
		Name: "brain-read-test",
		Steps: []pipeline.Step{
			{ID: "s1", Executor: "echo", Prompt: "hello"},
		},
	}

	_, err := pipeline.Run(context.Background(), p, mgr, "",
		pipeline.WithRunStore(s),
		pipeline.WithBrainInjector(pipeline.NewStoreBrainInjector(s)),
	)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	if !strings.Contains(captured, "## GLITCH Database Context") {
		t.Errorf("expected brain preamble in prompt, got: %q", captured)
	}
	if !strings.HasPrefix(strings.TrimSpace(captured), "## GLITCH Database Context") {
		t.Errorf("expected preamble at start, got: %q", captured[:min(len(captured), 100)])
	}
}

// TestBrainReadInjection_AbsentWithoutInjector verifies that when no BrainInjector
// is provided, the prompt does NOT get the database context preamble but DOES get
// the brain write instruction appended (brain is always on).
func TestBrainReadInjection_AbsentWithoutInjector(t *testing.T) {
	mgr := executor.NewManager()
	var captured string
	capturePlugin(t, mgr, "echo", "result", &captured)

	p := &pipeline.Pipeline{
		Name: "brain-no-injector-test",
		Steps: []pipeline.Step{
			{ID: "s1", Executor: "echo", Prompt: "just the prompt"},
		},
	}

	// No WithBrainInjector option.
	_, err := pipeline.Run(context.Background(), p, mgr, "")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	if strings.Contains(captured, "## GLITCH Database Context") {
		t.Errorf("unexpected brain preamble in prompt: %q", captured)
	}
	if !strings.HasPrefix(captured, "just the prompt") {
		t.Errorf("expected prompt to start with original text, got: %q", captured)
	}
	if !strings.Contains(captured, "BRAIN NOTE INSTRUCTION") {
		t.Errorf("expected brain write instruction in prompt, got: %q", captured)
	}
}

// TestBrainWriteInsertion_NoteInserted verifies that a write_brain step whose
// output contains a <brain> block persists a note in the store.
func TestBrainWriteInsertion_NoteInserted(t *testing.T) {
	s := openTestStore(t)
	mgr := executor.NewManager()
	brainOutput := `I analyzed the data. <brain_notes>Key insight: X is important</brain_notes> Done.`
	capturePlugin(t, mgr, "writer", brainOutput, nil)

	p := &pipeline.Pipeline{
		Name:       "brain-write-test",
		WriteBrain: true,
		Steps: []pipeline.Step{
			{ID: "write-step", Executor: "writer", Prompt: "analyze this"},
		},
	}

	_, err := pipeline.Run(context.Background(), p, mgr, "",
		pipeline.WithRunStore(s),
		pipeline.WithBrainInjector(pipeline.NewStoreBrainInjector(s)),
	)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	// Retrieve the run ID by listing recent brain notes (run_id = 1 for first run).
	ctx := context.Background()
	notes, err := s.RecentBrainNotes(ctx, 1, 10)
	if err != nil {
		t.Fatalf("RecentBrainNotes: %v", err)
	}
	if len(notes) == 0 {
		t.Fatal("expected at least one brain note, got none")
	}

	note := notes[0]
	if note.StepID != "write-step" {
		t.Errorf("StepID: want %q, got %q", "write-step", note.StepID)
	}
	if note.Body != "Key insight: X is important" {
		t.Errorf("Body: want %q, got %q", "Key insight: X is important", note.Body)
	}
	if note.Tags != "" {
		t.Errorf("Tags: want empty, got %q", note.Tags)
	}
}

// TestBrainFeedbackLoop verifies that a step A that writes a brain note
// causes step B (needs A) to receive that note in its preamble (brain always on).
func TestBrainFeedbackLoop(t *testing.T) {
	s := openTestStore(t)
	mgr := executor.NewManager()

	// Step A: outputs a brain note.
	writerOutput := `Analysis complete. <brain_notes>Loop insight from step A</brain_notes>`
	if err := mgr.Register(&executor.StubExecutor{
		ExecutorName: "writer",
		ExecuteFn: func(_ context.Context, _ string, _ map[string]string, w io.Writer) error {
			_, err := w.Write([]byte(writerOutput))
			return err
		},
	}); err != nil {
		t.Fatalf("Register writer: %v", err)
	}

	// Step B: captures its prompt to verify preamble injection.
	var capturedB string
	if err := mgr.Register(&executor.StubExecutor{
		ExecutorName: "reader",
		ExecuteFn: func(_ context.Context, input string, _ map[string]string, w io.Writer) error {
			capturedB = input
			_, err := w.Write([]byte("read done"))
			return err
		},
	}); err != nil {
		t.Fatalf("Register reader: %v", err)
	}

	trueVal := true
	p := &pipeline.Pipeline{
		Name: "brain-feedback-loop",
		Steps: []pipeline.Step{
			{
				ID:         "step-a",
				Executor:     "writer",
				Prompt:     "write something",
				WriteBrain: &trueVal,
			},
			{
				ID:     "step-b",
				Executor: "reader",
				Prompt: "read something",
				Needs:  []string{"step-a"},
			},
		},
	}

	_, err := pipeline.Run(context.Background(), p, mgr, "",
		pipeline.WithRunStore(s),
		pipeline.WithBrainInjector(pipeline.NewStoreBrainInjector(s)),
	)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	if !strings.Contains(capturedB, "Loop insight from step A") {
		t.Errorf("expected brain note in step-b preamble, got: %q", capturedB)
	}
	if !strings.Contains(capturedB, "## GLITCH Database Context") {
		t.Errorf("expected preamble header in step-b prompt, got: %q", capturedB)
	}
}

// TestBrainRunIsolation verifies that brain notes from a DIFFERENT run_id are
// not injected into the current run's preamble.
func TestBrainRunIsolation(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()

	// Manually insert a brain note with a different run_id (999).
	_, err := s.InsertBrainNote(ctx, store.BrainNote{
		RunID:     999,
		StepID:    "other-step",
		CreatedAt: time.Now().UnixMilli(),
		Tags:      "",
		Body:      "SECRET from other run",
	})
	if err != nil {
		t.Fatalf("InsertBrainNote: %v", err)
	}

	mgr := executor.NewManager()
	var captured string
	capturePlugin(t, mgr, "echo", "result", &captured)

	p := &pipeline.Pipeline{
		Name: "brain-isolation-test",
		Steps: []pipeline.Step{
			{ID: "s1", Executor: "echo", Prompt: "hello"},
		},
	}

	_, err = pipeline.Run(context.Background(), p, mgr, "",
		pipeline.WithRunStore(s),
		pipeline.WithBrainInjector(pipeline.NewStoreBrainInjector(s)),
	)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	if strings.Contains(captured, "SECRET from other run") {
		t.Errorf("brain note from other run leaked into current run's preamble: %q", captured)
	}
	// Should still have the preamble header (schema only, no notes from this run).
	if !strings.Contains(captured, "## GLITCH Database Context") {
		t.Errorf("expected preamble header, got: %q", captured)
	}
}

// TestBrainWriteInsertion_NoBrainBlock verifies that when a write_brain step
// emits output with no <brain> block, the step succeeds and no note is stored.
func TestBrainWriteInsertion_NoBrainBlock(t *testing.T) {
	s := openTestStore(t)
	mgr := executor.NewManager()
	capturePlugin(t, mgr, "emit-text", "just some text, no brain block here", nil)

	p := &pipeline.Pipeline{
		Name:       "no-brain-block",
		WriteBrain: true,
		Steps: []pipeline.Step{
			{ID: "s1", Executor: "emit-text"},
		},
	}

	_, err := pipeline.Run(context.Background(), p, mgr, "",
		pipeline.WithRunStore(s),
		pipeline.WithBrainInjector(pipeline.NewStoreBrainInjector(s)),
	)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	notes, err := s.RecentBrainNotes(context.Background(), 1, 10)
	if err != nil {
		t.Fatalf("RecentBrainNotes: %v", err)
	}
	if len(notes) != 0 {
		t.Errorf("expected 0 brain notes, got %d", len(notes))
	}
}

// TestBrainWriteInsertion_MalformedBrainBlock verifies that when a write_brain
// step emits malformed <brain> XML, the step succeeds and no note is stored.
func TestBrainWriteInsertion_MalformedBrainBlock(t *testing.T) {
	s := openTestStore(t)
	mgr := executor.NewManager()
	capturePlugin(t, mgr, "emit-bad", "<brain unclosed content", nil)

	p := &pipeline.Pipeline{
		Name:       "malformed-brain-block",
		WriteBrain: true,
		Steps: []pipeline.Step{
			{ID: "s1", Executor: "emit-bad"},
		},
	}

	_, err := pipeline.Run(context.Background(), p, mgr, "",
		pipeline.WithRunStore(s),
		pipeline.WithBrainInjector(pipeline.NewStoreBrainInjector(s)),
	)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	notes, err := s.RecentBrainNotes(context.Background(), 1, 10)
	if err != nil {
		t.Fatalf("RecentBrainNotes: %v", err)
	}
	if len(notes) != 0 {
		t.Errorf("expected 0 brain notes, got %d", len(notes))
	}
}

// TestBrainWriteInsertion_NoTagsAttribute verifies that a <brain> block with no
// tags attribute is stored with Tags == "".
func TestBrainWriteInsertion_NoTagsAttribute(t *testing.T) {
	s := openTestStore(t)
	mgr := executor.NewManager()
	capturePlugin(t, mgr, "emit-notags", "<brain_notes>plain note with no tags</brain_notes>", nil)

	p := &pipeline.Pipeline{
		Name:       "no-tags-brain-block",
		WriteBrain: true,
		Steps: []pipeline.Step{
			{ID: "s1", Executor: "emit-notags"},
		},
	}

	_, err := pipeline.Run(context.Background(), p, mgr, "",
		pipeline.WithRunStore(s),
		pipeline.WithBrainInjector(pipeline.NewStoreBrainInjector(s)),
	)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	notes, err := s.RecentBrainNotes(context.Background(), 1, 10)
	if err != nil {
		t.Fatalf("RecentBrainNotes: %v", err)
	}
	if len(notes) == 0 {
		t.Fatal("expected at least one brain note, got none")
	}
	note := notes[0]
	if note.Tags != "" {
		t.Errorf("Tags: want %q, got %q", "", note.Tags)
	}
	if note.Body != "plain note with no tags" {
		t.Errorf("Body: want %q, got %q", "plain note with no tags", note.Body)
	}
}

// TestBrainWriteInsertion_DAGPath verifies that a write_brain step executed via
// the DAG runner (step has Needs) persists a note with the correct fields.
func TestBrainWriteInsertion_DAGPath(t *testing.T) {
	s := openTestStore(t)
	mgr := executor.NewManager()

	// Step A: plain step that just succeeds.
	capturePlugin(t, mgr, "noop", "ok", nil)

	// Step B: depends on A, emits a brain block.
	trueVal := true
	if err := mgr.Register(&executor.StubExecutor{
		ExecutorName: "dag-writer",
		ExecuteFn: func(_ context.Context, _ string, _ map[string]string, w io.Writer) error {
			_, err := w.Write([]byte(`<brain_notes>insight from dag</brain_notes>`))
			return err
		},
	}); err != nil {
		t.Fatalf("Register dag-writer: %v", err)
	}

	p := &pipeline.Pipeline{
		Name: "dag-brain-write",
		Steps: []pipeline.Step{
			{ID: "step-a", Executor: "noop"},
			{
				ID:         "step-b",
				Executor:     "dag-writer",
				Needs:      []string{"step-a"},
				WriteBrain: &trueVal,
			},
		},
	}

	_, err := pipeline.Run(context.Background(), p, mgr, "",
		pipeline.WithRunStore(s),
		pipeline.WithBrainInjector(pipeline.NewStoreBrainInjector(s)),
	)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	notes, err := s.RecentBrainNotes(context.Background(), 1, 10)
	if err != nil {
		t.Fatalf("RecentBrainNotes: %v", err)
	}
	if len(notes) == 0 {
		t.Fatal("expected at least one brain note, got none")
	}
	note := notes[0]
	if note.Body != "insight from dag" {
		t.Errorf("Body: want %q, got %q", "insight from dag", note.Body)
	}
	if note.Tags != "" {
		t.Errorf("Tags: want empty, got %q", note.Tags)
	}
}

// TestBrainWriteInsertion_NoStoreConfigured verifies that a write_brain step runs
// without panic or error when no store is configured (ec.DB() == nil).
func TestBrainWriteInsertion_NoStoreConfigured(t *testing.T) {
	// write_brain: true step runs without WithRunStore
	// Assert: no panic, step succeeds, no error returned
	p := &pipeline.Pipeline{
		Name:       "no-store",
		WriteBrain: true,
		Steps: []pipeline.Step{
			{ID: "s1", Executor: "emit-brain"},
		},
	}
	mgr := executor.NewManager()
	_ = mgr.Register(&executor.StubExecutor{
		ExecutorName: "emit-brain",
		ExecuteFn: func(_ context.Context, _ string, _ map[string]string, w io.Writer) error {
			_, err := w.Write([]byte(`<brain_notes>note</brain_notes>`))
			return err
		},
	})
	// No WithRunStore — ec.DB() will be nil
	_, err := pipeline.Run(context.Background(), p, mgr, "")
	if err != nil {
		t.Fatalf("expected no error when store not configured, got %v", err)
	}
}

// TestBrainLegacyPath verifies that brain injection works on the legacy runner
// (pipelines without `needs`, `retry`, `for_each`, or `on_failure`).
// Brain is always on — no use_brain flag needed.
func TestBrainLegacyPath(t *testing.T) {
	s := openTestStore(t)
	mgr := executor.NewManager()
	var captured string
	capturePlugin(t, mgr, "echo", "result", &captured)

	// No Needs fields → isLegacyPipeline returns true.
	p := &pipeline.Pipeline{
		Name: "brain-legacy-test",
		Steps: []pipeline.Step{
			{ID: "s1", Executor: "echo", Prompt: "legacy hello"},
		},
	}

	_, err := pipeline.Run(context.Background(), p, mgr, "",
		pipeline.WithRunStore(s),
		pipeline.WithBrainInjector(pipeline.NewStoreBrainInjector(s)),
	)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	if !strings.Contains(captured, "## GLITCH Database Context") {
		t.Errorf("expected brain preamble in legacy path prompt, got: %q", captured)
	}
}
