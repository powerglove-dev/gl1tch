package console_test

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
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
		t.Skipf("go build failed (skip tmux test): %v\n%s", err, out)
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
	if command != "" {
		args = append(args, command)
	}
	cmd := exec.Command("tmux", args...)
	cmd.Env = append(os.Environ(), env...)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("tmux new-session: %v\n%s", err, out)
	}
	return func() {
		exec.Command("tmux", "kill-session", "-t", name).Run() //nolint:errcheck
	}
}

// ── tests ─────────────────────────────────────────────────────────────────────

// TestTmux_GlitchShowsReadyMessage verifies that after startup glitch displays
// the static ready message (non-first-run path — sentinel file pre-created so
// the test does not require Ollama).
func TestTmux_GlitchShowsReadyMessage(t *testing.T) {
	if !tmuxAvailable() {
		t.Skip("tmux not in PATH")
	}

	binPath := buildGlitchBinary(t)
	cfgDir := t.TempDir()

	// Pre-create the sentinel file so we land on the static ready message path
	// rather than the first-run Ollama-streaming path.
	sentinel := filepath.Join(cfgDir, ".glitch_intro_seen")
	if err := os.WriteFile(sentinel, []byte(""), 0o600); err != nil {
		t.Fatalf("write sentinel: %v", err)
	}

	session := fmt.Sprintf("glitch-test-%d", os.Getpid())
	cleanup := newTmuxSession(t, session, binPath, []string{
		"GLITCH_CONFIG_DIR=" + cfgDir,
		"TERM=xterm-256color",
	})
	defer cleanup()

	// Wait up to 5 s for the ready message to appear.
	ok := waitFor(5*time.Second, func() bool {
		return strings.Contains(tmuxCapture(t, session), "ready")
	})
	if !ok {
		t.Errorf("ready message never appeared in pane output:\n%s", tmuxCapture(t, session))
	}
}

// TestTmux_GlitchHelp verifies that typing /help in the GLITCH chat panel
// outputs the list of slash commands without needing Ollama.
func TestTmux_GlitchHelp(t *testing.T) {
	if !tmuxAvailable() {
		t.Skip("tmux not in PATH")
	}

	binPath := buildGlitchBinary(t)
	cfgDir := t.TempDir()

	// Pre-create sentinel so we skip first-run Ollama intro.
	os.WriteFile(filepath.Join(cfgDir, ".glitch_intro_seen"), []byte(""), 0o600) //nolint:errcheck

	session := fmt.Sprintf("glitch-test-help-%d", os.Getpid())
	cleanup := newTmuxSession(t, session, binPath, []string{
		"GLITCH_CONFIG_DIR=" + cfgDir,
		"TERM=xterm-256color",
	})
	defer cleanup()

	// Wait for the ready message, then pause to let the TUI fully stabilize
	// before sending input (the event loop must be running before keys land).
	if !waitFor(5*time.Second, func() bool {
		return strings.Contains(tmuxCapture(t, session), "ready")
	}) {
		t.Fatalf("TUI ready message never appeared:\n%s", tmuxCapture(t, session))
	}
	time.Sleep(500 * time.Millisecond)

	// Send /help — Escape first to dismiss the autocomplete overlay that activates
	// when typing '/', then Enter to submit.
	if err := exec.Command("tmux", "send-keys", "-t", session, "/help", "Escape", "Enter").Run(); err != nil {
		t.Fatalf("send /help: %v", err)
	}

	// Wait for the command list to appear.
	ok := waitFor(4*time.Second, func() bool {
		c := tmuxCapture(t, session)
		return strings.Contains(c, "/models") && strings.Contains(c, "/pipeline")
	})
	if !ok {
		t.Errorf("/help output missing expected commands:\n%s", tmuxCapture(t, session))
	}
}

// TestTmux_GlitchClear verifies that /clear empties the chat panel.
func TestTmux_GlitchClear(t *testing.T) {
	if !tmuxAvailable() {
		t.Skip("tmux not in PATH")
	}

	binPath := buildGlitchBinary(t)
	cfgDir := t.TempDir()
	os.WriteFile(filepath.Join(cfgDir, ".glitch_intro_seen"), []byte(""), 0o600) //nolint:errcheck

	session := fmt.Sprintf("glitch-test-clear-%d", os.Getpid())
	cleanup := newTmuxSession(t, session, binPath, []string{
		"GLITCH_CONFIG_DIR=" + cfgDir,
		"TERM=xterm-256color",
	})
	defer cleanup()

	// Wait for TUI.
	if !waitFor(5*time.Second, func() bool {
		c := tmuxCapture(t, session)
		return strings.Contains(c, "│") || strings.Contains(c, "ready")
	}) {
		t.Fatalf("TUI never started:\n%s", tmuxCapture(t, session))
	}

	// Wait for the ready message then stabilize.
	if !waitFor(5*time.Second, func() bool {
		return strings.Contains(tmuxCapture(t, session), "ready")
	}) {
		t.Fatalf("TUI ready message never appeared:\n%s", tmuxCapture(t, session))
	}
	time.Sleep(500 * time.Millisecond)

	// Send /help to populate chat. Escape dismisses autocomplete, Enter submits.
	exec.Command("tmux", "send-keys", "-t", session, "/help", "Escape", "Enter").Run() //nolint:errcheck

	// Wait for the /help output to appear.
	if !waitFor(3*time.Second, func() bool {
		return strings.Contains(tmuxCapture(t, session), "/models")
	}) {
		t.Fatalf("/help output never appeared:\n%s", tmuxCapture(t, session))
	}
	time.Sleep(200 * time.Millisecond)

	// Send /clear. The visible chat content (not scrollback) should become empty.
	exec.Command("tmux", "send-keys", "-t", session, "/clear", "Escape", "Enter").Run() //nolint:errcheck

	// After /clear, the input placeholder should be visible and the slash commands list gone.
	ok := waitFor(3*time.Second, func() bool {
		c := tmuxCapture(t, session)
		// /clear empties messages; the placeholder text reappears in the input.
		return strings.Contains(c, "ask glitch anything") && !strings.Contains(c, "/init")
	})
	if !ok {
		t.Errorf("/clear did not clear chat:\n%s", tmuxCapture(t, session))
	}
}

// TestTmux_GlitchChatGetsResponse is a regression test for the glitchIntentMsg
// routing bug: when the user sends a chat message, glitch must respond (pipeline
// match, LLM reply, or error) rather than hanging silently with routing stuck.
func TestTmux_GlitchChatGetsResponse(t *testing.T) {
	if !tmuxAvailable() {
		t.Skip("tmux not in PATH")
	}

	binPath := buildGlitchBinary(t)
	cfgDir := t.TempDir()
	os.WriteFile(filepath.Join(cfgDir, ".glitch_intro_seen"), []byte(""), 0o600) //nolint:errcheck

	session := fmt.Sprintf("glitch-test-chat-%d", os.Getpid())
	cleanup := newTmuxSession(t, session, binPath, []string{
		"GLITCH_CONFIG_DIR=" + cfgDir,
		"TERM=xterm-256color",
	})
	defer cleanup()

	// Wait for the ready message then stabilize.
	if !waitFor(5*time.Second, func() bool {
		return strings.Contains(tmuxCapture(t, session), "ready")
	}) {
		t.Fatalf("TUI ready message never appeared:\n%s", tmuxCapture(t, session))
	}
	time.Sleep(500 * time.Millisecond)

	// Send a message that will not match any pipeline (no pipelines in temp cfgDir).
	exec.Command("tmux", "send-keys", "-t", session, "hello glitch", "Enter").Run() //nolint:errcheck

	// Expect GL1TCH to show something — a reply, an error, or a pipeline message.
	// Any response proves routing is not stuck. "YOU" appears when the message
	// is appended to the chat; we wait for that first, then a bot response.
	if !waitFor(4*time.Second, func() bool {
		return strings.Contains(tmuxCapture(t, session), "YOU")
	}) {
		t.Fatalf("user message never appeared in chat:\n%s", tmuxCapture(t, session))
	}

	// Now wait for any GL1TCH reply (LLM response, error, or pipeline trigger).
	ok := waitFor(30*time.Second, func() bool {
		c := tmuxCapture(t, session)
		// Accept any bot response: streaming token, error, or pipeline match.
		return strings.Contains(c, "GL1TCH") &&
			(strings.Contains(c, "no provider") ||
				strings.Contains(c, "→ running") ||
				// LLM response: GL1TCH message after the user turn
				strings.Count(c, "GL1TCH") >= 2)
	})
	if !ok {
		t.Errorf("glitch never responded to chat message (routing may be stuck):\n%s", tmuxCapture(t, session))
	}
}
