package pipeline_test

import (
	"context"
	"io"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/adam-stokes/orcai/internal/pipeline"
	"github.com/adam-stokes/orcai/internal/plugin"
	"github.com/adam-stokes/orcai/internal/store"
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
func capturePlugin(t *testing.T, mgr *plugin.Manager, name, output string, captured *string) {
	t.Helper()
	if err := mgr.Register(&plugin.StubPlugin{
		PluginName: name,
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

// TestBrainReadInjection_PreamblePresent verifies that when use_brain is true
// and a StoreBrainInjector is configured, the plugin receives a prompt that
// starts with the brain preamble.
func TestBrainReadInjection_PreamblePresent(t *testing.T) {
	s := openTestStore(t)
	mgr := plugin.NewManager()
	var captured string
	capturePlugin(t, mgr, "echo", "result", &captured)

	p := &pipeline.Pipeline{
		Name:     "brain-read-test",
		UseBrain: true,
		Steps: []pipeline.Step{
			{ID: "s1", Plugin: "echo", Prompt: "hello"},
		},
	}

	_, err := pipeline.Run(context.Background(), p, mgr, "",
		pipeline.WithRunStore(s),
		pipeline.WithBrainInjector(pipeline.NewStoreBrainInjector(s)),
	)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	if !strings.Contains(captured, "## ORCAI Database Context") {
		t.Errorf("expected brain preamble in prompt, got: %q", captured)
	}
	if !strings.HasPrefix(strings.TrimSpace(captured), "## ORCAI Database Context") {
		t.Errorf("expected preamble at start, got: %q", captured[:min(len(captured), 100)])
	}
}

// TestBrainReadInjection_AbsentWithoutInjector verifies that when no injector
// is provided, the prompt is NOT prefixed with brain preamble even if use_brain: true.
func TestBrainReadInjection_AbsentWithoutInjector(t *testing.T) {
	mgr := plugin.NewManager()
	var captured string
	capturePlugin(t, mgr, "echo", "result", &captured)

	p := &pipeline.Pipeline{
		Name:     "brain-no-injector-test",
		UseBrain: true,
		Steps: []pipeline.Step{
			{ID: "s1", Plugin: "echo", Prompt: "just the prompt"},
		},
	}

	// No WithBrainInjector option.
	_, err := pipeline.Run(context.Background(), p, mgr, "")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	if strings.Contains(captured, "## ORCAI Database Context") {
		t.Errorf("unexpected brain preamble in prompt: %q", captured)
	}
	if captured != "just the prompt" {
		t.Errorf("expected prompt unchanged, got: %q", captured)
	}
}

// TestBrainReadInjection_AbsentWhenFlagOff verifies that even with an injector
// configured, if use_brain is false the preamble is NOT added.
func TestBrainReadInjection_AbsentWhenFlagOff(t *testing.T) {
	s := openTestStore(t)
	mgr := plugin.NewManager()
	var captured string
	capturePlugin(t, mgr, "echo", "result", &captured)

	p := &pipeline.Pipeline{
		Name:     "brain-off-test",
		UseBrain: false, // explicitly off
		Steps: []pipeline.Step{
			{ID: "s1", Plugin: "echo", Prompt: "plain prompt"},
		},
	}

	_, err := pipeline.Run(context.Background(), p, mgr, "",
		pipeline.WithRunStore(s),
		pipeline.WithBrainInjector(pipeline.NewStoreBrainInjector(s)),
	)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	if strings.Contains(captured, "## ORCAI Database Context") {
		t.Errorf("unexpected brain preamble in prompt: %q", captured)
	}
	if captured != "plain prompt" {
		t.Errorf("expected prompt unchanged, got: %q", captured)
	}
}

// TestBrainWriteInsertion_NoteInserted verifies that a write_brain step whose
// output contains a <brain> block persists a note in the store.
func TestBrainWriteInsertion_NoteInserted(t *testing.T) {
	s := openTestStore(t)
	mgr := plugin.NewManager()
	brainOutput := `I analyzed the data. <brain tags="test,analysis">Key insight: X is important</brain> Done.`
	capturePlugin(t, mgr, "writer", brainOutput, nil)

	p := &pipeline.Pipeline{
		Name:       "brain-write-test",
		WriteBrain: true,
		Steps: []pipeline.Step{
			{ID: "write-step", Plugin: "writer", Prompt: "analyze this"},
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
	if note.Tags != "test,analysis" {
		t.Errorf("Tags: want %q, got %q", "test,analysis", note.Tags)
	}
}

// TestBrainFeedbackLoop verifies that a step A that writes a brain note
// causes step B (use_brain: true, needs A) to receive that note in its preamble.
func TestBrainFeedbackLoop(t *testing.T) {
	s := openTestStore(t)
	mgr := plugin.NewManager()

	// Step A: outputs a brain note.
	writerOutput := `Analysis complete. <brain tags="loop">Loop insight from step A</brain>`
	if err := mgr.Register(&plugin.StubPlugin{
		PluginName: "writer",
		ExecuteFn: func(_ context.Context, _ string, _ map[string]string, w io.Writer) error {
			_, err := w.Write([]byte(writerOutput))
			return err
		},
	}); err != nil {
		t.Fatalf("Register writer: %v", err)
	}

	// Step B: captures its prompt to verify preamble injection.
	var capturedB string
	if err := mgr.Register(&plugin.StubPlugin{
		PluginName: "reader",
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
				Plugin:     "writer",
				Prompt:     "write something",
				WriteBrain: &trueVal,
			},
			{
				ID:       "step-b",
				Plugin:   "reader",
				Prompt:   "read something",
				Needs:    []string{"step-a"},
				UseBrain: &trueVal,
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
	if !strings.Contains(capturedB, "## ORCAI Database Context") {
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

	mgr := plugin.NewManager()
	var captured string
	capturePlugin(t, mgr, "echo", "result", &captured)

	p := &pipeline.Pipeline{
		Name:     "brain-isolation-test",
		UseBrain: true,
		Steps: []pipeline.Step{
			{ID: "s1", Plugin: "echo", Prompt: "hello"},
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
	if !strings.Contains(captured, "## ORCAI Database Context") {
		t.Errorf("expected preamble header, got: %q", captured)
	}
}

// TestBrainWriteInsertion_NoBrainBlock verifies that when a write_brain step
// emits output with no <brain> block, the step succeeds and no note is stored.
func TestBrainWriteInsertion_NoBrainBlock(t *testing.T) {
	s := openTestStore(t)
	mgr := plugin.NewManager()
	capturePlugin(t, mgr, "emit-text", "just some text, no brain block here", nil)

	p := &pipeline.Pipeline{
		Name:       "no-brain-block",
		WriteBrain: true,
		Steps: []pipeline.Step{
			{ID: "s1", Plugin: "emit-text"},
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
	mgr := plugin.NewManager()
	capturePlugin(t, mgr, "emit-bad", "<brain unclosed content", nil)

	p := &pipeline.Pipeline{
		Name:       "malformed-brain-block",
		WriteBrain: true,
		Steps: []pipeline.Step{
			{ID: "s1", Plugin: "emit-bad"},
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
	mgr := plugin.NewManager()
	capturePlugin(t, mgr, "emit-notags", "<brain>plain note with no tags</brain>", nil)

	p := &pipeline.Pipeline{
		Name:       "no-tags-brain-block",
		WriteBrain: true,
		Steps: []pipeline.Step{
			{ID: "s1", Plugin: "emit-notags"},
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
	mgr := plugin.NewManager()

	// Step A: plain step that just succeeds.
	capturePlugin(t, mgr, "noop", "ok", nil)

	// Step B: depends on A, emits a brain block.
	trueVal := true
	if err := mgr.Register(&plugin.StubPlugin{
		PluginName: "dag-writer",
		ExecuteFn: func(_ context.Context, _ string, _ map[string]string, w io.Writer) error {
			_, err := w.Write([]byte(`<brain tags="dag">insight from dag</brain>`))
			return err
		},
	}); err != nil {
		t.Fatalf("Register dag-writer: %v", err)
	}

	p := &pipeline.Pipeline{
		Name: "dag-brain-write",
		Steps: []pipeline.Step{
			{ID: "step-a", Plugin: "noop"},
			{
				ID:         "step-b",
				Plugin:     "dag-writer",
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
	if note.Tags != "dag" {
		t.Errorf("Tags: want %q, got %q", "dag", note.Tags)
	}
}

// TestBrainLegacyPath verifies that brain injection works on the legacy runner
// (pipelines without `needs`, `retry`, `for_each`, or `on_failure`).
func TestBrainLegacyPath(t *testing.T) {
	s := openTestStore(t)
	mgr := plugin.NewManager()
	var captured string
	capturePlugin(t, mgr, "echo", "result", &captured)

	// No Needs fields → isLegacyPipeline returns true.
	p := &pipeline.Pipeline{
		Name:     "brain-legacy-test",
		UseBrain: true,
		Steps: []pipeline.Step{
			{ID: "s1", Plugin: "echo", Prompt: "legacy hello"},
		},
	}

	_, err := pipeline.Run(context.Background(), p, mgr, "",
		pipeline.WithRunStore(s),
		pipeline.WithBrainInjector(pipeline.NewStoreBrainInjector(s)),
	)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	if !strings.Contains(captured, "## ORCAI Database Context") {
		t.Errorf("expected brain preamble in legacy path prompt, got: %q", captured)
	}
}

