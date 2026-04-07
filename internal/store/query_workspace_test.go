package store

import (
	"testing"
)

// TestQueryRunsForWorkspace locks the contract for the workspace
// scope filter on QueryRuns. Cross-workspace contamination here
// would mean workspace A's PipelineIndexer indexing workspace B's
// runs into glitch-pipelines, silently leaking pipeline data
// across workspace boundaries.
func TestQueryRunsForWorkspace(t *testing.T) {
	s := openTestStore(t)

	// Seed three runs across two workspaces + one global (empty
	// workspace id) so the filter has something to discriminate.
	idA1, err := s.RecordRunStartWithWorkspace("pipeline", "wf-a-1", "", "ws-a")
	if err != nil {
		t.Fatalf("seed ws-a #1: %v", err)
	}
	idA2, err := s.RecordRunStartWithWorkspace("pipeline", "wf-a-2", "", "ws-a")
	if err != nil {
		t.Fatalf("seed ws-a #2: %v", err)
	}
	idB, err := s.RecordRunStartWithWorkspace("pipeline", "wf-b", "", "ws-b")
	if err != nil {
		t.Fatalf("seed ws-b: %v", err)
	}
	idGlobal, err := s.RecordRunStart("pipeline", "wf-global", "")
	if err != nil {
		t.Fatalf("seed global: %v", err)
	}

	t.Run("empty workspace id returns everything", func(t *testing.T) {
		runs, err := s.QueryRunsForWorkspace("", 100)
		if err != nil {
			t.Fatalf("query: %v", err)
		}
		if len(runs) != 4 {
			t.Errorf("got %d runs, want 4", len(runs))
		}
	})

	t.Run("filters by workspace_id", func(t *testing.T) {
		runs, err := s.QueryRunsForWorkspace("ws-a", 100)
		if err != nil {
			t.Fatalf("query: %v", err)
		}
		if len(runs) != 2 {
			t.Fatalf("got %d ws-a runs, want 2", len(runs))
		}
		seen := map[int64]bool{}
		for _, r := range runs {
			if r.WorkspaceID != "ws-a" {
				t.Errorf("run %d has wrong workspace_id: %q", r.ID, r.WorkspaceID)
			}
			seen[r.ID] = true
		}
		if !seen[idA1] || !seen[idA2] {
			t.Errorf("missing ws-a runs: ids=%v", seen)
		}
		if seen[idB] || seen[idGlobal] {
			t.Errorf("ws-b or global leaked into ws-a result")
		}
	})

	t.Run("workspace b is independent", func(t *testing.T) {
		runs, err := s.QueryRunsForWorkspace("ws-b", 100)
		if err != nil {
			t.Fatalf("query: %v", err)
		}
		if len(runs) != 1 || runs[0].ID != idB {
			t.Errorf("ws-b query returned %v, want [%d]", runs, idB)
		}
	})

	t.Run("nonexistent workspace returns empty", func(t *testing.T) {
		runs, err := s.QueryRunsForWorkspace("ws-never", 100)
		if err != nil {
			t.Fatalf("query: %v", err)
		}
		if len(runs) != 0 {
			t.Errorf("got %d runs, want 0", len(runs))
		}
	})

	t.Run("legacy QueryRuns is unfiltered", func(t *testing.T) {
		runs, err := s.QueryRuns(100)
		if err != nil {
			t.Fatalf("QueryRuns: %v", err)
		}
		if len(runs) != 4 {
			t.Errorf("legacy QueryRuns got %d, want 4", len(runs))
		}
	})

	t.Run("RecordRunStart legacy path leaves workspace_id empty", func(t *testing.T) {
		r, err := s.GetRun(idGlobal)
		if err != nil {
			t.Fatalf("GetRun: %v", err)
		}
		if r.WorkspaceID != "" {
			t.Errorf("legacy global run has non-empty workspace_id: %q", r.WorkspaceID)
		}
	})
}
