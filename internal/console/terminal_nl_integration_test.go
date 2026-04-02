package console_test

// terminal_nl_integration_test.go — visual tmux tests for /terminal NL parsing.
//
// Each test sends a natural-language /terminal command to a live gl1tch session
// and verifies both the TUI confirmation message and the resulting tmux layout.
//
// Run: go test -v -run TestTmux_TerminalNL ./internal/console/

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// ── helpers shared with terminal_integration_test.go via same package ─────────
// setupTerminalSession, termCmd, captureGlitch, glitchPane, paneCount,
// paneWidths, paneHeights, waitForPaneCount, abs — all defined there.

// ── visual confirmation helper ────────────────────────────────────────────────

// assertGlitchSays waits up to maxWait for the gl1tch TUI pane to show text,
// failing the test with a screenshot if it never appears.
func assertGlitchSays(t *testing.T, session, text string, maxWait time.Duration) {
	t.Helper()
	ok := waitFor(maxWait, func() bool {
		return strings.Contains(captureGlitch(t, session), text)
	})
	if !ok {
		t.Errorf("gl1tch pane did not show %q within %s:\n%s",
			text, maxWait, captureGlitch(t, session))
	}
}

// ── NL: "open a terminal" ─────────────────────────────────────────────────────

// TestTmux_TerminalNL_OpenATerminal verifies that "/terminal open a terminal"
// creates one pane and the TUI confirms "opening terminal split."
func TestTmux_TerminalNL_OpenATerminal(t *testing.T) {
	session, cleanup := setupTerminalSession(t, "nl-open")
	defer cleanup()

	before := paneCount(t, session)
	termCmd(t, session, "open a terminal")

	if !waitForPaneCount(session, before+1, 5*time.Second) {
		t.Errorf("pane not created: got %d, want %d", paneCount(t, session), before+1)
	}
	assertGlitchSays(t, session, "opening terminal split.", 3*time.Second)
}

// TestTmux_TerminalNL_AShell verifies the shortest form: "/terminal a shell".
func TestTmux_TerminalNL_AShell(t *testing.T) {
	session, cleanup := setupTerminalSession(t, "nl-ashell")
	defer cleanup()

	before := paneCount(t, session)
	termCmd(t, session, "a shell")

	if !waitForPaneCount(session, before+1, 5*time.Second) {
		t.Errorf("pane not created")
	}
	assertGlitchSays(t, session, "opening terminal split.", 3*time.Second)
}

// ── NL: size ──────────────────────────────────────────────────────────────────

// TestTmux_TerminalNL_50PercentWidth verifies "/terminal open a shell 50% width"
// creates a pane with ~50% width and shows "opening 50% terminal split."
func TestTmux_TerminalNL_50PercentWidth(t *testing.T) {
	session, cleanup := setupTerminalSession(t, "nl-50pct")
	defer cleanup()

	termCmd(t, session, "open a shell 50% width")

	if !waitForPaneCount(session, 2, 5*time.Second) {
		t.Fatalf("pane not created")
	}

	// Visual confirmation in TUI.
	assertGlitchSays(t, session, "opening 50% terminal split.", 3*time.Second)

	// Geometry: both panes should be ~110 cols wide (220 / 2), tolerance ±5.
	ws := paneWidths(t, session)
	if len(ws) < 2 {
		t.Fatalf("expected 2 pane widths, got %d", len(ws))
	}
	if abs(ws[0]-ws[1]) > 5 {
		t.Errorf("pane widths not equal for 50%% split: %v", ws)
	}
}

// TestTmux_TerminalNL_HalfWidth verifies "/terminal half terminal" creates a 50% split.
func TestTmux_TerminalNL_HalfWidth(t *testing.T) {
	session, cleanup := setupTerminalSession(t, "nl-half")
	defer cleanup()

	termCmd(t, session, "half terminal")

	if !waitForPaneCount(session, 2, 5*time.Second) {
		t.Fatalf("pane not created")
	}
	assertGlitchSays(t, session, "opening 50% terminal split.", 3*time.Second)
}

// ── NL: direction ─────────────────────────────────────────────────────────────

// TestTmux_TerminalNL_BottomTerminal verifies "/terminal open a terminal at the bottom"
// creates a vertical (bottom) split.
func TestTmux_TerminalNL_BottomTerminal(t *testing.T) {
	session, cleanup := setupTerminalSession(t, "nl-bottom")
	defer cleanup()

	hsBefore := paneHeights(t, session)
	termCmd(t, session, "open a terminal at the bottom")

	if !waitForPaneCount(session, 2, 5*time.Second) {
		t.Fatalf("pane not created")
	}

	assertGlitchSays(t, session, "opening bottom terminal split.", 3*time.Second)

	// Geometry: for a vertical split the new pane should be shorter than the original.
	hs := paneHeights(t, session)
	if len(hs) < 2 {
		t.Fatalf("expected 2 height entries, got %d", len(hs))
	}
	if hs[0] >= hsBefore[0] {
		t.Errorf("gl1tch pane height (%d) should be less than original (%d) after a bottom split", hs[0], hsBefore[0])
	}
}

// TestTmux_TerminalNL_LeftTerminal verifies "/terminal left terminal" creates
// a left-side horizontal split.
func TestTmux_TerminalNL_LeftTerminal(t *testing.T) {
	session, cleanup := setupTerminalSession(t, "nl-left")
	defer cleanup()

	termCmd(t, session, "left terminal")

	if !waitForPaneCount(session, 2, 5*time.Second) {
		t.Fatalf("pane not created")
	}
	assertGlitchSays(t, session, "opening left terminal split.", 3*time.Second)
}

// ── NL: count ─────────────────────────────────────────────────────────────────

// TestTmux_TerminalNL_ThreeTerminals verifies "/terminal 3 terminals" opens
// exactly 3 terminal panes and the TUI confirms "opening 3 terminals."
func TestTmux_TerminalNL_ThreeTerminals(t *testing.T) {
	session, cleanup := setupTerminalSession(t, "nl-3")
	defer cleanup()

	before := paneCount(t, session)
	termCmd(t, session, "3 terminals")

	if !waitForPaneCount(session, before+3, 8*time.Second) {
		t.Errorf("expected %d panes, got %d", before+3, paneCount(t, session))
	}
	assertGlitchSays(t, session, "opening 3 terminals.", 3*time.Second)
}

// ── NL: multi-pane with CWD ───────────────────────────────────────────────────

// TestTmux_TerminalNL_ThreeShellsWithCWD verifies the primary use-case:
// "/terminal open 3 shells and set cwd to DIR1 DIR2 DIR3"
// — creates 3 panes, each in the specified directory, and shows the path list.
func TestTmux_TerminalNL_ThreeShellsWithCWD(t *testing.T) {
	// Use real temp directories so the cwd actually exists.
	d1 := t.TempDir()
	d2 := t.TempDir()
	d3 := t.TempDir()

	session, cleanup := setupTerminalSession(t, "nl-cwd3")
	defer cleanup()

	before := paneCount(t, session)
	input := fmt.Sprintf("open 3 shells and set cwd to %s %s %s", d1, d2, d3)
	termCmd(t, session, input)

	if !waitForPaneCount(session, before+3, 8*time.Second) {
		t.Errorf("expected %d panes, got %d", before+3, paneCount(t, session))
	}

	// TUI must list all three directories.
	assertGlitchSays(t, session, "opening 3 terminals:", 3*time.Second)

	// Each temp dir path should appear in the confirmation.
	for _, d := range []string{d1, d2, d3} {
		assertGlitchSays(t, session, d, 2*time.Second)
	}
}

// TestTmux_TerminalNL_SingleWithCWD verifies "/terminal terminal in DIR"
// opens one pane and confirms "opening terminal in DIR."
func TestTmux_TerminalNL_SingleWithCWD(t *testing.T) {
	dir := t.TempDir()

	session, cleanup := setupTerminalSession(t, "nl-cwd1")
	defer cleanup()

	before := paneCount(t, session)
	termCmd(t, session, "terminal in "+dir)

	if !waitForPaneCount(session, before+1, 5*time.Second) {
		t.Fatalf("pane not created")
	}
	assertGlitchSays(t, session, "opening terminal in "+dir+".", 3*time.Second)
}

// ── NL: raw command not intercepted ──────────────────────────────────────────

// TestTmux_TerminalNL_RawCommandPassthrough verifies that "/terminal bash" still
// works as a raw command (not NL-parsed into a generic open) and the TUI shows
// "opening terminal: bash".
func TestTmux_TerminalNL_RawCommandPassthrough(t *testing.T) {
	session, cleanup := setupTerminalSession(t, "nl-raw")
	defer cleanup()

	before := paneCount(t, session)
	termCmd(t, session, "bash")

	if !waitForPaneCount(session, before+1, 5*time.Second) {
		t.Fatalf("pane not created")
	}
	assertGlitchSays(t, session, "opening terminal: bash", 3*time.Second)
}

// TestTmux_TerminalNL_GlitchPaneUnaffected verifies that after all NL-spawned
// panes the gl1tch TUI (pane 0) is still alive and responsive.
func TestTmux_TerminalNL_GlitchPaneUnaffected(t *testing.T) {
	if !tmuxAvailable() {
		t.Skip("tmux not in PATH")
	}

	binPath := buildGlitchBinary(t)
	cfgDir := t.TempDir()
	os.WriteFile(filepath.Join(cfgDir, ".glitch_intro_seen"), []byte(""), 0o600) //nolint:errcheck

	session := fmt.Sprintf("glitch-nl-alive-%d", os.Getpid())
	cleanup := newTmuxSession(t, session, binPath, []string{
		"GLITCH_CONFIG_DIR=" + cfgDir,
		"TERM=xterm-256color",
	})
	defer cleanup()
	if !waitFor(5*time.Second, func() bool {
		return strings.Contains(captureGlitch(t, session), "ready")
	}) {
		t.Fatalf("TUI never started")
	}
	time.Sleep(500 * time.Millisecond)

	// Open several terminals via NL.
	termCmd(t, session, "open a terminal")
	waitForPaneCount(session, 2, 3*time.Second)
	termCmd(t, session, "open a shell 50% width")
	waitForPaneCount(session, 3, 3*time.Second)
	time.Sleep(200 * time.Millisecond)

	// Pane 0 must still respond to commands.
	exec.Command("tmux", "send-keys", "-t", glitchPane(session), "/help", "Escape", "Enter").Run() //nolint:errcheck
	ok := waitFor(4*time.Second, func() bool {
		return strings.Contains(captureGlitch(t, session), "/models")
	})
	if !ok {
		t.Errorf("gl1tch pane unresponsive after NL terminal opens:\n%s", captureGlitch(t, session))
	}
}
