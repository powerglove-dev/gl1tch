package bootstrap_test

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/8op-org/gl1tch/internal/bootstrap"
)

func TestWriteTmuxConf(t *testing.T) {
	dir := t.TempDir()
	confPath, err := bootstrap.WriteTmuxConf(dir, "/fake/orcai")
	if err != nil {
		t.Fatalf("WriteTmuxConf: %v", err)
	}
	data, err := os.ReadFile(confPath)
	if err != nil {
		t.Fatalf("tmux.conf not written: %v", err)
	}
	if len(data) == 0 {
		t.Error("tmux.conf is empty")
	}
	expected := filepath.Join(dir, "tmux.conf")
	if confPath != expected {
		t.Errorf("confPath = %q, want %q", confPath, expected)
	}
	if !strings.Contains(string(data), "status-position bottom") {
		t.Error("tmux.conf missing status-position bottom")
	}
}

func TestBuildTmuxConf_Keybindings(t *testing.T) {
	dir := t.TempDir()
	_, err := bootstrap.WriteTmuxConf(dir, "/fake/orcai")
	if err != nil {
		t.Fatalf("WriteTmuxConf: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "tmux.conf"))
	if err != nil {
		t.Fatalf("read tmux.conf: %v", err)
	}
	conf := string(data)
	// Strip tmux color directives (#[fg=...]) from the conf for plain-text assertions.
	colorRe := regexp.MustCompile(`#\[[^\]]*\]`)
	plain := colorRe.ReplaceAllString(conf, "")

	// ctrl+space leader must be present.
	if !strings.Contains(conf, "C-Space") {
		t.Error("tmux.conf missing C-Space leader binding")
	}
	// Backtick leader must be absent.
	if strings.Contains(conf, "bind-key -n `") {
		t.Error("tmux.conf still contains backtick leader binding")
	}
	// Global ESC binding must be absent.
	if strings.Contains(conf, "bind-key -n Escape select-pane") {
		t.Error("tmux.conf still contains global ESC intercept")
	}
	// Status bar hints: ^spc j jump must be present; ^spc h help must be gone.
	if !strings.Contains(plain, "^spc j jump") {
		t.Error("tmux.conf window-status-current-format missing '^spc j jump' hint")
	}
	if strings.Contains(plain, "^spc h help") {
		t.Error("tmux.conf window-status-current-format still contains removed '^spc h help' hint")
	}
	// Status bar must be centred with empty left/right.
	if !strings.Contains(conf, `status-justify centre`) {
		t.Error("tmux.conf missing 'status-justify centre'")
	}
	if strings.Contains(conf, "GLITCH") {
		t.Error("tmux.conf status-left still contains 'GLITCH' name")
	}
}

func TestBuildTmuxConf_WindowPaneChords(t *testing.T) {
	dir := t.TempDir()
	_, err := bootstrap.WriteTmuxConf(dir, "/fake/orcai")
	if err != nil {
		t.Fatalf("WriteTmuxConf: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "tmux.conf"))
	if err != nil {
		t.Fatalf("read tmux.conf: %v", err)
	}
	conf := string(data)

	chords := []struct {
		key  string
		desc string
	}{
		{"glitch-chord c", "new window (c)"},
		{"glitch-chord [", "previous window ([)"},
		{"glitch-chord ]", "next window (])"},
		{"glitch-chord |", "split pane right (|)"},
		{"glitch-chord -", "split pane down (-)"},
		{"glitch-chord Left", "select pane left"},
		{"glitch-chord Right", "select pane right"},
		{"glitch-chord Up", "select pane up"},
		{"glitch-chord Down", "select pane down"},
		{"glitch-chord x", "kill pane (x)"},
		{"glitch-chord j", "session/window jump (j)"},
	}
	for _, c := range chords {
		if !strings.Contains(conf, c.key) {
			t.Errorf("tmux.conf missing chord binding for %s", c.desc)
		}
	}
}

func TestBuildTmuxConf_WindowStatusFormats(t *testing.T) {
	dir := t.TempDir()
	_, err := bootstrap.WriteTmuxConf(dir, "/fake/orcai")
	if err != nil {
		t.Fatalf("WriteTmuxConf: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "tmux.conf"))
	if err != nil {
		t.Fatalf("read tmux.conf: %v", err)
	}
	conf := string(data)

	// window-status-format must be blank (suppresses raw window list).
	if !strings.Contains(conf, `window-status-format ""`) {
		t.Error("window-status-format must be blank to suppress window list")
	}
	// window-status-current-format carries the centred hint bar (not blank).
	if strings.Contains(conf, `window-status-current-format ""`) {
		t.Error("window-status-current-format should not be blank — it carries the hint bar")
	}
}

func TestSessionExists_NoSuchSession(t *testing.T) {
	if !bootstrap.HasTmux() {
		t.Skip("tmux not in PATH")
	}
	got := bootstrap.SessionExists("orcai-test-nonexistent-xyz")
	if got {
		t.Error("SessionExists returned true for a session that should not exist")
	}
}
