package panelrender_test

import (
	"strings"
	"testing"

	"github.com/adam-stokes/orcai/internal/panelrender"
)

// TestOverlayCenter_TitleBarPreserved verifies that when an overlay is exactly
// as tall as the base (startRow would be 0), the top row of the base is not
// overwritten by the overlay.
func TestOverlayCenter_TitleBarPreserved(t *testing.T) {
	base := strings.Join([]string{
		"TITLE BAR",
		"row1",
		"row2",
		"row3",
	}, "\n")
	overlay := strings.Join([]string{
		"[top border]",
		"[content1]",
		"[content2]",
		"[bottom]",
	}, "\n")

	result := panelrender.OverlayCenter(base, overlay, 40, 4)
	lines := strings.Split(result, "\n")

	if len(lines) == 0 {
		t.Fatal("empty result")
	}
	if strings.Contains(lines[0], "[top border]") {
		t.Errorf("row 0 (title bar) was overwritten by overlay: %q", lines[0])
	}
	if !strings.Contains(lines[0], "TITLE BAR") {
		t.Errorf("row 0 should still contain title bar, got: %q", lines[0])
	}
}

// TestOverlayCenter_SmallOverlay verifies that small overlays are still centered
// correctly (startRow stays > 1, so the min-1 clamp has no effect).
func TestOverlayCenter_SmallOverlay(t *testing.T) {
	base := strings.TrimSuffix(strings.Repeat("base line\n", 20), "\n")
	overlay := "line1\nline2\nline3"

	result := panelrender.OverlayCenter(base, overlay, 20, 20)
	lines := strings.Split(result, "\n")

	if strings.Contains(lines[0], "line1") {
		t.Errorf("small overlay should be vertically centered, not at top")
	}
	if strings.Contains(lines[1], "line1") {
		t.Errorf("small overlay should not appear at row 1")
	}

	found := false
	for _, l := range lines {
		if strings.Contains(l, "line1") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("line1 not found anywhere in result — overlay may have been dropped")
	}
}

// TestOverlayCenter_TallOverlayClipped verifies rows beyond h are not appended.
func TestOverlayCenter_TallOverlayClipped(t *testing.T) {
	base := strings.Repeat("base\n", 5)
	base = strings.TrimSuffix(base, "\n") // 5 lines
	overlay := strings.Join([]string{"a", "b", "c", "d", "e", "f", "g", "h"}, "\n")

	result := panelrender.OverlayCenter(base, overlay, 10, 5)
	lines := strings.Split(result, "\n")

	if len(lines) > 5 {
		t.Errorf("result should be capped at h=5 lines, got %d", len(lines))
	}
}
