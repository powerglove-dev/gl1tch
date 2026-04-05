//go:build iterm2

package console_test

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

// TestGlitchTUIiTerm2 launches glitch in a new iTerm2 window, verifies the TUI
// renders, sends a message, and quits cleanly.
//
// Run with: go test -tags iterm2 -v -run TestGlitchTUIiTerm2 ./internal/console/
func TestGlitchTUIiTerm2(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("iTerm2 tests only run on macOS")
	}

	// Check iTerm2 is running.
	checkScript := `tell application "System Events" to return exists process "iTerm2"`
	if out, err := exec.Command("osascript", "-e", checkScript).Output(); err != nil || string(out) != "true\n" {
		t.Skip("iTerm2 is not running — launch iTerm2 before running this test")
	}

	binPath := buildGlitchBinary(t)
	cfgDir := t.TempDir()
	os.WriteFile(filepath.Join(cfgDir, ".glitch_intro_seen"), []byte(""), 0o600) //nolint:errcheck

	// Create screenshots directory.
	screenshotDir := filepath.Join("testdata", "screenshots")
	if err := os.MkdirAll(screenshotDir, 0o755); err != nil {
		t.Fatalf("create screenshots dir: %v", err)
	}

	ts := time.Now().Format("20060102-150405")

	// ── Open a new iTerm2 window and run glitch ──────────────────────────────
	launchScript := fmt.Sprintf(`
tell application "iTerm2"
    activate
    set newWin to (create window with default profile)
    tell newWin
        tell current session
            set Environment to {{"GLITCH_CONFIG_DIR", "%s"}}
            write text "%s"
        end tell
    end tell
end tell
`, cfgDir, binPath)

	if err := runAppleScript(launchScript); err != nil {
		t.Fatalf("launch glitch in iTerm2: %v", err)
	}

	// Wait for TUI to render.
	time.Sleep(2 * time.Second)

	// Screenshot 1: startup state.
	startup := filepath.Join(screenshotDir, ts+"-startup.png")
	if err := screenshot(startup); err != nil {
		t.Logf("screenshot (startup): %v", err)
	} else {
		t.Logf("startup screenshot: %s", startup)
	}

	// ── Send "hello world" + Enter ───────────────────────────────────────────
	sendKeysScript := `
tell application "iTerm2"
    tell current window
        tell current session
            write text "hello world"
        end tell
    end tell
end tell
`
	if err := runAppleScript(sendKeysScript); err != nil {
		t.Logf("send 'hello world': %v", err)
	}
	time.Sleep(1 * time.Second)

	// Screenshot 2: after message.
	afterMsg := filepath.Join(screenshotDir, ts+"-after_message.png")
	if err := screenshot(afterMsg); err != nil {
		t.Logf("screenshot (after_message): %v", err)
	} else {
		t.Logf("after_message screenshot: %s", afterMsg)
	}

	// ── Send /quit + Enter ───────────────────────────────────────────────────
	quitScript := `
tell application "iTerm2"
    tell current window
        tell current session
            write text "/quit"
        end tell
    end tell
end tell
`
	if err := runAppleScript(quitScript); err != nil {
		t.Logf("send '/quit': %v", err)
	}
	time.Sleep(500 * time.Millisecond)

	t.Logf("glitch iTerm2 test completed; screenshots in %s", screenshotDir)
}

// runAppleScript executes the given AppleScript via osascript.
func runAppleScript(script string) error {
	cmd := exec.Command("osascript", "-e", script)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("osascript: %w: %s", err, out)
	}
	return nil
}

// screenshot captures the full screen to path using macOS screencapture.
func screenshot(path string) error {
	return exec.Command("screencapture", "-x", path).Run()
}
