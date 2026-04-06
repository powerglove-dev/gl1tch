package store

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"
)

func TestChatWorkflowCRUD(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()

	t.Run("empty workspace returns empty slice", func(t *testing.T) {
		got, err := s.ListChatWorkflows(ctx, "ws-1")
		if err != nil {
			t.Fatalf("ListChatWorkflows: %v", err)
		}
		if got == nil {
			t.Error("want non-nil empty slice, got nil")
		}
		if len(got) != 0 {
			t.Errorf("want 0 workflows, got %d", len(got))
		}
	})

	t.Run("insert and list", func(t *testing.T) {
		s := openTestStore(t)

		id1, err := s.InsertChatWorkflow(ctx, ChatWorkflow{
			WorkspaceID: "ws-1",
			Name:        "scan-then-audit",
			StepsJSON:   `[{"type":"prompt","label":"Security Scan"},{"type":"pipeline","label":"workflow-audit"}]`,
		})
		if err != nil {
			t.Fatalf("InsertChatWorkflow: %v", err)
		}
		if id1 <= 0 {
			t.Errorf("expected positive id, got %d", id1)
		}

		// Different workspace - should not show up
		_, err = s.InsertChatWorkflow(ctx, ChatWorkflow{
			WorkspaceID: "ws-2",
			Name:        "other-ws-flow",
			StepsJSON:   `[]`,
		})
		if err != nil {
			t.Fatalf("InsertChatWorkflow ws-2: %v", err)
		}

		time.Sleep(2 * time.Millisecond)

		id2, err := s.InsertChatWorkflow(ctx, ChatWorkflow{
			WorkspaceID: "ws-1",
			Name:        "second",
			StepsJSON:   `[]`,
		})
		if err != nil {
			t.Fatalf("InsertChatWorkflow second: %v", err)
		}

		// List should return only ws-1 workflows, newest first
		got, err := s.ListChatWorkflows(ctx, "ws-1")
		if err != nil {
			t.Fatalf("ListChatWorkflows: %v", err)
		}
		if len(got) != 2 {
			t.Fatalf("want 2 workflows for ws-1, got %d", len(got))
		}
		if got[0].ID != id2 {
			t.Errorf("want newest first (id=%d), got id=%d", id2, got[0].ID)
		}
		if got[1].ID != id1 {
			t.Errorf("want oldest last (id=%d), got id=%d", id1, got[1].ID)
		}
		if got[1].Name != "scan-then-audit" {
			t.Errorf("want name 'scan-then-audit', got %q", got[1].Name)
		}
		if got[1].StepsJSON == "" {
			t.Error("want non-empty StepsJSON")
		}
	})

	t.Run("get by ID", func(t *testing.T) {
		s := openTestStore(t)

		id, err := s.InsertChatWorkflow(ctx, ChatWorkflow{
			WorkspaceID: "ws-1",
			Name:        "fetch-me",
			StepsJSON:   `[{"type":"agent","label":"copilot"}]`,
		})
		if err != nil {
			t.Fatalf("InsertChatWorkflow: %v", err)
		}

		got, err := s.GetChatWorkflow(ctx, id)
		if err != nil {
			t.Fatalf("GetChatWorkflow: %v", err)
		}
		if got.Name != "fetch-me" {
			t.Errorf("want name 'fetch-me', got %q", got.Name)
		}
		if got.WorkspaceID != "ws-1" {
			t.Errorf("want workspace 'ws-1', got %q", got.WorkspaceID)
		}
	})

	t.Run("get nonexistent returns ErrNoRows", func(t *testing.T) {
		s := openTestStore(t)
		_, err := s.GetChatWorkflow(ctx, 9999)
		if !errors.Is(err, sql.ErrNoRows) {
			t.Errorf("want sql.ErrNoRows, got %v", err)
		}
	})

	t.Run("update workflow", func(t *testing.T) {
		s := openTestStore(t)

		id, err := s.InsertChatWorkflow(ctx, ChatWorkflow{
			WorkspaceID: "ws-1",
			Name:        "before",
			StepsJSON:   `[]`,
		})
		if err != nil {
			t.Fatalf("InsertChatWorkflow: %v", err)
		}

		if err := s.UpdateChatWorkflow(ctx, id, "after", `[{"type":"prompt"}]`); err != nil {
			t.Fatalf("UpdateChatWorkflow: %v", err)
		}

		got, err := s.GetChatWorkflow(ctx, id)
		if err != nil {
			t.Fatalf("GetChatWorkflow: %v", err)
		}
		if got.Name != "after" {
			t.Errorf("want name 'after', got %q", got.Name)
		}
		if got.StepsJSON != `[{"type":"prompt"}]` {
			t.Errorf("want updated steps_json, got %q", got.StepsJSON)
		}
	})

	t.Run("delete workflow", func(t *testing.T) {
		s := openTestStore(t)

		id, err := s.InsertChatWorkflow(ctx, ChatWorkflow{
			WorkspaceID: "ws-1",
			Name:        "doomed",
			StepsJSON:   `[]`,
		})
		if err != nil {
			t.Fatalf("InsertChatWorkflow: %v", err)
		}

		if err := s.DeleteChatWorkflow(ctx, id); err != nil {
			t.Fatalf("DeleteChatWorkflow: %v", err)
		}

		_, err = s.GetChatWorkflow(ctx, id)
		if !errors.Is(err, sql.ErrNoRows) {
			t.Errorf("want sql.ErrNoRows after delete, got %v", err)
		}
	})
}
