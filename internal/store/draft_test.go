package store

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"
)

func TestDraftCRUD(t *testing.T) {
	ctx := context.Background()

	t.Run("empty workspace returns non-nil empty slice", func(t *testing.T) {
		s := openTestStore(t)
		got, err := s.ListDraftsByWorkspace(ctx, "ws-1", "")
		if err != nil {
			t.Fatalf("ListDraftsByWorkspace: %v", err)
		}
		if got == nil {
			t.Error("want non-nil empty slice, got nil")
		}
		if len(got) != 0 {
			t.Errorf("want 0 drafts, got %d", len(got))
		}
	})

	t.Run("create assigns id and timestamps", func(t *testing.T) {
		s := openTestStore(t)
		id, err := s.CreateDraft(ctx, Draft{
			WorkspaceID: "ws-1",
			Kind:        DraftKindPrompt,
			Title:       "first",
			Body:        "initial body",
		})
		if err != nil {
			t.Fatalf("CreateDraft: %v", err)
		}
		if id <= 0 {
			t.Fatalf("want positive id, got %d", id)
		}

		got, err := s.GetDraft(ctx, id)
		if err != nil {
			t.Fatalf("GetDraft: %v", err)
		}
		if got.Title != "first" || got.Body != "initial body" {
			t.Errorf("round-trip mismatch: %+v", got)
		}
		if got.CreatedAt == 0 || got.UpdatedAt == 0 {
			t.Errorf("want timestamps populated, got created=%d updated=%d", got.CreatedAt, got.UpdatedAt)
		}
		if got.Turns == nil {
			t.Errorf("want non-nil empty turns, got nil")
		}
		if got.TargetID != 0 {
			t.Errorf("want unset target_id, got %d", got.TargetID)
		}
	})

	t.Run("list scopes by workspace and kind", func(t *testing.T) {
		s := openTestStore(t)

		// Two prompts in ws-1, one workflow in ws-1, one prompt in ws-2.
		mustCreateDraft(t, s, Draft{WorkspaceID: "ws-1", Kind: DraftKindPrompt, Title: "p1"})
		time.Sleep(2 * time.Millisecond)
		mustCreateDraft(t, s, Draft{WorkspaceID: "ws-1", Kind: DraftKindPrompt, Title: "p2"})
		mustCreateDraft(t, s, Draft{WorkspaceID: "ws-1", Kind: DraftKindWorkflow, Title: "w1"})
		mustCreateDraft(t, s, Draft{WorkspaceID: "ws-2", Kind: DraftKindPrompt, Title: "other"})

		all, err := s.ListDraftsByWorkspace(ctx, "ws-1", "")
		if err != nil {
			t.Fatalf("ListDraftsByWorkspace all: %v", err)
		}
		if len(all) != 3 {
			t.Errorf("want 3 drafts in ws-1, got %d", len(all))
		}

		prompts, err := s.ListDraftsByWorkspace(ctx, "ws-1", DraftKindPrompt)
		if err != nil {
			t.Fatalf("ListDraftsByWorkspace prompts: %v", err)
		}
		if len(prompts) != 2 {
			t.Fatalf("want 2 prompt drafts in ws-1, got %d", len(prompts))
		}
		// Newest first.
		if prompts[0].Title != "p2" {
			t.Errorf("want p2 first (newest), got %q", prompts[0].Title)
		}

		// ws-2 isolation.
		other, err := s.ListDraftsByWorkspace(ctx, "ws-2", "")
		if err != nil {
			t.Fatalf("ListDraftsByWorkspace ws-2: %v", err)
		}
		if len(other) != 1 || other[0].Title != "other" {
			t.Errorf("want only ws-2's 'other' draft, got %+v", other)
		}
	})

	t.Run("update replaces editable fields", func(t *testing.T) {
		s := openTestStore(t)
		id := mustCreateDraft(t, s, Draft{WorkspaceID: "ws-1", Kind: DraftKindPrompt, Title: "before"})

		err := s.UpdateDraft(ctx, Draft{
			ID:         id,
			Title:      "after",
			Body:       "new body",
			TargetID:   42,
			TargetPath: "/tmp/foo.yaml",
		})
		if err != nil {
			t.Fatalf("UpdateDraft: %v", err)
		}

		got, _ := s.GetDraft(ctx, id)
		if got.Title != "after" || got.Body != "new body" {
			t.Errorf("update did not persist: %+v", got)
		}
		if got.TargetID != 42 || got.TargetPath != "/tmp/foo.yaml" {
			t.Errorf("target fields did not persist: id=%d path=%q", got.TargetID, got.TargetPath)
		}
	})

	t.Run("update missing draft errors", func(t *testing.T) {
		s := openTestStore(t)
		err := s.UpdateDraft(ctx, Draft{ID: 9999, Title: "x"})
		if err == nil {
			t.Error("want error updating missing draft, got nil")
		}
	})

	t.Run("append turn updates body and history", func(t *testing.T) {
		s := openTestStore(t)
		id := mustCreateDraft(t, s, Draft{
			WorkspaceID: "ws-1",
			Kind:        DraftKindPrompt,
			Title:       "iter",
			Body:        "v0",
		})

		// First refinement.
		t1 := DraftTurn{
			Role:      "user",
			Text:      "make it more concise",
			Body:      "v1",
			Provider:  "ollama",
			Model:     "llama3.2",
			Timestamp: time.Now().Unix(),
		}
		if err := s.AppendDraftTurn(ctx, id, t1, "v1"); err != nil {
			t.Fatalf("AppendDraftTurn first: %v", err)
		}

		// Second refinement.
		t2 := DraftTurn{
			Role:      "user",
			Text:      "add an example",
			Body:      "v2",
			Provider:  "ollama",
			Model:     "llama3.2",
			Timestamp: time.Now().Unix(),
		}
		if err := s.AppendDraftTurn(ctx, id, t2, "v2"); err != nil {
			t.Fatalf("AppendDraftTurn second: %v", err)
		}

		got, err := s.GetDraft(ctx, id)
		if err != nil {
			t.Fatalf("GetDraft: %v", err)
		}
		if got.Body != "v2" {
			t.Errorf("want body v2, got %q", got.Body)
		}
		if len(got.Turns) != 2 {
			t.Fatalf("want 2 turns, got %d", len(got.Turns))
		}
		if got.Turns[0].Text != "make it more concise" {
			t.Errorf("turn[0] text mismatch: %q", got.Turns[0].Text)
		}
		if got.Turns[1].Body != "v2" {
			t.Errorf("turn[1] body mismatch: %q", got.Turns[1].Body)
		}
	})

	t.Run("append on missing draft errors", func(t *testing.T) {
		s := openTestStore(t)
		err := s.AppendDraftTurn(ctx, 9999, DraftTurn{Role: "user"}, "x")
		if err == nil {
			t.Error("want error appending to missing draft, got nil")
		}
	})

	t.Run("delete removes the draft", func(t *testing.T) {
		s := openTestStore(t)
		id := mustCreateDraft(t, s, Draft{WorkspaceID: "ws-1", Kind: DraftKindPrompt, Title: "doomed"})
		if err := s.DeleteDraft(ctx, id); err != nil {
			t.Fatalf("DeleteDraft: %v", err)
		}
		_, err := s.GetDraft(ctx, id)
		if !errors.Is(err, sql.ErrNoRows) {
			t.Errorf("want sql.ErrNoRows after delete, got %v", err)
		}
	})

	t.Run("delete missing draft errors", func(t *testing.T) {
		s := openTestStore(t)
		err := s.DeleteDraft(ctx, 9999)
		if err == nil {
			t.Error("want error deleting missing draft, got nil")
		}
	})

	t.Run("get missing draft returns ErrNoRows", func(t *testing.T) {
		s := openTestStore(t)
		_, err := s.GetDraft(ctx, 9999)
		if !errors.Is(err, sql.ErrNoRows) {
			t.Errorf("want sql.ErrNoRows, got %v", err)
		}
	})
}

// mustCreateDraft is a tiny helper so the table-driven cases stay readable.
// Fails the test on insert errors so callers don't have to repeat that boilerplate.
func mustCreateDraft(t *testing.T, s *Store, d Draft) int64 {
	t.Helper()
	id, err := s.CreateDraft(context.Background(), d)
	if err != nil {
		t.Fatalf("CreateDraft: %v", err)
	}
	return id
}
