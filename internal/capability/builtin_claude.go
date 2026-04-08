package capability

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// ClaudeHistoryCapability is the Go-implemented capability that indexes new
// entries from ~/.claude/history.jsonl. It is the proof that complex stateful
// collectors fit cleanly into the Capability interface — the existing poll
// logic from internal/collector/claude.go ports over verbatim, except instead
// of bulk-indexing directly it emits Doc events through the channel and lets
// the runner do the bulk write.
//
// State (file offset, seeded flag) lives on the struct because the runner
// invokes the same instance once per tick and expects it to remember where
// it left off across calls.
//
// Workspace scoping (Dirs) keeps the same semantics as the original
// collector: an empty Dirs slice means "no filter, index everything," used
// by the headless serve path; a non-empty slice filters entries to those
// whose project path is inside one of the workspace directories.
type ClaudeHistoryCapability struct {
	WorkspaceID string
	Dirs        []string
	// Interval overrides the default 2-minute tick when set. Mostly useful
	// for tests.
	Interval time.Duration

	mu         sync.Mutex
	lastOffset int64
	seeded     bool
}

func (c *ClaudeHistoryCapability) Manifest() Manifest {
	every := c.Interval
	if every == 0 {
		every = 2 * time.Minute
	}
	return Manifest{
		// Named "claude" rather than "claude.history" so the brain
		// popover's existing collector heartbeat lookup keeps working
		// without renaming. The dotted-namespace form was nicer in the
		// abstract but the popover key is what's load-bearing today.
		Name:        "claude",
		Description: "Indexes new entries from ~/.claude/history.jsonl. Workspace-scoped via Dirs filter; emits one document per prompt with session id and project metadata.",
		Category:    "providers.claude",
		Trigger:     Trigger{Mode: TriggerInterval, Every: every},
		Sink:        Sink{Index: true},
	}
}

func (c *ClaudeHistoryCapability) Invoke(ctx context.Context, _ Input) (<-chan Event, error) {
	ch := make(chan Event, 64)
	go func() {
		defer close(ch)
		c.poll(ctx, ch)
	}()
	return ch, nil
}

func (c *ClaudeHistoryCapability) poll(ctx context.Context, ch chan<- Event) {
	home, err := os.UserHomeDir()
	if err != nil {
		ch <- Event{Kind: EventError, Err: fmt.Errorf("home dir: %w", err)}
		return
	}
	historyPath := filepath.Join(home, ".claude", "history.jsonl")

	c.mu.Lock()
	defer c.mu.Unlock()

	// First call: seed the offset to the end of the file so we only index
	// new entries from this point forward. This matches the behaviour of
	// the original collector — backfilling history is a separate concern
	// (the per-project collector handles that).
	if !c.seeded {
		if info, err := os.Stat(historyPath); err == nil {
			c.lastOffset = info.Size()
		}
		c.seeded = true
		return
	}

	f, err := os.Open(historyPath)
	if err != nil {
		if os.IsNotExist(err) {
			return
		}
		ch <- Event{Kind: EventError, Err: err}
		return
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		ch <- Event{Kind: EventError, Err: err}
		return
	}
	if info.Size() <= c.lastOffset {
		return
	}
	if _, err := f.Seek(c.lastOffset, 0); err != nil {
		ch <- Event{Kind: EventError, Err: err}
		return
	}

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 1024*256), 1024*256)
	for scanner.Scan() {
		if ctx.Err() != nil {
			return
		}
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var entry claudeHistoryEntry
		if err := json.Unmarshal(line, &entry); err != nil {
			continue
		}
		if entry.Display == "" {
			continue
		}
		if !pathInDirs(entry.Project, c.Dirs) {
			continue
		}

		project := filepath.Base(entry.Project)
		if project == "." || project == "" {
			project = "unknown"
		}
		ts := time.UnixMilli(entry.Timestamp)

		// Emit a plain map rather than esearch.Event so the capability
		// package stays free of an esearch import. The bulk indexer
		// only cares that the value is JSON-marshalable; field names
		// match the Event struct in internal/esearch/events.go so the
		// resulting documents are query-compatible with everything
		// observer already does.
		doc := map[string]any{
			"type":         "claude.prompt",
			"source":       "claude",
			"workspace_id": c.WorkspaceID,
			"repo":         project,
			"author":       "user",
			"message":      truncateString(entry.Display, 500),
			"body":         entry.Display,
			"metadata": map[string]any{
				"session_id":   entry.SessionID,
				"project_path": entry.Project,
				"has_pastes":   len(entry.PastedContents) > 0,
			},
			"timestamp": ts,
		}
		ch <- Event{Kind: EventDoc, Doc: doc}
	}

	if pos, err := f.Seek(0, 1); err == nil {
		c.lastOffset = pos
	}
}

// claudeHistoryEntry mirrors the JSONL schema in ~/.claude/history.jsonl.
// Duplicated from internal/collector/claude.go on purpose — the new
// capability package owns its own type so the old collector package can be
// deleted independently when the migration is complete.
type claudeHistoryEntry struct {
	Display        string         `json:"display"`
	PastedContents map[string]any `json:"pastedContents"`
	Timestamp      int64          `json:"timestamp"` // unix millis
	Project        string         `json:"project"`
	SessionID      string         `json:"sessionId"`
}

// pathInDirs reports whether path is equal to or under any of dirs. Empty
// dirs means "include everything." Cleaned prefix-match avoids the classic
// /foo matching /foobar bug.
func pathInDirs(path string, dirs []string) bool {
	if len(dirs) == 0 {
		return true
	}
	if path == "" {
		return false
	}
	cleanPath := filepath.Clean(path)
	for _, d := range dirs {
		if d == "" {
			continue
		}
		cleanDir := filepath.Clean(d)
		if cleanPath == cleanDir {
			return true
		}
		if strings.HasPrefix(cleanPath, cleanDir+string(filepath.Separator)) {
			return true
		}
	}
	return false
}

func truncateString(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
