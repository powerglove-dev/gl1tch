// Package brainaudit records brain injection audit entries after each agent step.
package brainaudit

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// DefaultAuditPath returns the default path for the brain audit log.
func DefaultAuditPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(".orcai", "brain_audit.jsonl")
	}
	return filepath.Join(home, ".orcai", "brain_audit.jsonl")
}

// AuditEntry records a single brain injection event for an agent step.
type AuditEntry struct {
	RunID              string   `json:"run_id"`
	StepName           string   `json:"step_name"`
	BrainNotesInjected []string `json:"brain_notes_injected"`
	PromptLengthChars  int      `json:"prompt_length_chars"`
	Timestamp          string   `json:"timestamp"`
}

// Append appends a JSON line for entry to the audit log at DefaultAuditPath.
// Best-effort: returns an error but callers should not fail the step on error.
func Append(entry AuditEntry) error {
	return AppendTo(DefaultAuditPath(), entry)
}

// AppendTo appends a JSON line for entry to the audit log at the given path.
func AppendTo(path string, entry AuditEntry) error {
	if entry.Timestamp == "" {
		entry.Timestamp = time.Now().Format(time.RFC3339)
	}
	if entry.BrainNotesInjected == nil {
		entry.BrainNotesInjected = []string{}
	}

	data, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("brainaudit: marshal entry: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("brainaudit: mkdir: %w", err)
	}

	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("brainaudit: open %q: %w", path, err)
	}
	defer f.Close()

	_, err = fmt.Fprintf(f, "%s\n", data)
	return err
}
