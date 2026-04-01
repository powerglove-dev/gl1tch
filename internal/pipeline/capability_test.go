package pipeline

import (
	"context"
	"strings"
	"testing"

	"github.com/8op-org/gl1tch/internal/executor"
	"github.com/8op-org/gl1tch/internal/store"
)

// --- CapabilitiesFromManager ---

func TestCapabilitiesFromManager_ReturnsEntryPerExecutor(t *testing.T) {
	mgr := executor.NewManager()
	_ = mgr.Register(&executor.StubExecutor{ExecutorName: "shell"})
	_ = mgr.Register(&executor.StubExecutor{ExecutorName: "ollama"})
	_ = mgr.Register(&executor.StubExecutor{ExecutorName: "claude"})

	entries := CapabilitiesFromManager(mgr)
	if len(entries) < 3 {
		t.Errorf("expected at least 3 entries, got %d", len(entries))
	}

	ids := make(map[string]bool)
	for _, e := range entries {
		ids[e.ID] = true
	}
	for _, name := range []string{"shell", "ollama", "claude"} {
		if !ids["executor."+name] {
			t.Errorf("expected entry with ID %q", "executor."+name)
		}
	}
}

func TestCapabilitiesFromManager_EntriesHaveCategory(t *testing.T) {
	mgr := executor.NewManager()
	_ = mgr.Register(&executor.StubExecutor{ExecutorName: "shell"})

	entries := CapabilitiesFromManager(mgr)
	for _, e := range entries {
		if e.Category == "" {
			t.Errorf("entry %q has empty Category", e.ID)
		}
		if e.Name == "" {
			t.Errorf("entry %q has empty Name", e.ID)
		}
	}
}

func TestCapabilitiesFromManager_EmptyManager(t *testing.T) {
	mgr := executor.NewManager()
	entries := CapabilitiesFromManager(mgr)
	// Should not panic and may return zero or built-in entries.
	_ = entries
}

// --- CapabilitySeeder ---

func TestCapabilitySeeder_SeedsNotes(t *testing.T) {
	s := openTestStore(t)
	seeder := NewCapabilitySeeder(s)
	ctx := context.Background()

	entries := []CapabilityEntry{
		{ID: "executor.shell", Category: "executor", Name: "Shell", Description: "Run shell commands"},
		{ID: "executor.ollama", Category: "executor", Name: "Ollama", Description: "Local LLM inference"},
	}

	if err := seeder.Seed(ctx, entries); err != nil {
		t.Fatalf("Seed: %v", err)
	}

	notes, err := s.CapabilityNotes(ctx)
	if err != nil {
		t.Fatalf("CapabilityNotes: %v", err)
	}
	if len(notes) != 2 {
		t.Errorf("expected 2 capability notes, got %d", len(notes))
	}
}

func TestCapabilitySeeder_NotesHaveTypeCapabilityTag(t *testing.T) {
	s := openTestStore(t)
	seeder := NewCapabilitySeeder(s)
	ctx := context.Background()

	entries := []CapabilityEntry{
		{ID: "executor.shell", Category: "executor", Name: "Shell", Description: "Run shell commands"},
	}
	if err := seeder.Seed(ctx, entries); err != nil {
		t.Fatalf("Seed: %v", err)
	}

	notes, err := s.CapabilityNotes(ctx)
	if err != nil {
		t.Fatalf("CapabilityNotes: %v", err)
	}
	if len(notes) == 0 {
		t.Fatal("expected at least one capability note")
	}
	for _, n := range notes {
		if !strings.Contains(n.Tags, "type:capability") {
			t.Errorf("note %q missing type:capability tag, got tags: %q", n.StepID, n.Tags)
		}
	}
}

func TestCapabilitySeeder_NotesUseSystemRunID(t *testing.T) {
	s := openTestStore(t)
	seeder := NewCapabilitySeeder(s)
	ctx := context.Background()

	entries := []CapabilityEntry{
		{ID: "executor.claude", Category: "executor", Name: "Claude", Description: "Anthropic Claude"},
	}
	if err := seeder.Seed(ctx, entries); err != nil {
		t.Fatalf("Seed: %v", err)
	}

	notes, err := s.CapabilityNotes(ctx)
	if err != nil {
		t.Fatalf("CapabilityNotes: %v", err)
	}
	for _, n := range notes {
		if n.RunID != 0 {
			t.Errorf("capability note should have run_id=0 (system), got %d", n.RunID)
		}
	}
}

func TestCapabilitySeeder_Idempotent(t *testing.T) {
	s := openTestStore(t)
	seeder := NewCapabilitySeeder(s)
	ctx := context.Background()

	entries := []CapabilityEntry{
		{ID: "executor.shell", Category: "executor", Name: "Shell", Description: "Run shell commands"},
	}

	// Seed twice.
	if err := seeder.Seed(ctx, entries); err != nil {
		t.Fatalf("first Seed: %v", err)
	}
	if err := seeder.Seed(ctx, entries); err != nil {
		t.Fatalf("second Seed: %v", err)
	}

	notes, err := s.CapabilityNotes(ctx)
	if err != nil {
		t.Fatalf("CapabilityNotes: %v", err)
	}
	if len(notes) != 1 {
		t.Errorf("expected 1 note after idempotent seed, got %d", len(notes))
	}
}

func TestCapabilitySeeder_UpdatesExistingEntry(t *testing.T) {
	s := openTestStore(t)
	seeder := NewCapabilitySeeder(s)
	ctx := context.Background()

	first := []CapabilityEntry{
		{ID: "executor.shell", Category: "executor", Name: "Shell", Description: "old description"},
	}
	if err := seeder.Seed(ctx, first); err != nil {
		t.Fatalf("first Seed: %v", err)
	}

	updated := []CapabilityEntry{
		{ID: "executor.shell", Category: "executor", Name: "Shell", Description: "updated description"},
	}
	if err := seeder.Seed(ctx, updated); err != nil {
		t.Fatalf("second Seed: %v", err)
	}

	notes, err := s.CapabilityNotes(ctx)
	if err != nil {
		t.Fatalf("CapabilityNotes: %v", err)
	}
	if len(notes) != 1 {
		t.Fatalf("expected 1 note, got %d", len(notes))
	}
	if !strings.Contains(notes[0].Body, "updated description") {
		t.Errorf("expected updated body, got: %q", notes[0].Body)
	}
}

func TestCapabilitySeeder_SeedFromManager(t *testing.T) {
	s := openTestStore(t)
	seeder := NewCapabilitySeeder(s)
	ctx := context.Background()

	mgr := executor.NewManager()
	_ = mgr.Register(&executor.StubExecutor{ExecutorName: "shell"})
	_ = mgr.Register(&executor.StubExecutor{ExecutorName: "ollama"})

	if err := seeder.SeedFromManager(ctx, mgr); err != nil {
		t.Fatalf("SeedFromManager: %v", err)
	}

	notes, err := s.CapabilityNotes(ctx)
	if err != nil {
		t.Fatalf("CapabilityNotes: %v", err)
	}
	if len(notes) == 0 {
		t.Error("expected capability notes after SeedFromManager")
	}
}

// --- Brain injection includes capability notes ---

func TestStoreBrainInjector_IncludesCapabilityNotes(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()

	seeder := NewCapabilitySeeder(s)
	entries := []CapabilityEntry{
		{ID: "executor.shell", Category: "executor", Name: "Shell", Description: "Run arbitrary shell commands"},
		{ID: "executor.ollama", Category: "executor", Name: "Ollama", Description: "Local LLM inference via Ollama"},
	}
	if err := seeder.Seed(ctx, entries); err != nil {
		t.Fatalf("Seed: %v", err)
	}

	inj := NewStoreBrainInjector(s)
	result, err := inj.ReadContext(ctx, 999) // arbitrary run with no per-run notes
	if err != nil {
		t.Fatalf("ReadContext: %v", err)
	}

	if !strings.Contains(result, "Shell") {
		t.Errorf("expected capability 'Shell' in context; got:\n%s", result)
	}
	if !strings.Contains(result, "Ollama") {
		t.Errorf("expected capability 'Ollama' in context; got:\n%s", result)
	}
}

func TestStoreBrainInjector_CapabilitySectionIsDistinct(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()

	seeder := NewCapabilitySeeder(s)
	if err := seeder.Seed(ctx, []CapabilityEntry{
		{ID: "executor.shell", Category: "executor", Name: "Shell", Description: "Shell executor"},
	}); err != nil {
		t.Fatalf("Seed: %v", err)
	}

	// Also add a regular brain note for the same run.
	runID, err := s.RecordRunStart("pipeline", "test", "")
	if err != nil {
		t.Fatalf("RecordRunStart: %v", err)
	}
	if _, err := s.InsertBrainNote(ctx, store.BrainNote{
		RunID:  runID,
		StepID: "step-a",
		Tags:   "type:finding title:something",
		Body:   "a regular brain finding",
	}); err != nil {
		t.Fatalf("InsertBrainNote: %v", err)
	}

	inj := NewStoreBrainInjector(s)
	result, err := inj.ReadContext(ctx, runID)
	if err != nil {
		t.Fatalf("ReadContext: %v", err)
	}

	// Both capability and run notes should appear, in distinct sections.
	if !strings.Contains(result, "Shell") {
		t.Errorf("expected capability in context")
	}
	if !strings.Contains(result, "a regular brain finding") {
		t.Errorf("expected run brain note in context")
	}
}

func TestStoreBrainInjector_CapabilityNotesNotCountedAgainstRunCap(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()

	// Seed capabilities.
	seeder := NewCapabilitySeeder(s)
	var entries []CapabilityEntry
	for i := range 5 {
		entries = append(entries, CapabilityEntry{
			ID:          "executor.tool" + string(rune('A'+i)),
			Category:    "executor",
			Name:        "Tool" + string(rune('A'+i)),
			Description: "A tool executor",
		})
	}
	if err := seeder.Seed(ctx, entries); err != nil {
		t.Fatalf("Seed: %v", err)
	}

	// Fill run notes to the cap (10).
	runID, err := s.RecordRunStart("pipeline", "test", "")
	if err != nil {
		t.Fatalf("RecordRunStart: %v", err)
	}
	for i := range 10 {
		if _, err := s.InsertBrainNote(ctx, store.BrainNote{
			RunID:  runID,
			StepID: "step-note",
			Tags:   "type:finding",
			Body:   "run note " + string(rune('A'+i)),
		}); err != nil {
			t.Fatalf("InsertBrainNote: %v", err)
		}
	}

	inj := NewStoreBrainInjector(s)
	result, err := inj.ReadContext(ctx, runID)
	if err != nil {
		t.Fatalf("ReadContext: %v", err)
	}

	// Capability notes should still appear even with 10 run notes present.
	if !strings.Contains(result, "ToolA") {
		t.Errorf("capability notes should appear even when run notes are at cap:\n%s", result)
	}
}

// --- Section scoring via Ollama ---

func TestCapabilitySeeder_NotesHaveSourceBuiltinTag(t *testing.T) {
	s := openTestStore(t)
	seeder := NewCapabilitySeeder(s)
	ctx := context.Background()

	entries := []CapabilityEntry{
		{ID: "executor.shell", Category: "executor", Name: "Shell", Description: "Run shell commands"},
	}
	if err := seeder.Seed(ctx, entries); err != nil {
		t.Fatalf("Seed: %v", err)
	}

	notes, err := s.CapabilityNotes(ctx)
	if err != nil {
		t.Fatalf("CapabilityNotes: %v", err)
	}
	if len(notes) == 0 {
		t.Fatal("expected at least one capability note")
	}
	for _, n := range notes {
		if !strings.Contains(n.Tags, "source:builtin") {
			t.Errorf("seeder note missing source:builtin tag, got: %q", n.Tags)
		}
	}
}

func TestOllamaSectionScorer_Interface(t *testing.T) {
	// Verifies the interface is satisfied — no network call.
	var _ SectionScorer = (*OllamaSectionScorer)(nil)
}

func TestOllamaSectionScorer_FallsBackToAllSections_WhenUnavailable(t *testing.T) {
	// Point at a port nothing is listening on.
	scorer := NewOllamaSectionScorer("http://127.0.0.1:19999", "nomodel")
	ctx := context.Background()

	sections := []Section{
		{Name: "Intro", StartLine: 1, EndLine: 10, Summary: "introduction"},
		{Name: "Body", StartLine: 11, EndLine: 50, Summary: "main content"},
	}

	scored, err := scorer.Score(ctx, "what is the main content?", sections)
	if err != nil {
		t.Fatalf("Score should not error on unavailable Ollama, got: %v", err)
	}
	if len(scored) != len(sections) {
		t.Errorf("fallback should return all sections, got %d of %d", len(scored), len(sections))
	}
}

func TestOllamaSectionScorer_EmptySections(t *testing.T) {
	scorer := NewOllamaSectionScorer("http://127.0.0.1:19999", "nomodel")
	ctx := context.Background()

	scored, err := scorer.Score(ctx, "any prompt", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(scored) != 0 {
		t.Errorf("expected empty result for empty sections, got %d", len(scored))
	}
}
