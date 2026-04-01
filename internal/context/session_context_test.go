package context_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	appcontext "github.com/8op-org/gl1tch/internal/context"
)

func TestGetRecentContextEmptyDir(t *testing.T) {
	homeDir := t.TempDir()
	msgs := appcontext.GetRecentContext(homeDir, "/projects/myapp")
	if msgs == nil {
		t.Fatal("expected non-nil slice")
	}
	if len(msgs) != 0 {
		t.Fatalf("expected 0 messages from empty dir, got %d", len(msgs))
	}
}

func TestGetRecentContextReadsMessages(t *testing.T) {
	homeDir := t.TempDir()
	cwd := "/projects/myapp"
	encoded := strings.ReplaceAll(cwd, "/", "-")
	sessDir := filepath.Join(homeDir, ".stok", "sessions", "history", encoded)
	if err := os.MkdirAll(sessDir, 0755); err != nil {
		t.Fatal(err)
	}

	msgs := []map[string]string{
		{"role": "user", "content": "fix the login bug"},
		{"role": "assistant", "content": "sure, looking at auth.go"},
	}
	data, _ := json.Marshal(msgs)
	if err := os.WriteFile(filepath.Join(sessDir, "sess1.json"), data, 0644); err != nil {
		t.Fatal(err)
	}

	results := appcontext.GetRecentContext(homeDir, cwd)
	if len(results) == 0 {
		t.Fatal("expected messages from session file")
	}
	if results[0] != "user: fix the login bug" {
		t.Fatalf("unexpected first message: %s", results[0])
	}
}

func TestGetRecentContextLimitsMessages(t *testing.T) {
	homeDir := t.TempDir()
	cwd := "/projects/myapp"
	encoded := strings.ReplaceAll(cwd, "/", "-")
	sessDir := filepath.Join(homeDir, ".stok", "sessions", "history", encoded)
	_ = os.MkdirAll(sessDir, 0755)

	// Write 30 messages — should be capped at 20.
	var msgs []map[string]string
	for i := 0; i < 30; i++ {
		msgs = append(msgs, map[string]string{"role": "user", "content": "message"})
	}
	data, _ := json.Marshal(msgs)
	_ = os.WriteFile(filepath.Join(sessDir, "sess1.json"), data, 0644)

	results := appcontext.GetRecentContext(homeDir, cwd)
	if len(results) > 20 {
		t.Fatalf("expected at most 20 messages, got %d", len(results))
	}
}
