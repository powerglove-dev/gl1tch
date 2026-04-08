package capability

import (
	"bufio"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

// CopilotCapability ports CopilotCollector from internal/collector/copilot.go.
// It indexes both the GitHub Copilot CLI command history (a JSON state file)
// and the per-process log files. State (last command count, set of indexed
// log paths) lives on the struct because each tick advances the cursors.
//
// Named "copilot" rather than "copilot.history" so the brain popover's
// existing collector heartbeat lookup keeps working without renaming.
type CopilotCapability struct {
	Interval    time.Duration
	WorkspaceID string

	mu               sync.Mutex
	lastCommandCount int
	indexedLogs      map[string]bool
}

func (c *CopilotCapability) Manifest() Manifest {
	every := c.Interval
	if every == 0 {
		every = 2 * time.Minute
	}
	return Manifest{
		Name:        "copilot",
		Description: "Indexes GitHub Copilot CLI command history and per-process log entries from ~/.copilot/. Filters log lines through an interest predicate so MCP startup noise stays out of the brain.",
		Category:    "providers.copilot",
		Trigger:     Trigger{Mode: TriggerInterval, Every: every},
		Sink:        Sink{Index: true},
	}
}

func (c *CopilotCapability) Invoke(ctx context.Context, _ Input) (<-chan Event, error) {
	ch := make(chan Event, 64)
	go func() {
		defer close(ch)
		c.poll(ctx, ch)
	}()
	return ch, nil
}

func (c *CopilotCapability) poll(ctx context.Context, ch chan<- Event) {
	home, err := os.UserHomeDir()
	if err != nil {
		ch <- Event{Kind: EventError, Err: err}
		return
	}
	dir := filepath.Join(home, ".copilot")

	c.mu.Lock()
	defer c.mu.Unlock()
	if c.indexedLogs == nil {
		c.indexedLogs = make(map[string]bool)
	}

	c.pollCommands(ctx, dir, ch)
	c.pollLogs(ctx, dir, ch)
}

func (c *CopilotCapability) pollCommands(_ context.Context, dir string, ch chan<- Event) {
	path := filepath.Join(dir, "command-history-state.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	var state struct {
		CommandHistory []string `json:"commandHistory"`
	}
	if err := json.Unmarshal(data, &state); err != nil {
		return
	}
	if len(state.CommandHistory) <= c.lastCommandCount {
		return
	}
	newCmds := state.CommandHistory[c.lastCommandCount:]
	now := time.Now()
	for i, cmd := range newCmds {
		if strings.TrimSpace(cmd) == "" || cmd == "/quit" || cmd == "/clear" {
			continue
		}
		ch <- Event{Kind: EventDoc, Doc: map[string]any{
			"type":         "copilot.command",
			"source":       "copilot",
			"workspace_id": c.WorkspaceID,
			"author":       "user",
			"message":      cmd,
			"metadata": map[string]any{
				"index": c.lastCommandCount + i,
			},
			"timestamp": now,
		}}
	}
	c.lastCommandCount = len(state.CommandHistory)
}

// copilotLogRe matches lines like "2026-04-06T15:02:13.400Z [INFO] message".
var copilotLogRe = regexp.MustCompile(`^(\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}\.\d{3}Z)\s+\[(\w+)]\s+(.+)$`)

func (c *CopilotCapability) pollLogs(ctx context.Context, dir string, ch chan<- Event) {
	logsDir := filepath.Join(dir, "logs")
	files, err := filepath.Glob(filepath.Join(logsDir, "process-*.log"))
	if err != nil {
		return
	}
	for _, path := range files {
		if ctx.Err() != nil {
			return
		}
		if c.indexedLogs[path] {
			continue
		}
		c.parseLogFile(path, ch)
		c.indexedLogs[path] = true
	}
}

func (c *CopilotCapability) parseLogFile(path string, ch chan<- Event) {
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()
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
		if !isInterestingCopilotLog(level, message) {
			continue
		}
		ch <- Event{Kind: EventDoc, Doc: map[string]any{
			"type":         "copilot.log",
			"source":       "copilot",
			"workspace_id": c.WorkspaceID,
			"author":       "copilot-cli",
			"message":      message,
			"metadata": map[string]any{
				"level":    level,
				"log_file": filepath.Base(path),
			},
			"timestamp": ts,
		}}
	}
}

// isInterestingCopilotLog filters out the MCP startup noise that's actually
// fine and keeps real warnings, errors, and lifecycle events. Ported as-is
// from the original collector — the heuristic is "noisy but useful enough."
func isInterestingCopilotLog(level, message string) bool {
	if level == "ERROR" || level == "WARNING" {
		if strings.Contains(message, "Starting remote MCP client") ||
			strings.Contains(message, "Creating MCP client") ||
			strings.Contains(message, "Connecting MCP client") ||
			strings.Contains(message, "MCP client for") ||
			strings.Contains(message, "GitHub MCP server configured") {
			return false
		}
		return true
	}
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
