package console_test

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

// tmuxAvailable returns true if tmux is in PATH.
func tmuxAvailable() bool {
	_, err := exec.LookPath("tmux")
	return err == nil
}

// buildGlitchBinary compiles the glitch binary into a temp dir and returns
// the path. The test is skipped if go build fails.
func buildGlitchBinary(t *testing.T) string {
	t.Helper()
	binDir := t.TempDir()
	binPath := filepath.Join(binDir, "glitch")
	// Resolve the module root: go up from internal/console to the project root.
	moduleRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("resolve module root: %v", err)
	}
	cmd := exec.Command("go", "build", "-o", binPath, ".")
	cmd.Dir = moduleRoot
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Skipf("go build failed (skip integration test): %v\n%s", err, out)
	}
	return binPath
}

// tmuxCapture returns the visible text content of the given tmux pane.
func tmuxCapture(t *testing.T, session string) string {
	t.Helper()
	out, err := exec.Command("tmux", "capture-pane", "-p", "-t", session).Output()
	if err != nil {
		t.Logf("tmux capture-pane error: %v", err)
		return ""
	}
	return string(out)
}

// tmuxSend sends keys to the given tmux session.
func tmuxSend(session string, keys ...string) error {
	for _, k := range keys {
		if err := exec.Command("tmux", "send-keys", "-t", session, k, "").Run(); err != nil {
			return err
		}
	}
	return nil
}

// waitFor polls fn every 200ms for up to maxWait, returning true when fn returns true.
func waitFor(maxWait time.Duration, fn func() bool) bool {
	deadline := time.Now().Add(maxWait)
	for time.Now().Before(deadline) {
		if fn() {
			return true
		}
		time.Sleep(200 * time.Millisecond)
	}
	return false
}

// newTmuxSession creates a detached tmux session and returns a cleanup func.
// The session is sized 220x50 to give the TUI enough room to render.
func newTmuxSession(t *testing.T, name, command string, env []string) func() {
	t.Helper()
	args := []string{"new-session", "-d", "-s", name, "-x", "220", "-y", "50"}
	// Use -e to pass each env var explicitly into the session, overriding any
	// stale values the tmux server may have inherited from previous test runs.
	for _, kv := range env {
		args = append(args, "-e", kv)
	}
	if command != "" {
		args = append(args, command)
	}
	cmd := exec.Command("tmux", args...)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("tmux new-session: %v\n%s", err, out)
	}
	return func() {
		exec.Command("tmux", "kill-session", "-t", name).Run() //nolint:errcheck
	}
}

// testCfgDir creates a temp config dir with the intro sentinel pre-written
// so tests skip the first-run Ollama path.
func testCfgDir(t *testing.T) string {
	t.Helper()
	cfgDir := t.TempDir()
	os.WriteFile(filepath.Join(cfgDir, ".glitch_intro_seen"), []byte(""), 0o600) //nolint:errcheck
	return cfgDir
}

// testSession returns a unique session name based on a suffix and PID.
func testSession(suffix string) string {
	return fmt.Sprintf("glitch-%s-%d", suffix, os.Getpid())
}
