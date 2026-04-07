// skill_agent_files.go is the file-backed read/write surface for the
// editor popup's skill and agent kinds.
//
// Skills and agents are markdown files discovered by chatui.ScanIndex
// from a small set of well-known locations:
//
//   skills:
//     ~/.claude/skills/<name>/SKILL.md     (global)
//     <workspace>/.claude/skills/<name>/SKILL.md  (workspace)
//   agents:
//     ~/.claude/commands/<name>.md         (global)
//     <workspace>/.claude/commands/<name>.md  (workspace)
//
// The "skill path" returned by ScanIndex points at the *directory*
// (the SKILL.md lives one level inside), so all the helpers in here
// translate skill paths to their inner SKILL.md before reading.
//
// Read access is open to anything that lives under one of these
// known locations. Write access is restricted to workspace paths
// only — global entities are read-only by design, and the popup
// surfaces a "save as new" action that forks a global entity into a
// fresh workspace copy instead of letting the user overwrite ~/.claude.
package glitchd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/8op-org/gl1tch/internal/store"
)

// readableExt is the set of file extensions the editor will load.
// Skills and agents are always markdown — refusing other extensions
// is a cheap belt-and-suspenders against a malformed Wails call
// pointing at something the user didn't intend to edit.
var readableExt = map[string]bool{
	".md":   true,
	".mdx":  true,
	".yaml": true, // for cli:copilot agents
	".yml":  true,
}

// resolveSkillFilePath returns the actual SKILL.md file given a path
// from chatui.ScanIndex. ScanIndex returns the *directory*; the file
// is one level deeper.
func resolveSkillFilePath(p string) string {
	// If the caller already passed the SKILL.md, leave it alone.
	if strings.HasSuffix(p, "SKILL.md") {
		return p
	}
	return filepath.Join(p, "SKILL.md")
}

// isSkillOrAgentPath returns true when path lives under one of the
// known skill/agent locations (workspace or global). Used as the
// safety gate for read-side operations — write paths use a stricter
// "must be under a workspace dir" check.
//
// We deliberately accept paths under $HOME (read access) so the popup
// can preview a global skill before forking it. The write gate then
// blocks save against any global path.
func isSkillOrAgentPath(p string) bool {
	abs, err := filepath.Abs(p)
	if err != nil {
		return false
	}
	// Reject anything that doesn't look like a skill/agent file by
	// extension. SKILL.md, name.md, agent.yaml are all valid.
	target := abs
	if strings.HasSuffix(abs, string(os.PathSeparator)+"SKILL.md") {
		target = abs
	} else if info, err := os.Stat(abs); err == nil && info.IsDir() {
		// Directory passed → expect SKILL.md inside.
		target = filepath.Join(abs, "SKILL.md")
	}
	if !readableExt[strings.ToLower(filepath.Ext(target))] {
		return false
	}
	// Walk up; one of the ancestors must be ".claude", ".stok", or
	// ".copilot". This catches both global ($HOME/.claude/...) and
	// workspace (<dir>/.claude/...) layouts in one check.
	dir := filepath.Dir(target)
	for i := 0; i < 8 && dir != "/" && dir != "."; i++ {
		base := filepath.Base(dir)
		if base == ".claude" || base == ".stok" || base == ".copilot" {
			return true
		}
		dir = filepath.Dir(dir)
	}
	return false
}

// isWorkspaceWritablePath returns true when path is under one of the
// active workspace's directories. Used to decide whether the popup is
// allowed to overwrite the path on save (workspace = yes, global = no).
//
// We check membership against every workspace directory rather than
// just the primary one because users may have multiple repos in a
// workspace and a skill could live in any of them.
func isWorkspaceWritablePath(ctx context.Context, st *store.Store, workspaceID, path string) bool {
	if workspaceID == "" || path == "" {
		return false
	}
	ws, err := st.GetWorkspace(ctx, workspaceID)
	if err != nil {
		return false
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return false
	}
	for _, dir := range ws.Directories {
		dirAbs, err := filepath.Abs(dir)
		if err != nil {
			continue
		}
		// strings.HasPrefix is good enough because both sides are
		// absolute and we add a separator to avoid /foo matching
		// /foobar.
		if strings.HasPrefix(abs, dirAbs+string(os.PathSeparator)) || abs == dirAbs {
			return true
		}
	}
	return false
}

// ReadSkillOrAgentFile returns the markdown contents of a skill or
// agent file. Accepts either a SKILL.md path directly or a skill
// directory (the helper resolves SKILL.md inside it). Returns
// {content: "..."} on success or {error: "..."} on failure.
func ReadSkillOrAgentFile(path string) string {
	if strings.TrimSpace(path) == "" {
		return errorJSON(fmt.Errorf("path is required"))
	}
	if !isSkillOrAgentPath(path) {
		return errorJSON(fmt.Errorf("refusing %q: not a recognized skill/agent location", path))
	}
	resolved := path
	if info, err := os.Stat(path); err == nil && info.IsDir() {
		resolved = resolveSkillFilePath(path)
	}
	raw, err := os.ReadFile(resolved)
	if err != nil {
		return errorJSON(err)
	}
	b, _ := json.Marshal(map[string]string{"content": string(raw)})
	return string(b)
}

// SkillPathForName returns the absolute SKILL.md path where a new
// workspace skill with the given name would be written. Returns ""
// when the workspace has no directories.
func SkillPathForName(ctx context.Context, workspaceID, name string) string {
	st, err := OpenStore()
	if err != nil {
		return ""
	}
	dir := primaryWorkspaceDir(ctx, st, workspaceID)
	if dir == "" {
		return ""
	}
	clean := safeName(name)
	if clean == "" {
		return ""
	}
	return filepath.Join(dir, ".claude", "skills", clean, "SKILL.md")
}

// AgentPathForName returns the absolute path where a new workspace
// agent with the given name would be written. Returns "" when the
// workspace has no directories.
func AgentPathForName(ctx context.Context, workspaceID, name string) string {
	st, err := OpenStore()
	if err != nil {
		return ""
	}
	dir := primaryWorkspaceDir(ctx, st, workspaceID)
	if dir == "" {
		return ""
	}
	clean := safeName(name)
	if clean == "" {
		return ""
	}
	return filepath.Join(dir, ".claude", "commands", clean+".md")
}

// safeName strips a name down to a filename-safe slug (alphanumerics,
// dash, underscore). Used so the popup's title field can't sneak
// path traversal characters into the destination filename.
func safeName(s string) string {
	s = strings.TrimSpace(s)
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z',
			r >= 'A' && r <= 'Z',
			r >= '0' && r <= '9',
			r == '-' || r == '_':
			b.WriteRune(r)
		case r == ' ':
			b.WriteRune('-')
		}
	}
	return b.String()
}

// writeSkillFile writes content to a workspace skill at
// <workspace>/.claude/skills/<name>/SKILL.md. Creates the parent
// directory chain on demand. The path MUST already be validated as
// workspace-writable by the caller.
func writeSkillFile(path, content string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create skill dir: %w", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return fmt.Errorf("write skill: %w", err)
	}
	return nil
}

// writeAgentFile writes content to a workspace agent at
// <workspace>/.claude/commands/<name>.md. Same caller-must-validate
// contract as writeSkillFile.
func writeAgentFile(path, content string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create agent dir: %w", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return fmt.Errorf("write agent: %w", err)
	}
	return nil
}
