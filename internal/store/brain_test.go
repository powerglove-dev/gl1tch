package store

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

func TestInsertBrainNote_RoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.db")
	s, err := OpenAt(path)
	if err != nil {
		t.Fatalf("OpenAt: %v", err)
	}
	defer s.Close()

	ctx := context.Background()
	now := time.Now().UnixMilli()

	note := BrainNote{
		RunID:     1,
		StepID:    "step-a",
		CreatedAt: now,
		Tags:      "tag1,tag2",
		Body:      "hello brain",
	}

	id, err := s.InsertBrainNote(ctx, note)
	if err != nil {
		t.Fatalf("InsertBrainNote: %v", err)
	}
	if id <= 0 {
		t.Errorf("want id > 0, got %d", id)
	}

	notes, err := s.RecentBrainNotes(ctx, note.RunID, 10)
	if err != nil {
		t.Fatalf("RecentBrainNotes: %v", err)
	}
	if len(notes) != 1 {
		t.Fatalf("want 1 note, got %d", len(notes))
	}

	got := notes[0]
	if got.ID != id {
		t.Errorf("ID: want %d, got %d", id, got.ID)
	}
	if got.RunID != note.RunID {
		t.Errorf("RunID: want %d, got %d", note.RunID, got.RunID)
	}
	if got.StepID != note.StepID {
		t.Errorf("StepID: want %q, got %q", note.StepID, got.StepID)
	}
	if got.CreatedAt != note.CreatedAt {
		t.Errorf("CreatedAt: want %d, got %d", note.CreatedAt, got.CreatedAt)
	}
	if got.Tags != note.Tags {
		t.Errorf("Tags: want %q, got %q", note.Tags, got.Tags)
	}
	if got.Body != note.Body {
		t.Errorf("Body: want %q, got %q", note.Body, got.Body)
	}
}

func TestBrainNotes_MultiRunIsolation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.db")
	s, err := OpenAt(path)
	if err != nil {
		t.Fatalf("OpenAt: %v", err)
	}
	defer s.Close()

	ctx := context.Background()
	now := time.Now().UnixMilli()

	// Insert notes for two different run IDs.
	for _, runID := range []int64{10, 20} {
		_, err := s.InsertBrainNote(ctx, BrainNote{
			RunID:     runID,
			StepID:    "step-x",
			CreatedAt: now,
			Body:      "note for run",
		})
		if err != nil {
			t.Fatalf("InsertBrainNote(run %d): %v", runID, err)
		}
	}

	// Query only run 10 — should not return run 20's note.
	notes, err := s.RecentBrainNotes(ctx, 10, 10)
	if err != nil {
		t.Fatalf("RecentBrainNotes: %v", err)
	}
	if len(notes) != 1 {
		t.Fatalf("want 1 note for run 10, got %d", len(notes))
	}
	if notes[0].RunID != 10 {
		t.Errorf("want RunID=10, got %d", notes[0].RunID)
	}
}

func TestBrainNotes_LimitCap(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.db")
	s, err := OpenAt(path)
	if err != nil {
		t.Fatalf("OpenAt: %v", err)
	}
	defer s.Close()

	ctx := context.Background()
	runID := int64(42)

	for i := 0; i < 5; i++ {
		_, err := s.InsertBrainNote(ctx, BrainNote{
			RunID:     runID,
			StepID:    "step-loop",
			CreatedAt: time.Now().UnixMilli() + int64(i),
			Body:      "note",
		})
		if err != nil {
			t.Fatalf("InsertBrainNote[%d]: %v", i, err)
		}
	}

	notes, err := s.RecentBrainNotes(ctx, runID, 3)
	if err != nil {
		t.Fatalf("RecentBrainNotes: %v", err)
	}
	if len(notes) != 3 {
		t.Errorf("want exactly 3 notes (limit), got %d", len(notes))
	}
}

func TestBrainNotes_EmptyResult(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.db")
	s, err := OpenAt(path)
	if err != nil {
		t.Fatalf("OpenAt: %v", err)
	}
	defer s.Close()

	ctx := context.Background()

	notes, err := s.RecentBrainNotes(ctx, 999, 10)
	if err != nil {
		t.Fatalf("RecentBrainNotes: unexpected error: %v", err)
	}
	if notes == nil {
		t.Error("want non-nil empty slice, got nil")
	}
	if len(notes) != 0 {
		t.Errorf("want 0 notes, got %d", len(notes))
	}
}
