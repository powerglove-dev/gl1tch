package modal

import (
	"strings"
	"testing"

	"github.com/8op-org/gl1tch/internal/themes"
)

// TestRenderConfirm_WithNilBundle verifies that a nil bundle doesn't panic and
// returns a non-empty string with the Dracula fallback colors.
func TestRenderConfirm_WithNilBundle(t *testing.T) {
	cfg := Config{
		Bundle:  nil,
		Title:   "Test Quit?",
		Message: "Are you sure?",
	}
	result := RenderConfirm(cfg, 80, 24)
	if result == "" {
		t.Fatal("RenderConfirm returned empty string with nil bundle")
	}
	if !strings.Contains(result, "Test Quit?") {
		t.Errorf("expected title in output, got: %q", result)
	}
}

// TestRenderConfirm_WithBundle verifies that bundle colors are applied and
// the output is non-empty and contains the title and message.
func TestRenderConfirm_WithBundle(t *testing.T) {
	bundle := &themes.Bundle{
		Palette: themes.Palette{
			FG:     "#ffffff",
			Accent: "#ff79c6",
			Dim:    "#44475a",
			Error:  "#ff5555",
		},
		Modal: themes.Modal{
			Border:  "#50fa7b",
			TitleBG: "#50fa7b",
			TitleFG: "#282a36",
		},
	}
	cfg := Config{
		Bundle:  bundle,
		Title:   "Bundle Confirm",
		Message: "With bundle colors",
	}
	result := RenderConfirm(cfg, 80, 24)
	if result == "" {
		t.Fatal("RenderConfirm returned empty string with bundle")
	}
	if !strings.Contains(result, "Bundle Confirm") {
		t.Errorf("expected title in output, got: %q", result)
	}
	if !strings.Contains(result, "With bundle colors") {
		t.Errorf("expected message in output, got: %q", result)
	}
	// ResolveColors should return the bundle border color (not the fallback).
	c := ResolveColors(cfg)
	if c.Border != "#50fa7b" {
		t.Errorf("expected border #50fa7b, got %q", c.Border)
	}
	if c.TitleBG != "#50fa7b" {
		t.Errorf("expected titleBG #50fa7b, got %q", c.TitleBG)
	}
	if c.TitleFG != "#282a36" {
		t.Errorf("expected titleFG #282a36, got %q", c.TitleFG)
	}
}

// TestRenderAlert_WithNilBundle verifies RenderAlert with nil bundle doesn't panic.
func TestRenderAlert_WithNilBundle(t *testing.T) {
	cfg := Config{
		Bundle: nil,
		Title:  "Alert Title",
	}
	result := RenderAlert(cfg, "Something went wrong", 80, 24)
	if result == "" {
		t.Fatal("RenderAlert returned empty string with nil bundle")
	}
	if !strings.Contains(result, "Alert Title") {
		t.Errorf("expected title in output, got: %q", result)
	}
	if !strings.Contains(result, "Something went wrong") {
		t.Errorf("expected message in output, got: %q", result)
	}
}

// TestRenderScroll_WithNilBundle verifies RenderScroll with nil bundle doesn't panic.
func TestRenderScroll_WithNilBundle(t *testing.T) {
	cfg := Config{
		Bundle: nil,
		Title:  "Scroll Title",
	}
	lines := []string{"line one", "line two", "line three"}
	result := RenderScroll(cfg, lines, 0, 80, 24)
	if result == "" {
		t.Fatal("RenderScroll returned empty string with nil bundle")
	}
	if !strings.Contains(result, "Scroll Title") {
		t.Errorf("expected title in output, got: %q", result)
	}
}

// TestRenderScroll_Offset verifies that the offset parameter clips lines correctly.
func TestRenderScroll_Offset(t *testing.T) {
	cfg := Config{
		Bundle: nil,
		Title:  "Offset Test",
	}
	// Create 20 lines so the visible window (h-6 = 18 at h=24) is narrower.
	var lines []string
	for i := 0; i < 20; i++ {
		lines = append(lines, strings.Repeat("x", i+1))
	}

	// With offset=0, the first visible line should be "x" (1 char).
	result0 := RenderScroll(cfg, lines, 0, 80, 10) // visibleH = 10-6=4
	if !strings.Contains(result0, lines[0]) {
		t.Errorf("offset=0: expected first line in output")
	}

	// With offset=5, the first visible line should be "xxxxxx" (6 chars).
	result5 := RenderScroll(cfg, lines, 5, 80, 10) // visibleH = 4
	if !strings.Contains(result5, lines[5]) {
		t.Errorf("offset=5: expected line[5] %q in output, got %q", lines[5], result5)
	}
	if strings.Contains(result5, " "+lines[0]+" ") {
		t.Errorf("offset=5: unexpected first line in output")
	}
}
