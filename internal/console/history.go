package console

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// historyEntry is a single line in a session's history.jsonl file.
type historyEntry struct {
	TS   time.Time `json:"ts"`
	Role string    `json:"role"` // "user" | "assistant" | "system"
	Text string    `json:"text"`
}

// historyDir returns the directory that holds history.jsonl for the named session.
// It always respects GLITCH_CONFIG_DIR (set in tests for isolation).
func historyDir(cfgDir, sessionName string) string {
	return filepath.Join(cfgDir, "sessions", sessionName)
}

// historyPath returns the full path to the history.jsonl file for a session.
func historyPath(cfgDir, sessionName string) string {
	return filepath.Join(historyDir(cfgDir, sessionName), "history.jsonl")
}

// appendHistoryCmd returns a tea.Cmd that asynchronously appends user and
// assistant entries to the session's history.jsonl file. It never blocks the
// BubbleTea Update loop.
func appendHistoryCmd(cfgDir, sessionName, userText, assistantText string) tea.Cmd {
	if cfgDir == "" || sessionName == "" {
		return nil
	}
	// Capture values before entering the goroutine — safe to read from Update.
	dir := historyDir(cfgDir, sessionName)
	path := historyPath(cfgDir, sessionName)
	now := time.Now().UTC()
	entries := []historyEntry{
		{TS: now, Role: "user", Text: userText},
		{TS: now, Role: "assistant", Text: assistantText},
	}
	return func() tea.Msg {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil
		}
		f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
		if err != nil {
			return nil
		}
		defer f.Close()
		enc := json.NewEncoder(f)
		for _, e := range entries {
			_ = enc.Encode(e) //nolint:errcheck
		}
		return nil
	}
}

// appendPipelineHistoryCmd returns a tea.Cmd that asynchronously appends a
// system-role pipeline completion entry to the session's history.jsonl.
func appendPipelineHistoryCmd(cfgDir, sessionName, pipelineName string, runID string, failed bool) tea.Cmd {
	if cfgDir == "" || sessionName == "" {
		return nil
	}
	dir := historyDir(cfgDir, sessionName)
	path := historyPath(cfgDir, sessionName)
	status := "completed"
	if failed {
		status = "failed"
	}
	text := "Pipeline '" + pipelineName + "' " + status + "."
	if runID != "" {
		text += " Run #" + runID + "."
	}
	entry := historyEntry{TS: time.Now().UTC(), Role: "system", Text: text}
	return func() tea.Msg {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil
		}
		f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
		if err != nil {
			return nil
		}
		defer f.Close()
		_ = json.NewEncoder(f).Encode(entry) //nolint:errcheck
		return nil
	}
}

// loadHistory reads the last N entries from history.jsonl (capped at maxEntries)
// and returns them as glitchTurn slice for injection into a session's turns.
// "system" role entries are skipped (they are informational only).
// Returns nil without error if the file does not exist.
func loadHistory(cfgDir, sessionName string, maxEntries int) ([]glitchTurn, error) {
	path := historyPath(cfgDir, sessionName)
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer f.Close()

	// Read all lines into a ring of at most maxEntries.
	var all []historyEntry
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var e historyEntry
		if err := json.Unmarshal(line, &e); err != nil {
			continue // skip malformed lines
		}
		all = append(all, e)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}

	// Take the last maxEntries entries.
	if len(all) > maxEntries {
		all = all[len(all)-maxEntries:]
	}

	// Convert to glitchTurn, dropping system-role entries.
	var turns []glitchTurn
	for _, e := range all {
		if e.Role == "system" {
			continue
		}
		turns = append(turns, glitchTurn{role: e.Role, text: e.Text})
	}
	return turns, nil
}
