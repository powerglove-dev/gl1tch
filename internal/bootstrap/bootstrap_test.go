package bootstrap_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/adam-stokes/orcai/internal/bootstrap"
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
	// Status bar must contain chord hints.
	if !strings.Contains(conf, "^spc n new") {
		t.Error("tmux.conf status-right missing '^spc n new' hint")
	}
	if !strings.Contains(conf, "^spc t panel") {
		t.Error("tmux.conf status-right missing '^spc t panel' hint")
	}
	// Sysop toggle chord must be present.
	if !strings.Contains(conf, "orcai-sysop") {
		t.Error("tmux.conf missing sysop toggle chord binding")
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
