// workflow_files.go is the file-backed counterpart to chain_yaml.go.
// It exposes the save / delete operations the desktop app uses to
// manage workflow YAML files on disk.
//
// gl1tch has a single source of truth for workflows: real
// .workflow.yaml files under <workspace>/.glitch/workflows/. The
// chain bar's "save workflow" button writes one of these directly.
package glitchd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/8op-org/gl1tch/internal/collector"
	"github.com/8op-org/gl1tch/internal/store"
)

// errWorkspaceHasNoDirs is returned when a save/migrate cannot find
// any directory to write into for the given workspace. The user has
// to add a directory to the workspace before workflows can be saved.
var errWorkspaceHasNoDirs = errors.New("workspace has no directories — add a directory before saving workflows")

// SaveChainAsWorkflow serializes a desktop builder chain to YAML and
// writes it under the workspace's primary directory at
// <dir>/.glitch/workflows/<name>.workflow.yaml.
//
// Returns a JSON object with the resulting {path, name} on success
// or {error: "..."} on failure. The desktop app surfaces the error
// inline next to the save button.
//
// Save errors out (rather than falling back to a global location)
// when the workspace has no directories — that's the explicit
// "everything is workspace-scoped unless noted" rule.
func SaveChainAsWorkflow(ctx context.Context, workspaceID, name, description, stepsJSON, defaultProvider, defaultModel string) string {
	if strings.TrimSpace(workspaceID) == "" {
		return errorJSON(fmt.Errorf("workspace_id is required"))
	}
	st, err := OpenStore()
	if err != nil {
		return errorJSON(fmt.Errorf("open store: %w", err))
	}
	dir := primaryWorkspaceDir(ctx, st, workspaceID)
	if dir == "" {
		return errorJSON(errWorkspaceHasNoDirs)
	}

	yamlBody, err := ChainStepsToYAML(stepsJSON, name, description, defaultProvider, defaultModel)
	if err != nil {
		return errorJSON(err)
	}

	path, err := SaveWorkflow(dir, name, yamlBody)
	if err != nil {
		return errorJSON(err)
	}
	b, _ := json.Marshal(map[string]string{"path": path, "name": name})
	return string(b)
}

// DeleteWorkflowFile removes a single .workflow.yaml file. The path
// MUST live under a `.glitch/workflows/` directory — anything else is
// rejected so a malformed Wails call can't accidentally delete files
// outside the workflows tree.
//
// Returns the empty string on success, an error message on failure.
// (We use a string return rather than error so the Wails generator
// produces a frontend-friendly signature.)
func DeleteWorkflowFile(path string) string {
	if err := validateWorkflowPath(path); err != "" {
		return err
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err.Error()
	}
	return ""
}

// ReadWorkflowFile returns the raw text contents of a workflow YAML
// file. Same path safety check as DeleteWorkflowFile — the path must
// live under a .glitch/workflows directory and end in .workflow.yaml.
//
// Returns a JSON object {content: "..."} on success or {error: "..."}
// on failure. JSON-wrapping the content lets us share the same
// error-or-payload shape with the rest of the editor API.
func ReadWorkflowFile(path string) string {
	if err := validateWorkflowPath(path); err != "" {
		return errorJSON(fmt.Errorf("%s", err))
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return errorJSON(err)
	}
	b, _ := json.Marshal(map[string]string{"content": string(raw)})
	return string(b)
}

// WriteWorkflowFile overwrites a workflow YAML file with new content.
// Used by the editor popup's save action when the user has a workflow
// draft pointed at an existing file. Same safety check as the others.
//
// Returns "" on success, an error message on failure. The parent dir
// is created on demand so this also handles the "save as" case where
// the .glitch/workflows directory doesn't exist yet.
func WriteWorkflowFile(path, content string) string {
	if err := validateWorkflowPath(path); err != "" {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err.Error()
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return err.Error()
	}
	return ""
}

// validateWorkflowPath returns "" when path is acceptable for workflow
// I/O (under .glitch/workflows, ends in .workflow.yaml) or a
// human-readable refusal when not. Centralised so read/write/delete
// share the exact same gate.
func validateWorkflowPath(path string) string {
	if strings.TrimSpace(path) == "" {
		return "path is required"
	}
	if !isUnderWorkflowsDir(path) {
		return fmt.Sprintf("refusing %q: not inside a .glitch/workflows directory", path)
	}
	if !strings.HasSuffix(path, ".workflow.yaml") {
		return fmt.Sprintf("refusing %q: not a .workflow.yaml file", path)
	}
	return ""
}

// WorkflowPathForName returns the absolute path where a new workflow
// with the given name would be written for the given workspace.
// Used by the editor popup when promoting a brand-new workflow draft
// so the frontend can show "this will save to <path>" before the user
// commits.
//
// Returns "" if the workspace has no directories. The caller is
// responsible for surfacing that as an error to the user.
func WorkflowPathForName(ctx context.Context, workspaceID, name string) string {
	st, err := OpenStore()
	if err != nil {
		return ""
	}
	dir := primaryWorkspaceDir(ctx, st, workspaceID)
	if dir == "" {
		return ""
	}
	name = strings.TrimSuffix(name, ".workflow.yaml")
	return filepath.Join(dir, ".glitch", "workflows", name+".workflow.yaml")
}

// CreateDraftFromTarget creates a new draft seeded from an existing
// entity (prompt row, workflow file, skill SKILL.md, or agent .md)
// so the editor popup can be opened on something the user already
// has. The draft's target_id / target_path is set so a later
// PromoteDraft writes back to the original entity instead of
// creating a duplicate.
//
// For file-backed kinds (workflow / skill / agent), the resolved
// target_path stored on the draft is normalized so subsequent
// promotes hit the exact same file. Skills are normalized to
// <dir>/SKILL.md so the popup never has to worry about whether the
// caller passed a directory or a file.
//
// Returns the created draft as JSON (same shape as CreateDraft) or
// {error: "..."} on failure.
func CreateDraftFromTarget(ctx context.Context, workspaceID, kind string, targetID int64, targetPath string) string {
	if strings.TrimSpace(workspaceID) == "" {
		return errorJSON(fmt.Errorf("workspace_id is required"))
	}
	st, err := OpenStore()
	if err != nil {
		return errorJSON(err)
	}

	var title, body string
	resolvedPath := targetPath

	switch kind {
	case store.DraftKindPrompt:
		if targetID == 0 {
			return errorJSON(fmt.Errorf("prompt target requires a target_id"))
		}
		p, err := st.GetPrompt(ctx, targetID)
		if err != nil {
			return errorJSON(fmt.Errorf("load prompt %d: %w", targetID, err))
		}
		title = p.Title
		body = p.Body

	case store.DraftKindWorkflow:
		if strings.TrimSpace(targetPath) == "" {
			return errorJSON(fmt.Errorf("workflow target requires a target_path"))
		}
		if err := validateWorkflowPath(targetPath); err != "" {
			return errorJSON(fmt.Errorf("%s", err))
		}
		raw, rerr := os.ReadFile(targetPath)
		if rerr != nil {
			return errorJSON(fmt.Errorf("read workflow file: %w", rerr))
		}
		body = string(raw)
		// Title is the file's basename without the .workflow.yaml suffix.
		title = strings.TrimSuffix(filepath.Base(targetPath), ".workflow.yaml")

	case store.DraftKindSkill:
		if strings.TrimSpace(targetPath) == "" {
			return errorJSON(fmt.Errorf("skill target requires a target_path"))
		}
		if !isSkillOrAgentPath(targetPath) {
			return errorJSON(fmt.Errorf("refusing %q: not a recognized skill location", targetPath))
		}
		// Normalize to the SKILL.md inside the directory if a dir was passed.
		resolvedPath = targetPath
		if info, statErr := os.Stat(targetPath); statErr == nil && info.IsDir() {
			resolvedPath = resolveSkillFilePath(targetPath)
		}
		raw, rerr := os.ReadFile(resolvedPath)
		if rerr != nil {
			return errorJSON(fmt.Errorf("read skill file: %w", rerr))
		}
		body = string(raw)
		// Title is the parent directory name (skill name lives in the dir).
		title = filepath.Base(filepath.Dir(resolvedPath))

	case store.DraftKindAgent:
		if strings.TrimSpace(targetPath) == "" {
			return errorJSON(fmt.Errorf("agent target requires a target_path"))
		}
		if !isSkillOrAgentPath(targetPath) {
			return errorJSON(fmt.Errorf("refusing %q: not a recognized agent location", targetPath))
		}
		raw, rerr := os.ReadFile(targetPath)
		if rerr != nil {
			return errorJSON(fmt.Errorf("read agent file: %w", rerr))
		}
		body = string(raw)
		// Strip the extension; agents are <name>.md.
		base := filepath.Base(targetPath)
		ext := filepath.Ext(base)
		title = strings.TrimSuffix(base, ext)

	case store.DraftKindCollectors:
		// Collectors drafts always edit the active workspace's
		// collectors.yaml. We resolve the path from the workspace id
		// rather than trusting whatever the caller passed, so a
		// malformed Wails call can't redirect the write somewhere
		// else. The starter file is created on demand.
		if err := collector.EnsureWorkspaceConfig(workspaceID); err != nil {
			return errorJSON(fmt.Errorf("ensure collectors config: %w", err))
		}
		path, perr := collector.WorkspaceConfigPath(workspaceID)
		if perr != nil {
			return errorJSON(perr)
		}
		raw, rerr := os.ReadFile(path)
		if rerr != nil {
			return errorJSON(fmt.Errorf("read collectors config: %w", rerr))
		}
		body = string(raw)
		title = "collectors"
		resolvedPath = path

	default:
		return errorJSON(fmt.Errorf("unsupported draft kind %q", kind))
	}

	id, err := st.CreateDraft(ctx, store.Draft{
		WorkspaceID: workspaceID,
		Kind:        kind,
		Title:       title,
		Body:        body,
		TargetID:    targetID,
		TargetPath:  resolvedPath,
	})
	if err != nil {
		return errorJSON(err)
	}
	d, err := st.GetDraft(ctx, id)
	if err != nil {
		return errorJSON(err)
	}
	return draftJSONWithReadOnly(ctx, st, d)
}

// draftJSONWithReadOnly serializes a draft and tags it with a
// read_only flag when the target_path lives outside any of the
// active workspace's directories. The popup uses this to disable
// the save button on global entities and force the user through
// the "save as new" path instead.
func draftJSONWithReadOnly(ctx context.Context, st *store.Store, d store.Draft) string {
	info := toDraftInfo(d)
	type wire struct {
		DraftInfo
		ReadOnly bool `json:"read_only"`
	}
	out := wire{DraftInfo: info}
	if d.TargetPath != "" && !isWorkspaceWritablePath(ctx, st, d.WorkspaceID, d.TargetPath) {
		out.ReadOnly = true
	}
	b, _ := json.Marshal(out)
	return string(b)
}

// primaryWorkspaceDir returns the first directory associated with a
// workspace, used as the destination for new workflow YAML files.
// Returns "" when the workspace doesn't exist or has no directories.
func primaryWorkspaceDir(ctx context.Context, st *store.Store, workspaceID string) string {
	if workspaceID == "" {
		return ""
	}
	ws, err := st.GetWorkspace(ctx, workspaceID)
	if err != nil || len(ws.Directories) == 0 {
		return ""
	}
	return ws.Directories[0]
}

// isUnderWorkflowsDir returns true when path's parent directory is
// named "workflows" and its grandparent is named ".glitch". Used as a
// safety check before any os.Remove call so a malformed path can't
// nuke files outside the workflows tree.
func isUnderWorkflowsDir(path string) bool {
	abs, err := filepath.Abs(path)
	if err != nil {
		return false
	}
	parent := filepath.Base(filepath.Dir(abs))
	grand := filepath.Base(filepath.Dir(filepath.Dir(abs)))
	return parent == "workflows" && grand == ".glitch"
}

