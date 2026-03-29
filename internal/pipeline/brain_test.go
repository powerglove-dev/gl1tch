package pipeline

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/adam-stokes/orcai/internal/store"
)

func openTestStore(t *testing.T) *store.Store {
	t.Helper()
	s, err := store.OpenAt(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatalf("open test store: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestStoreBrainInjector_NoNotes(t *testing.T) {
	s := openTestStore(t)
	inj := NewStoreBrainInjector(s)

	ctx := context.Background()
	result, err := inj.ReadContext(ctx, 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result == "" {
		t.Fatal("expected non-empty preamble")
	}
	if !strings.Contains(result, "## ORCAI Database Context") {
		t.Errorf("expected schema header; got:\n%s", result)
	}
	if strings.Contains(result, "## Brain Notes") {
		t.Errorf("expected no brain notes section; got:\n%s", result)
	}
}

func TestStoreBrainInjector_WithNotes(t *testing.T) {
	s := openTestStore(t)

	// Create a run to associate notes with.
	runID, err := s.RecordRunStart("pipeline", "test-pipeline", "")
	if err != nil {
		t.Fatalf("RecordRunStart: %v", err)
	}

	ctx := context.Background()
	bodies := []string{"note alpha", "note beta", "note gamma"}
	for i, body := range bodies {
		_, err := s.InsertBrainNote(ctx, store.BrainNote{
			RunID:     runID,
			StepID:    "step-" + string(rune('a'+i)),
			CreatedAt: time.Now().UnixMilli(),
			Body:      body,
		})
		if err != nil {
			t.Fatalf("InsertBrainNote: %v", err)
		}
	}

	inj := NewStoreBrainInjector(s)
	result, err := inj.ReadContext(ctx, runID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(result, "## Brain Notes (this run)") {
		t.Errorf("expected brain notes header; got:\n%s", result)
	}
	for _, body := range bodies {
		if !strings.Contains(result, body) {
			t.Errorf("expected note body %q in result; got:\n%s", body, result)
		}
	}
}

func TestStoreBrainInjector_CapAt10(t *testing.T) {
	s := openTestStore(t)

	runID, err := s.RecordRunStart("pipeline", "test-pipeline", "")
	if err != nil {
		t.Fatalf("RecordRunStart: %v", err)
	}

	ctx := context.Background()
	const totalNotes = 15
	const prefix = "captest-note-body-"
	for i := 0; i < totalNotes; i++ {
		_, err := s.InsertBrainNote(ctx, store.BrainNote{
			RunID:     runID,
			StepID:    "step-cap",
			CreatedAt: time.Now().UnixMilli(),
			Body:      prefix + string(rune('A'+i)),
		})
		if err != nil {
			t.Fatalf("InsertBrainNote: %v", err)
		}
	}

	inj := NewStoreBrainInjector(s)
	result, err := inj.ReadContext(ctx, runID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	count := strings.Count(result, prefix)
	if count > 10 {
		t.Errorf("expected at most 10 notes, got %d in result:\n%s", count, result)
	}
	if count == 0 {
		t.Errorf("expected at least some notes; got:\n%s", result)
	}
}

func TestStoreBrainInjector_TruncateAt500(t *testing.T) {
	s := openTestStore(t)

	runID, err := s.RecordRunStart("pipeline", "test-pipeline", "")
	if err != nil {
		t.Fatalf("RecordRunStart: %v", err)
	}

	ctx := context.Background()
	longBody := strings.Repeat("x", 800)
	_, err = s.InsertBrainNote(ctx, store.BrainNote{
		RunID:     runID,
		StepID:    "step-trunc",
		CreatedAt: time.Now().UnixMilli(),
		Body:      longBody,
	})
	if err != nil {
		t.Fatalf("InsertBrainNote: %v", err)
	}

	inj := NewStoreBrainInjector(s)
	result, err := inj.ReadContext(ctx, runID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(result, "...[truncated]") {
		t.Errorf("expected truncation marker; got:\n%s", result)
	}
	if strings.Contains(result, longBody) {
		t.Errorf("expected full 800-char body NOT to appear in result; got:\n%s", result)
	}
}
