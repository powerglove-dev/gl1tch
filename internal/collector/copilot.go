package collector

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/8op-org/gl1tch/internal/esearch"
)

// CopilotCollector indexes GitHub Copilot CLI history and log files.
type CopilotCollector struct {
	Interval    time.Duration
	WorkspaceID string
}

func (c *CopilotCollector) Name() string { return "copilot" }

func (c *CopilotCollector) Start(ctx context.Context, es *esearch.Client) error {
	if c.Interval == 0 {
		c.Interval = 120 * time.Second
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("copilot collector: home dir: %w", err)
	}
	copilotDir := filepath.Join(home, ".copilot")

	// Track state.
	var lastCommandCount int
	indexedLogs := make(map[string]bool)

	ticker := time.NewTicker(c.Interval)
	defer ticker.Stop()

	// Run once immediately to backfill.
	lastCommandCount = c.pollCommands(ctx, es, copilotDir, lastCommandCount)
	c.pollLogs(ctx, es, copilotDir, indexedLogs)

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			start := time.Now()
			before := lastCommandCount
			beforeLogs := len(indexedLogs)
			lastCommandCount = c.pollCommands(ctx, es, copilotDir, lastCommandCount)
			c.pollLogs(ctx, es, copilotDir, indexedLogs)
			RecordRun("copilot", start,
				(lastCommandCount-before)+(len(indexedLogs)-beforeLogs), nil)
		}
	}
}

// pollCommands reads the command history and indexes new entries.
func (c *CopilotCollector) pollCommands(ctx context.Context, es *esearch.Client, dir string, lastCount int) int {
	path := filepath.Join(dir, "command-history-state.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return lastCount
	}

	var state struct {
		CommandHistory []string `json:"commandHistory"`
	}
	if err := json.Unmarshal(data, &state); err != nil {
		return lastCount
	}

	if len(state.CommandHistory) <= lastCount {
		return len(state.CommandHistory)
	}

	// Index only new commands.
	newCmds := state.CommandHistory[lastCount:]
	var docs []any
	now := time.Now()

	for i, cmd := range newCmds {
		if strings.TrimSpace(cmd) == "" || cmd == "/quit" || cmd == "/clear" {
			continue
		}
		docs = append(docs, esearch.Event{
			Type:    "copilot.command",
			Source:  "copilot",
			Author:  "user",
			Message: cmd,
			Metadata: map[string]any{
				"index": lastCount + i,
			},
			Timestamp: now, // history doesn't have timestamps, use now
		})
	}

	if len(docs) > 0 {
		slog.Info("copilot collector: new commands", "count", len(docs))
		if err := es.BulkIndex(ctx, esearch.IndexEvents, StampWorkspaceID(c.WorkspaceID, docs)); err != nil {
			slog.Warn("copilot collector: index error", "err", err)
			return lastCount // don't advance cursor on error
		}
	}

	return len(state.CommandHistory)
}

// copilot log line regex: 2026-04-06T15:02:13.400Z [INFO] message
var copilotLogRe = regexp.MustCompile(`^(\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}\.\d{3}Z)\s+\[(\w+)]\s+(.+)$`)

// pollLogs reads Copilot CLI log files and indexes interesting entries.
func (c *CopilotCollector) pollLogs(ctx context.Context, es *esearch.Client, dir string, indexed map[string]bool) {
	logsDir := filepath.Join(dir, "logs")
	files, err := filepath.Glob(filepath.Join(logsDir, "process-*.log"))
	if err != nil {
		return
	}

	for _, path := range files {
		if indexed[path] {
			continue
		}

		docs := c.parseLogFile(path)
		if len(docs) > 0 {
			if err := es.BulkIndex(ctx, esearch.IndexEvents, StampWorkspaceID(c.WorkspaceID, docs)); err != nil {
				slog.Warn("copilot collector: log index error", "file", filepath.Base(path), "err", err)
				continue
			}
			slog.Info("copilot collector: indexed log", "file", filepath.Base(path), "events", len(docs))
		}
		indexed[path] = true
	}
}

func (c *CopilotCollector) parseLogFile(path string) []any {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()

	var docs []any
	scanner := bufio.NewScanner(f)

	for scanner.Scan() {
		line := scanner.Text()
		matches := copilotLogRe.FindStringSubmatch(line)
		if len(matches) < 4 {
			continue
		}

		ts, _ := time.Parse("2006-01-02T15:04:05.000Z", matches[1])
		level := matches[2]
		message := matches[3]

		// Only index interesting log entries.
		if !isInterestingCopilotLog(level, message) {
			continue
		}

		docs = append(docs, esearch.Event{
			Type:    "copilot.log",
			Source:  "copilot",
			Author:  "copilot-cli",
			Message: message,
			Metadata: map[string]any{
				"level":    level,
				"log_file": filepath.Base(path),
			},
			Timestamp: ts,
		})
	}

	return docs
}

// isInterestingCopilotLog filters out noise from Copilot logs.
func isInterestingCopilotLog(level, message string) bool {
	// Always include errors and warnings.
	if level == "ERROR" || level == "WARNING" {
		// Skip the MCP connection noise that's actually fine.
		if strings.Contains(message, "Starting remote MCP client") ||
			strings.Contains(message, "Creating MCP client") ||
			strings.Contains(message, "Connecting MCP client") ||
			strings.Contains(message, "MCP client for") ||
			strings.Contains(message, "GitHub MCP server configured") {
			return false
		}
		return true
	}

	// Interesting INFO messages.
	interesting := []string{
		"Starting Copilot CLI",
		"Using default model",
		"Welcome ",
		"Workspace initialized",
	}
	for _, prefix := range interesting {
		if strings.HasPrefix(message, prefix) || strings.Contains(message, prefix) {
			return true
		}
	}

	return false
}
