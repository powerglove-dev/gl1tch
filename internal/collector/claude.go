package collector

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/8op-org/gl1tch/internal/esearch"
)

// ClaudeCollector indexes Claude Code conversation history from ~/.claude/.
type ClaudeCollector struct {
	// Interval is how often to poll for new entries. Defaults to 120s.
	Interval time.Duration
	// WorkspaceID is stamped on every indexed event so brain queries
	// can scope to one workspace's claude history.
	WorkspaceID string
}

func (c *ClaudeCollector) Name() string { return "claude" }

// claudeHistoryEntry matches the JSONL schema in ~/.claude/history.jsonl.
type claudeHistoryEntry struct {
	Display        string         `json:"display"`
	PastedContents map[string]any `json:"pastedContents"`
	Timestamp      int64          `json:"timestamp"` // unix millis
	Project        string         `json:"project"`
	SessionID      string         `json:"sessionId"`
}

func (c *ClaudeCollector) Start(ctx context.Context, es *esearch.Client) error {
	if c.Interval == 0 {
		c.Interval = 120 * time.Second
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("claude collector: home dir: %w", err)
	}
	historyPath := filepath.Join(home, ".claude", "history.jsonl")

	var lastOffset int64 // track how far we've read

	// Seed the offset to the end of the file so we only index new entries.
	if info, err := os.Stat(historyPath); err == nil {
		lastOffset = info.Size()
		slog.Info("claude collector: seeded offset", "bytes", lastOffset)
	}

	ticker := time.NewTicker(c.Interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			start := time.Now()
			newOffset, err := c.poll(ctx, es, historyPath, lastOffset)
			// Heartbeat: bytes-of-new-data is a reasonable stand-in
			// for "indexed count" since the .jsonl is line-delimited.
			indexed := int(newOffset - lastOffset)
			if indexed < 0 {
				indexed = 0
			}
			RecordRun("claude", start, indexed, err)
			if err != nil {
				slog.Warn("claude collector: poll error", "err", err)
				continue
			}
			lastOffset = newOffset
		}
	}
}

func (c *ClaudeCollector) poll(ctx context.Context, es *esearch.Client, path string, offset int64) (int64, error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return offset, nil
		}
		return offset, err
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return offset, err
	}
	if info.Size() <= offset {
		return offset, nil // no new data
	}

	if _, err := f.Seek(offset, 0); err != nil {
		return offset, err
	}

	var docs []any
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 1024*256), 1024*256)

	for scanner.Scan() {
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

		// Convert timestamp from unix millis.
		ts := time.UnixMilli(entry.Timestamp)

		// Extract project name from path.
		project := filepath.Base(entry.Project)
		if project == "." || project == "" {
			project = "unknown"
		}

		docs = append(docs, esearch.Event{
			Type:    "claude.prompt",
			Source:  "claude",
			Repo:    project,
			Author:  "user",
			Message: truncate(entry.Display, 500),
			Body:    entry.Display,
			Metadata: map[string]any{
				"session_id":     entry.SessionID,
				"project_path":   entry.Project,
				"has_pastes":     len(entry.PastedContents) > 0,
			},
			Timestamp: ts,
		})
	}

	newOffset, _ := f.Seek(0, 1) // current position

	if len(docs) > 0 {
		slog.Info("claude collector: new prompts", "count", len(docs))
		if err := es.BulkIndex(ctx, esearch.IndexEvents, StampWorkspaceID(c.WorkspaceID, docs)); err != nil {
			return offset, fmt.Errorf("bulk index: %w", err)
		}
	}

	return newOffset, nil
}

// Also index Claude Code project conversation files for richer context.

// ClaudeProjectCollector indexes per-project session JSONL files from
// ~/.claude/projects/. Runs once on startup to backfill, then watches
// for new files.
type ClaudeProjectCollector struct {
	Interval    time.Duration
	WorkspaceID string
}

func (c *ClaudeProjectCollector) Name() string { return "claude-projects" }

type claudeProjectEntry struct {
	Type      string `json:"type"`
	Operation string `json:"operation"`
	Timestamp string `json:"timestamp"` // ISO 8601
	SessionID string `json:"sessionId"`
	Content   string `json:"content"`
}

func (c *ClaudeProjectCollector) Start(ctx context.Context, es *esearch.Client) error {
	if c.Interval == 0 {
		c.Interval = 300 * time.Second // every 5 min
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("claude-projects: home dir: %w", err)
	}
	projectsDir := filepath.Join(home, ".claude", "projects")

	// Track which files we've already processed.
	indexed := make(map[string]bool)

	ticker := time.NewTicker(c.Interval)
	defer ticker.Stop()

	// Run once immediately.
	start := time.Now()
	beforeCount := len(indexed)
	c.pollProjects(ctx, es, projectsDir, indexed)
	RecordRun("claude-projects", start, len(indexed)-beforeCount, nil)

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			start := time.Now()
			before := len(indexed)
			c.pollProjects(ctx, es, projectsDir, indexed)
			RecordRun("claude-projects", start, len(indexed)-before, nil)
		}
	}
}

func (c *ClaudeProjectCollector) pollProjects(ctx context.Context, es *esearch.Client, dir string, indexed map[string]bool) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}

	for _, projDir := range entries {
		if !projDir.IsDir() {
			continue
		}
		projectName := decodeClaudeProjectName(projDir.Name())
		projPath := filepath.Join(dir, projDir.Name())

		files, err := filepath.Glob(filepath.Join(projPath, "*.jsonl"))
		if err != nil {
			continue
		}

		for _, f := range files {
			if indexed[f] {
				continue
			}

			docs := c.parseSessionFile(f, projectName)
			if len(docs) > 0 {
				if err := es.BulkIndex(ctx, esearch.IndexEvents, StampWorkspaceID(c.WorkspaceID, docs)); err != nil {
					slog.Warn("claude-projects: index error", "file", filepath.Base(f), "err", err)
					continue
				}
				slog.Info("claude-projects: indexed session", "project", projectName, "events", len(docs))
			}
			indexed[f] = true
		}
	}
}

func (c *ClaudeProjectCollector) parseSessionFile(path, project string) []any {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()

	var docs []any
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

		docs = append(docs, esearch.Event{
			Type:    "claude.session." + entry.Type,
			Source:  "claude",
			Repo:    project,
			Author:  "claude-code",
			Message: truncate(entry.Content, 500),
			Body:    entry.Content,
			Metadata: map[string]any{
				"session_id": sessionID,
				"operation":  entry.Operation,
				"entry_type": entry.Type,
			},
			Timestamp: ts,
		})
	}

	return docs
}

// decodeClaudeProjectName converts the encoded directory name back to a
// human-readable project name. e.g. "-Users-stokes-Projects-gl1tch" → "gl1tch"
func decodeClaudeProjectName(encoded string) string {
	parts := strings.Split(encoded, "-")
	if len(parts) > 0 {
		return parts[len(parts)-1]
	}
	return encoded
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
