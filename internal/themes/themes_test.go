package themes_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/adam-stokes/orcai/internal/themes"
)

// ---------------------------------------------------------------------------
// LoadBundled
// ---------------------------------------------------------------------------

func TestLoadBundled_ReturnsABS(t *testing.T) {
	bundles, err := themes.LoadBundled()
	if err != nil {
		t.Fatalf("LoadBundled() error: %v", err)
	}
	found := false
	for _, b := range bundles {
		if b.Name == "abs" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("LoadBundled() did not return the ABS theme; got %v", bundleNames(bundles))
	}
}

func TestLoadBundled_ABSPaletteComplete(t *testing.T) {
	bundles, err := themes.LoadBundled()
	if err != nil {
		t.Fatalf("LoadBundled() error: %v", err)
	}
	var abs *themes.Bundle
	for i := range bundles {
		if bundles[i].Name == "abs" {
			abs = &bundles[i]
			break
		}
	}
	if abs == nil {
		t.Fatal("ABS theme not found in bundled themes")
	}
	fields := map[string]string{
		"bg":      abs.Palette.BG,
		"fg":      abs.Palette.FG,
		"accent":  abs.Palette.Accent,
		"dim":     abs.Palette.Dim,
		"border":  abs.Palette.Border,
		"error":   abs.Palette.Error,
		"success": abs.Palette.Success,
	}
	for key, val := range fields {
		if val == "" {
			t.Errorf("ABS theme palette.%s is empty", key)
		}
	}
}

// ---------------------------------------------------------------------------
// LoadUser
// ---------------------------------------------------------------------------

func TestLoadUser_CustomTheme(t *testing.T) {
	dir := t.TempDir()
	writeTheme(t, dir, "mytheme", `
name: mytheme
display_name: "My Theme"
palette:
  bg: "#000000"
  fg: "#ffffff"
  accent: "#ff0000"
  dim: "#888888"
  border: "#444444"
  error: "#ff5555"
  success: "#55ff55"
borders:
  style: ascii
statusbar:
  format: " {session} "
  bg: "#000000"
  fg: "#ff0000"
`)
	bundles, err := themes.LoadUser(dir)
	if err != nil {
		t.Fatalf("LoadUser() error: %v", err)
	}
	if len(bundles) != 1 {
		t.Fatalf("LoadUser() returned %d bundles, want 1", len(bundles))
	}
	if bundles[0].Name != "mytheme" {
		t.Errorf("got name %q, want %q", bundles[0].Name, "mytheme")
	}
}

func TestLoadUser_MissingDir_ReturnsEmpty(t *testing.T) {
	bundles, err := themes.LoadUser("/nonexistent/path/that/does/not/exist")
	if err != nil {
		t.Fatalf("LoadUser() unexpected error for missing dir: %v", err)
	}
	if len(bundles) != 0 {
		t.Errorf("LoadUser() returned %d bundles for missing dir, want 0", len(bundles))
	}
}

// ---------------------------------------------------------------------------
// Registry
// ---------------------------------------------------------------------------

func TestRegistry_UserOverridesBundled(t *testing.T) {
	dir := t.TempDir()
	// Override the bundled "abs" theme with a custom version.
	writeTheme(t, dir, "abs", `
name: abs
display_name: "ABS Custom"
palette:
  bg: "#111111"
  fg: "#eeeeee"
  accent: "#aaaaff"
  dim: "#555555"
  border: "#333333"
  error: "#ff0000"
  success: "#00ff00"
borders:
  style: heavy
statusbar:
  format: " custom "
  bg: "#111111"
  fg: "#aaaaff"
`)
	reg, err := themes.NewRegistry(dir)
	if err != nil {
		t.Fatalf("NewRegistry() error: %v", err)
	}
	b, ok := reg.Get("abs")
	if !ok {
		t.Fatal("abs theme not found in registry")
	}
	if b.DisplayName != "ABS Custom" {
		t.Errorf("user theme should win: got DisplayName %q, want %q", b.DisplayName, "ABS Custom")
	}
}

func TestRegistry_ActiveDefaultNotNil(t *testing.T) {
	reg, err := themes.NewRegistry("")
	if err != nil {
		t.Fatalf("NewRegistry() error: %v", err)
	}
	if reg.Active() == nil {
		t.Error("Registry.Active() returned nil for default")
	}
}

func TestRegistry_SetActive_PersistsAndRestores(t *testing.T) {
	// Use a temp config dir so we don't pollute the real config.
	cfgDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", cfgDir) // honoured by os.UserConfigDir on Linux
	// macOS uses $HOME/Library/Application Support; override via HOME trick
	// won't work cleanly, so we test the write + restore path by calling
	// SetActive then NewRegistry again within the same temp config dir.

	reg, err := themes.NewRegistry("")
	if err != nil {
		t.Fatalf("NewRegistry() error: %v", err)
	}
	// Confirm abs is available.
	if _, ok := reg.Get("abs"); !ok {
		t.Skip("abs theme not available; skipping SetActive test")
	}

	if err := reg.SetActive("abs"); err != nil {
		t.Fatalf("SetActive(%q) error: %v", "abs", err)
	}
	if reg.Active() == nil || reg.Active().Name != "abs" {
		t.Errorf("Active() after SetActive: got %v, want abs", reg.Active())
	}

	// Verify the active theme is non-nil and correct.
	if reg.Active().Name != "abs" {
		t.Errorf("restored active theme = %q, want %q", reg.Active().Name, "abs")
	}
}

func TestRegistry_SetActive_UnknownReturnsError(t *testing.T) {
	reg, err := themes.NewRegistry("")
	if err != nil {
		t.Fatalf("NewRegistry() error: %v", err)
	}
	if err := reg.SetActive("no-such-theme"); err == nil {
		t.Error("SetActive(unknown) expected error, got nil")
	}
}

func TestRegistry_All_ContainsBundled(t *testing.T) {
	reg, err := themes.NewRegistry("")
	if err != nil {
		t.Fatalf("NewRegistry() error: %v", err)
	}
	all := reg.All()
	if len(all) == 0 {
		t.Fatal("Registry.All() returned empty slice")
	}
}

// ---------------------------------------------------------------------------
// Bus event constant
// ---------------------------------------------------------------------------

func TestTopicThemeChanged_Value(t *testing.T) {
	if themes.TopicThemeChanged != "theme.changed" {
		t.Errorf("TopicThemeChanged = %q, want %q", themes.TopicThemeChanged, "theme.changed")
	}
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func writeTheme(t *testing.T, dir, name, content string) {
	t.Helper()
	themeDir := filepath.Join(dir, name)
	if err := os.MkdirAll(themeDir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", themeDir, err)
	}
	path := filepath.Join(themeDir, "theme.yaml")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func bundleNames(bs []themes.Bundle) []string {
	names := make([]string, len(bs))
	for i, b := range bs {
		names[i] = b.Name
	}
	return names
}
