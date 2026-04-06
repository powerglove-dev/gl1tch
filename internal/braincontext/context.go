// Package braincontext provides types for linking brain notes to pipeline workspaces.
package braincontext

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// WorkspaceType identifies the kind of workspace a brain context is associated with.
type WorkspaceType string

const (
	WorkspacePipeline  WorkspaceType = "pipeline"
	WorkspaceAgent     WorkspaceType = "agent"
	WorkspaceScheduled WorkspaceType = "scheduled"
)

// WorkspaceContext links a workspace (pipeline, agent, or scheduled) to a set of
// brain note IDs. This is persisted as a JSON sidecar file alongside the pipeline YAML.
type WorkspaceContext struct {
	WorkspaceType WorkspaceType `json:"workspace_type,omitempty"`
	WorkspaceID   string        `json:"workspace_id"`
	LinkedNoteIDs []string      `json:"linked_note_ids"`
}

// Empty returns a zero-value WorkspaceContext.
func Empty() WorkspaceContext { return WorkspaceContext{} }

// sidecarPath returns the path of the .brain.json sidecar for a given workflow file.
// e.g. /workflows/my.workflow.yaml → /workflows/my.brain.json
func sidecarPath(workflowPath string) string {
	base := filepath.Base(workflowPath)
	name := strings.TrimSuffix(base, ".workflow.yaml")
	if name == base {
		name = strings.TrimSuffix(base, ".yaml")
	}
	return filepath.Join(filepath.Dir(workflowPath), name+".brain.json")
}

// LoadWorkspaceContext reads the brain sidecar file for the given pipeline path.
// Returns Empty() if the sidecar does not exist (not an error).
func LoadWorkspaceContext(pipelinePath string) (WorkspaceContext, error) {
	sp := sidecarPath(pipelinePath)
	data, err := os.ReadFile(sp)
	if os.IsNotExist(err) {
		return Empty(), nil
	}
	if err != nil {
		return Empty(), fmt.Errorf("braincontext: read sidecar %q: %w", sp, err)
	}
	var wc WorkspaceContext
	if err := json.Unmarshal(data, &wc); err != nil {
		return Empty(), fmt.Errorf("braincontext: parse sidecar %q: %w", sp, err)
	}
	return wc, nil
}

// SaveWorkspaceContext writes the WorkspaceContext to the sidecar file alongside path.
func SaveWorkspaceContext(pipelinePath string, wc WorkspaceContext) error {
	sp := sidecarPath(pipelinePath)
	data, err := json.MarshalIndent(wc, "", "  ")
	if err != nil {
		return fmt.Errorf("braincontext: marshal: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(sp), 0o755); err != nil {
		return fmt.Errorf("braincontext: mkdir: %w", err)
	}
	if err := os.WriteFile(sp, data, 0o600); err != nil {
		return fmt.Errorf("braincontext: write sidecar %q: %w", sp, err)
	}
	return nil
}
