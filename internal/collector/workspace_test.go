package collector

import (
	"testing"

	"github.com/8op-org/gl1tch/internal/esearch"
)

func TestStampWorkspaceID(t *testing.T) {
	t.Run("empty workspace id is a no-op", func(t *testing.T) {
		docs := []any{
			esearch.Event{Type: "git.commit"},
			esearch.PipelineRun{Name: "x"},
		}
		out := StampWorkspaceID("", docs)
		if len(out) != 2 {
			t.Fatalf("len = %d, want 2", len(out))
		}
		if e, ok := out[0].(esearch.Event); !ok || e.WorkspaceID != "" {
			t.Errorf("event got stamped despite empty workspace id: %+v", out[0])
		}
		if p, ok := out[1].(esearch.PipelineRun); !ok || p.WorkspaceID != "" {
			t.Errorf("pipeline run got stamped despite empty workspace id: %+v", out[1])
		}
	})

	t.Run("stamps event values", func(t *testing.T) {
		docs := []any{
			esearch.Event{Type: "git.commit"},
			esearch.Event{Type: "git.push"},
		}
		out := StampWorkspaceID("ws-1", docs)
		for i, d := range out {
			e, ok := d.(esearch.Event)
			if !ok {
				t.Fatalf("entry %d not an Event: %T", i, d)
			}
			if e.WorkspaceID != "ws-1" {
				t.Errorf("entry %d: workspace_id = %q, want ws-1", i, e.WorkspaceID)
			}
		}
	})

	t.Run("stamps pipeline run values", func(t *testing.T) {
		docs := []any{esearch.PipelineRun{Name: "x"}}
		out := StampWorkspaceID("ws-2", docs)
		p, ok := out[0].(esearch.PipelineRun)
		if !ok {
			t.Fatalf("not a PipelineRun: %T", out[0])
		}
		if p.WorkspaceID != "ws-2" {
			t.Errorf("workspace_id = %q, want ws-2", p.WorkspaceID)
		}
	})

	t.Run("stamps event pointers in place", func(t *testing.T) {
		e := &esearch.Event{Type: "git.commit"}
		docs := []any{e}
		StampWorkspaceID("ws-3", docs)
		if e.WorkspaceID != "ws-3" {
			t.Errorf("pointer not updated in place: %+v", e)
		}
	})

	t.Run("does not overwrite an existing workspace id", func(t *testing.T) {
		docs := []any{
			esearch.Event{Type: "git.commit", WorkspaceID: "preset"},
		}
		out := StampWorkspaceID("ws-new", docs)
		e := out[0].(esearch.Event)
		if e.WorkspaceID != "preset" {
			t.Errorf("preset id was clobbered: got %q", e.WorkspaceID)
		}
	})

	t.Run("stamps map[string]any docs", func(t *testing.T) {
		docs := []any{
			map[string]any{"type": "git.commit"},
		}
		out := StampWorkspaceID("ws-4", docs)
		m := out[0].(map[string]any)
		if m["workspace_id"] != "ws-4" {
			t.Errorf("map not stamped: %+v", m)
		}
	})

	t.Run("does not overwrite map workspace id", func(t *testing.T) {
		docs := []any{
			map[string]any{"workspace_id": "preset"},
		}
		out := StampWorkspaceID("ws-new", docs)
		m := out[0].(map[string]any)
		if m["workspace_id"] != "preset" {
			t.Errorf("map preset id clobbered: got %v", m["workspace_id"])
		}
	})

	t.Run("ignores unknown doc types", func(t *testing.T) {
		type custom struct{ X int }
		docs := []any{custom{X: 1}, "string", 42}
		out := StampWorkspaceID("ws", docs)
		if len(out) != 3 {
			t.Fatalf("len = %d, want 3", len(out))
		}
		// Unknown types just pass through unchanged.
		if _, ok := out[0].(custom); !ok {
			t.Errorf("custom type lost")
		}
	})
}
