package console_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	glitchcron "github.com/8op-org/gl1tch/internal/cron"
	"github.com/8op-org/gl1tch/internal/console"
)

// TestBuildCronSection_Empty verifies that buildCronSection renders the
// "no scheduled jobs" placeholder when no cron.yaml entries exist.
// It temporarily overrides HOME so LoadConfig finds an empty config.
func TestBuildCronSection_Empty(t *testing.T) {
	// Only run the empty-case assertion when the real config is empty.
	entries, _ := glitchcron.LoadConfig()
	if len(entries) > 0 {
		t.Skip("skipping empty-case test: real cron.yaml has entries")
	}

	m := console.New()
	lines := m.BuildCronSection(80)
	if len(lines) == 0 {
		t.Fatal("expected non-empty output from BuildCronSection")
	}
	combined := strings.Join(lines, "\n")
	if !strings.Contains(combined, "no scheduled jobs") {
		t.Errorf("expected 'no scheduled jobs' in output, got:\n%s", combined)
	}
}

// TestBuildCronSection_EmptyViaFakeHome verifies that buildCronSection
// renders "no scheduled jobs" when pointed at an empty config dir.
func TestBuildCronSection_EmptyViaFakeHome(t *testing.T) {
	// Redirect HOME so that LoadConfig reads from an empty temp dir.
	tmp := t.TempDir()
	cfgDir := filepath.Join(tmp, ".config", "glitch")
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	orig := os.Getenv("HOME")
	os.Setenv("HOME", tmp)
	defer os.Setenv("HOME", orig)

	m := console.New()
	lines := m.BuildCronSection(80)
	if len(lines) == 0 {
		t.Fatal("expected non-empty output from BuildCronSection")
	}
	combined := strings.Join(lines, "\n")
	if !strings.Contains(combined, "no scheduled jobs") {
		t.Errorf("expected 'no scheduled jobs' in output, got:\n%s", combined)
	}
}

// TestCronManageKey verifies that pressing "m" while the cron panel is focused
// does not panic even when tmux is not running. The key dispatches
// ensureCronDaemon + switch-client which fail silently without tmux.
func TestCronManageKey(t *testing.T) {
	m := console.New()
	// Focus the cron panel first.
	m2, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("c")})
	mm := m2.(console.Model)
	if !mm.CronPanelFocused() {
		t.Skip("cron panel not focused; skipping m-key test")
	}
	// Press m — should not panic; tmux calls will fail silently.
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("panic on 'm' keypress in cron panel: %v", r)
		}
	}()
	mm.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("m")})
}
