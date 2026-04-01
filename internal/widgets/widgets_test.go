package widgets_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/8op-org/gl1tch/internal/widgets"
)

// writeManifest writes a widget.yaml file into dir/subdir/widget.yaml.
func writeManifest(t *testing.T, dir, subdir, content string) {
	t.Helper()
	sub := filepath.Join(dir, subdir)
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", sub, err)
	}
	path := filepath.Join(sub, widgets.ManifestFile)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

// TestDiscover_ReturnsManifests verifies that two valid widget.yaml files in
// separate subdirectories are both discovered and parsed correctly.
func TestDiscover_ReturnsManifests(t *testing.T) {
	dir := t.TempDir()

	writeManifest(t, dir, "welcome", `
name: welcome
binary: /usr/local/bin/orcai-welcome
description: "ABS welcome dashboard"
subscribe:
  - theme.changed
  - session.started
`)
	writeManifest(t, dir, "weather", `
name: weather
binary: /usr/local/bin/orcai-weather
description: "Weather widget"
subscribe:
  - glitch.telemetry
`)

	got, err := widgets.Discover(dir)
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("Discover returned %d manifests; want 2", len(got))
	}

	// Build a name→manifest map for stable assertions regardless of order.
	byName := make(map[string]widgets.Manifest, len(got))
	for _, m := range got {
		byName[m.Name] = m
	}

	welcome, ok := byName["welcome"]
	if !ok {
		t.Fatal("manifest 'welcome' not found in results")
	}
	if welcome.Binary != "/usr/local/bin/orcai-welcome" {
		t.Errorf("welcome.Binary = %q; want /usr/local/bin/orcai-welcome", welcome.Binary)
	}
	if len(welcome.Subscribe) != 2 {
		t.Errorf("welcome.Subscribe len = %d; want 2", len(welcome.Subscribe))
	}

	weather, ok := byName["weather"]
	if !ok {
		t.Fatal("manifest 'weather' not found in results")
	}
	if weather.Description != "Weather widget" {
		t.Errorf("weather.Description = %q; want \"Weather widget\"", weather.Description)
	}
}

// TestDiscover_MalformedYAML verifies that a malformed widget.yaml returns an error.
func TestDiscover_MalformedYAML(t *testing.T) {
	dir := t.TempDir()

	writeManifest(t, dir, "broken", "name: [this is: not: valid yaml\n\tbad indent")

	_, err := widgets.Discover(dir)
	if err == nil {
		t.Fatal("Discover should return an error for malformed YAML, got nil")
	}
}

// TestDiscover_MissingDir verifies that a non-existent directory returns an
// empty slice and nil error.
func TestDiscover_MissingDir(t *testing.T) {
	got, err := widgets.Discover("/tmp/glitch-widgets-does-not-exist-xyzzy")
	if err != nil {
		t.Fatalf("Discover on missing dir: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("Discover on missing dir returned %d manifests; want 0", len(got))
	}
}

// TestDiscover_SkipsFiles verifies that files placed directly in dir (not in a
// subdirectory) are ignored, even if named widget.yaml.
func TestDiscover_SkipsFiles(t *testing.T) {
	dir := t.TempDir()

	// Write a widget.yaml directly in dir (not in a subdir) — should be ignored.
	directFile := filepath.Join(dir, widgets.ManifestFile)
	if err := os.WriteFile(directFile, []byte("name: toplevel\nbinary: /bin/false\n"), 0o644); err != nil {
		t.Fatalf("write top-level file: %v", err)
	}

	// Also write a non-yaml file in a subdir to ensure it's skipped.
	sub := filepath.Join(dir, "mywidget")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(sub, "README.md"), []byte("hello"), 0o644); err != nil {
		t.Fatalf("write README: %v", err)
	}

	got, err := widgets.Discover(dir)
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	// The subdirectory has no widget.yaml, so nothing should be discovered.
	if len(got) != 0 {
		t.Errorf("Discover returned %d manifests; want 0", len(got))
	}
}

// TestResolveOverride_UsesOverrideBinary verifies that when an glitch-<name>
// binary is present on PATH, ResolveOverride returns it instead of the
// manifest binary.
func TestResolveOverride_UsesOverrideBinary(t *testing.T) {
	// Create a temp dir with a fake orcai-weather executable.
	tmpDir := t.TempDir()
	overridePath := filepath.Join(tmpDir, "glitch-weather")
	if err := os.WriteFile(overridePath, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatalf("write fake override: %v", err)
	}

	origPath := os.Getenv("PATH")
	os.Setenv("PATH", tmpDir+string(os.PathListSeparator)+origPath)
	defer os.Setenv("PATH", origPath)

	m := widgets.Manifest{
		Name:   "weather",
		Binary: "/usr/local/bin/orcai-weather-manifest",
	}

	got := widgets.ResolveOverride(m)
	if got != overridePath {
		t.Errorf("ResolveOverride = %q; want %q", got, overridePath)
	}
}

// TestResolveOverride_FallsBackToManifest verifies that when no glitch-<name>
// override exists in PATH, ResolveOverride returns the manifest's Binary.
func TestResolveOverride_FallsBackToManifest(t *testing.T) {
	origPath := os.Getenv("PATH")
	os.Setenv("PATH", "")
	defer os.Setenv("PATH", origPath)

	m := widgets.Manifest{
		Name:   "nonexistent-widget-xyz",
		Binary: "/some/manifest/binary",
	}

	got := widgets.ResolveOverride(m)
	if got != m.Binary {
		t.Errorf("ResolveOverride = %q; want %q", got, m.Binary)
	}
}

// TestLaunch_BadBinary verifies that Launch handles missing binaries gracefully.
// Since tmux new-window itself succeeds even with a nonexistent binary (the
// window opens and the shell reports the error), we focus on two things:
//  1. If tmux is not in PATH, the test is skipped.
//  2. Launch does not panic when called with a nonexistent binary.
func TestLaunch_BadBinary(t *testing.T) {
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux not in PATH")
	}

	m := widgets.Manifest{
		Name:   "nonexistent-widget",
		Binary: "/usr/local/bin/orcai-widget-does-not-exist-xyzzy",
	}

	// tmux new-window with a bad binary typically succeeds at the tmux level
	// (it opens a window and the shell reports command not found). However, if
	// there's no tmux session to target, tmux will return an error. We use a
	// deliberately invalid session name to trigger that tmux-level error, which
	// lets us confirm Launch propagates exec errors without requiring a live
	// tmux session.
	err := widgets.Launch(m, "no-such-session-xyzzy")
	// tmux new-window returns a non-zero exit code when targeting a session that
	// does not exist, so Launch must propagate that as a non-nil error.
	if err == nil {
		t.Error("Launch with bad session expected error, got nil")
	}
}
