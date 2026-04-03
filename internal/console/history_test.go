package console

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestAppendHistoryCmd_WritesUserAndAssistant(t *testing.T) {
	dir := t.TempDir()
	cmd := appendHistoryCmd(dir, "main", "hello world", "hi there")
	if cmd == nil {
		t.Fatal("appendHistoryCmd returned nil for valid inputs")
	}
	// Execute the Cmd synchronously in the test.
	msg := cmd()
	_ = msg // always nil by design

	p := historyPath(dir, "main")
	data, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("reading history file: %v", err)
	}
	content := string(data)
	if !strings.Contains(content, "hello world") {
		t.Errorf("user text not found in history file: %q", content)
	}
	if !strings.Contains(content, "hi there") {
		t.Errorf("assistant text not found in history file: %q", content)
	}
	if !strings.Contains(content, `"user"`) {
		t.Errorf(`"user" role not found in history file: %q`, content)
	}
	if !strings.Contains(content, `"assistant"`) {
		t.Errorf(`"assistant" role not found in history file: %q`, content)
	}
}

func TestAppendHistoryCmd_ReturnsNilForEmptyInputs(t *testing.T) {
	if appendHistoryCmd("", "main", "u", "a") != nil {
		t.Error("expected nil when cfgDir is empty")
	}
	if appendHistoryCmd("/tmp", "", "u", "a") != nil {
		t.Error("expected nil when sessionName is empty")
	}
}

func TestLoadHistory_ReturnsNilForMissingFile(t *testing.T) {
	dir := t.TempDir()
	turns, err := loadHistory(dir, "nonexistent", 20)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if turns != nil {
		t.Errorf("expected nil turns for missing file, got %v", turns)
	}
}

func TestLoadHistory_ReadsAndConvertsToTurns(t *testing.T) {
	dir := t.TempDir()
	// Write two history entries manually.
	cmd := appendHistoryCmd(dir, "main", "question", "answer")
	cmd() //nolint:errcheck

	turns, err := loadHistory(dir, "main", 20)
	if err != nil {
		t.Fatalf("loadHistory error: %v", err)
	}
	if len(turns) != 2 {
		t.Fatalf("expected 2 turns, got %d", len(turns))
	}
	if turns[0].role != "user" || turns[0].text != "question" {
		t.Errorf("turn[0] = %+v, want role=user text=question", turns[0])
	}
	if turns[1].role != "assistant" || turns[1].text != "answer" {
		t.Errorf("turn[1] = %+v, want role=assistant text=answer", turns[1])
	}
}

func TestLoadHistory_CapsAtMaxEntries(t *testing.T) {
	dir := t.TempDir()
	// Write 15 pairs = 30 entries total.
	for i := 0; i < 15; i++ {
		appendHistoryCmd(dir, "main", "u", "a")()
	}
	// Request at most 6 entries.
	turns, err := loadHistory(dir, "main", 6)
	if err != nil {
		t.Fatalf("loadHistory error: %v", err)
	}
	if len(turns) > 6 {
		t.Errorf("expected at most 6 turns, got %d", len(turns))
	}
}

func TestLoadHistory_SkipsSystemEntries(t *testing.T) {
	dir := t.TempDir()
	// Write a pipeline history entry directly.
	appendPipelineHistoryCmd(dir, "main", "my-pipeline", "pipe-001", false)()
	// Write a regular exchange.
	appendHistoryCmd(dir, "main", "user question", "assistant answer")()

	turns, err := loadHistory(dir, "main", 20)
	if err != nil {
		t.Fatalf("loadHistory error: %v", err)
	}
	// System entry should be skipped; only user+assistant remain.
	for _, t2 := range turns {
		if t2.role == "system" {
			t.Errorf("system role should be filtered out, got %+v", t2)
		}
	}
	if len(turns) != 2 {
		t.Errorf("expected 2 turns (no system), got %d", len(turns))
	}
}

func TestAppendPipelineHistoryCmd_WritesSystemEntry(t *testing.T) {
	dir := t.TempDir()
	cmd := appendPipelineHistoryCmd(dir, "work", "backup", "job-42", false)
	if cmd == nil {
		t.Fatal("appendPipelineHistoryCmd returned nil")
	}
	cmd()

	data, err := os.ReadFile(historyPath(dir, "work"))
	if err != nil {
		t.Fatalf("reading history file: %v", err)
	}
	content := string(data)
	if !strings.Contains(content, "backup") {
		t.Errorf("pipeline name not found in history: %q", content)
	}
	if !strings.Contains(content, `"system"`) {
		t.Errorf(`"system" role not found in history: %q`, content)
	}
	if !strings.Contains(content, "completed") {
		t.Errorf(`"completed" not found in history: %q`, content)
	}
}

func TestAppendPipelineHistoryCmd_FailedStatus(t *testing.T) {
	dir := t.TempDir()
	appendPipelineHistoryCmd(dir, "work", "deploy", "job-99", true)()
	data, _ := os.ReadFile(historyPath(dir, "work"))
	if !strings.Contains(string(data), "failed") {
		t.Errorf("expected 'failed' in history for a failed pipeline: %q", string(data))
	}
}

func TestHistoryPath_RespectsSessionDir(t *testing.T) {
	path := historyPath("/cfg", "mysession")
	expected := filepath.Join("/cfg", "sessions", "mysession", "history.jsonl")
	if path != expected {
		t.Errorf("historyPath = %q, want %q", path, expected)
	}
}

func TestAppendHistoryCmd_Appends(t *testing.T) {
	dir := t.TempDir()
	appendHistoryCmd(dir, "main", "first", "reply1")()
	appendHistoryCmd(dir, "main", "second", "reply2")()

	turns, err := loadHistory(dir, "main", 20)
	if err != nil {
		t.Fatalf("loadHistory error: %v", err)
	}
	if len(turns) != 4 {
		t.Errorf("expected 4 turns, got %d", len(turns))
	}
}

func TestLoadHistory_HandlesMalformedLines(t *testing.T) {
	dir := t.TempDir()
	p := historyPath(dir, "main")
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	// Write one valid line and one malformed line.
	content := `{"ts":"` + time.Now().UTC().Format(time.RFC3339Nano) + `","role":"user","text":"ok"}` + "\n"
	content += "not-json\n"
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	turns, err := loadHistory(dir, "main", 20)
	if err != nil {
		t.Fatalf("loadHistory returned error on malformed input: %v", err)
	}
	// Only the valid line should produce a turn.
	if len(turns) != 1 {
		t.Errorf("expected 1 turn (malformed line skipped), got %d", len(turns))
	}
}
