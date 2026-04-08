package capability

import (
	"bufio"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// ClaudeProjectsCapability ports ClaudeProjectCollector from
// internal/collector/claude.go. It scans ~/.claude/projects/ for per-session
// JSONL files and emits one document per session entry on first sight of each
// file. State (the set of files already indexed) lives on the struct so each
// tick only walks new files.
//
// Workspace scoping uses the same encoded-directory-name comparison as the
// original — Claude Code names project dirs by replacing "/" with "-", which
// is unambiguous in the encode direction even though decoding is lossy.
//
// Named "claude-projects" so the brain popover heartbeat lookup keeps working
// without renaming, matching the legacy collector name.
type ClaudeProjectsCapability struct {
	WorkspaceID string
	Dirs        []string
	Interval    time.Duration

	mu      sync.Mutex
	indexed map[string]bool
}

func (c *ClaudeProjectsCapability) Manifest() Manifest {
	every := c.Interval
	if every == 0 {
		every = 5 * time.Minute
	}
	return Manifest{
		Name:        "claude-projects",
		Description: "Indexes per-project Claude Code session JSONL files from ~/.claude/projects/. Workspace-scoped via Dirs filter using Claude's encoded-directory-name format. Emits one document per assistant or user turn.",
		Category:    "providers.claude",
		Trigger:     Trigger{Mode: TriggerInterval, Every: every},
		Sink:        Sink{Index: true},
		// Claude session turns go into the dedicated claude history
		// index alongside the history-file capability, not into
		// glitch-events. Same rationale: high-volume, search-worthy,
		// but not a coordination signal.
		Invocation: Invocation{Index: "glitch-claude-history"},
	}
}

func (c *ClaudeProjectsCapability) Invoke(ctx context.Context, _ Input) (<-chan Event, error) {
	ch := make(chan Event, 64)
	go func() {
		defer close(ch)
		c.poll(ctx, ch)
	}()
	return ch, nil
}

func (c *ClaudeProjectsCapability) poll(ctx context.Context, ch chan<- Event) {
	home, err := os.UserHomeDir()
	if err != nil {
		ch <- Event{Kind: EventError, Err: err}
		return
	}
	dir := filepath.Join(home, ".claude", "projects")

	c.mu.Lock()
	defer c.mu.Unlock()
	if c.indexed == nil {
		c.indexed = make(map[string]bool)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}

	// Pre-encode workspace dirs for comparison against on-disk names.
	// Encode is lossless; decode is lossy because path components can
	// contain hyphens. The original collector documents this at length.
	encodedDirs := make(map[string]bool, len(c.Dirs))
	for _, d := range c.Dirs {
		if d == "" {
			continue
		}
		encodedDirs[encodeClaudeDirName(filepath.Clean(d))] = true
	}

	for _, projDir := range entries {
		if ctx.Err() != nil {
			return
		}
		if !projDir.IsDir() {
			continue
		}
		if len(encodedDirs) > 0 && !encodedDirs[projDir.Name()] {
			continue
		}
		projectName := decodeClaudeProjectName(projDir.Name())
		projPath := filepath.Join(dir, projDir.Name())
		files, err := filepath.Glob(filepath.Join(projPath, "*.jsonl"))
		if err != nil {
			continue
		}
		for _, f := range files {
			if c.indexed[f] {
				continue
			}
			c.parseSessionFile(f, projectName, ch)
			c.indexed[f] = true
		}
	}
}

type claudeProjectEntry struct {
	Type      string `json:"type"`
	Operation string `json:"operation"`
	Timestamp string `json:"timestamp"`
	SessionID string `json:"sessionId"`
	Content   string `json:"content"`
}

func (c *ClaudeProjectsCapability) parseSessionFile(path, project string, ch chan<- Event) {
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 1024*256), 1024*256)
	sessionID := strings.TrimSuffix(filepath.Base(path), ".jsonl")
	for scanner.Scan() {
		var entry claudeProjectEntry
		if err := json.Unmarshal(scanner.Bytes(), &entry); err != nil {
			continue
		}
		if entry.Content == "" || entry.Type == "" {
			continue
		}
		ts, _ := time.Parse(time.RFC3339Nano, entry.Timestamp)
		if ts.IsZero() {
			ts = time.Now()
		}
		ch <- Event{Kind: EventDoc, Doc: map[string]any{
			"type":         "claude.session." + entry.Type,
			"source":       "claude",
			"workspace_id": c.WorkspaceID,
			"repo":         project,
			"author":       "claude-code",
			"message":      truncateString(entry.Content, 500),
			"body":         entry.Content,
			"metadata": map[string]any{
				"session_id": sessionID,
				"operation":  entry.Operation,
				"entry_type": entry.Type,
			},
			"timestamp": ts,
		}}
	}
}

// decodeClaudeProjectName extracts a human-readable project label from
// Claude Code's encoded directory name (e.g. "-Users-stokes-Projects-gl1tch"
// → "gl1tch"). Used only as a display label on emitted docs — workspace
// scoping goes the other direction via encodeClaudeDirName.
func decodeClaudeProjectName(encoded string) string {
	parts := strings.Split(encoded, "-")
	if len(parts) > 0 {
		return parts[len(parts)-1]
	}
	return encoded
}

// encodeClaudeDirName converts an absolute path into Claude Code's directory
// naming convention by replacing "/" with "-". Lossless and dependency-free,
// which is why workspace scoping compares encoded forms instead of decoding
// directory names back to paths.
func encodeClaudeDirName(absPath string) string {
	return strings.ReplaceAll(absPath, "/", "-")
}
