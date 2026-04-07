// workspace.go provides the small surface every collector uses to
// stamp its indexed documents with the owning workspace's id.
//
// Per the Phase 4 split, every collector runs inside a "workspace
// pod" — a tree of services scoped to one workspace — and indexed
// docs must carry the workspace_id keyword field so brain queries
// can scope to a single workspace's data.
//
// Collectors hold a WorkspaceID string field. They build a slice of
// docs the same way they always have (esearch.Event values, mostly)
// and then call StampWorkspaceID(workspaceID, docs) immediately
// before BulkIndex. The helper walks the slice once, sets the field
// on every Event / PipelineRun it recognizes, and leaves anything
// else untouched.
//
// Empty workspaceID is allowed and means "global / unattributed".
// Stamping a doc that already has a non-empty WorkspaceID is a
// no-op so collectors that build their own pre-tagged docs never
// get clobbered.
package collector

import (
	"github.com/8op-org/gl1tch/internal/esearch"
)

// StampWorkspaceID walks docs and sets WorkspaceID on every entry
// that is an esearch.Event or esearch.PipelineRun (by value), unless
// the entry already has a non-empty WorkspaceID. Other doc shapes
// (raw map[string]any, custom structs) pass through untouched —
// callers indexing those types should set workspace_id explicitly.
//
// The helper takes the slice by value because the underlying entries
// are concrete struct values (not pointers) inside the []any slice;
// we have to assign the modified copy back into place.
func StampWorkspaceID(workspaceID string, docs []any) []any {
	if workspaceID == "" {
		return docs
	}
	for i, d := range docs {
		switch v := d.(type) {
		case esearch.Event:
			if v.WorkspaceID == "" {
				v.WorkspaceID = workspaceID
				docs[i] = v
			}
		case *esearch.Event:
			if v != nil && v.WorkspaceID == "" {
				v.WorkspaceID = workspaceID
			}
		case esearch.PipelineRun:
			if v.WorkspaceID == "" {
				v.WorkspaceID = workspaceID
				docs[i] = v
			}
		case *esearch.PipelineRun:
			if v != nil && v.WorkspaceID == "" {
				v.WorkspaceID = workspaceID
			}
		case map[string]any:
			// Raw map docs (used by some collectors) get the same
			// treatment via a string key. Don't overwrite an existing
			// non-empty value.
			if existing, ok := v["workspace_id"].(string); !ok || existing == "" {
				v["workspace_id"] = workspaceID
			}
		}
	}
	return docs
}
