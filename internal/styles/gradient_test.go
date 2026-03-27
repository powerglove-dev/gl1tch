package styles

import (
	"strings"
	"testing"
)

func TestInterpolateRGB_BlackToWhite(t *testing.T) {
	result := InterpolateRGB("#000000", "#ffffff", 10)
	if len(result) != 10 {
		t.Fatalf("want 10 results, got %d", len(result))
	}
	// result[0] should be near #000000
	if result[0] != "#000000" {
		t.Errorf("result[0]: want #000000, got %q", result[0])
	}
	// result[9] should be near #ffffff
	if result[9] != "#ffffff" {
		t.Errorf("result[9]: want #ffffff, got %q", result[9])
	}
	// result[4] or result[5] should be near midpoint (~#7f7f7f or #808080)
	mid := result[4]
	r, g, b := hexToRGB(mid)
	// Each channel should be roughly 111 (4/9 * 255 ≈ 113) — allow ±10 tolerance
	if int(r) < 100 || int(r) > 125 {
		t.Errorf("result[4] R channel %d is not near midpoint (~111)", r)
	}
	if int(g) < 100 || int(g) > 125 {
		t.Errorf("result[4] G channel %d is not near midpoint (~111)", g)
	}
	if int(b) < 100 || int(b) > 125 {
		t.Errorf("result[4] B channel %d is not near midpoint (~111)", b)
	}
}

func TestInterpolateRGB_SingleStep(t *testing.T) {
	result := InterpolateRGB("#bd93f9", "#ff79c6", 1)
	if len(result) != 1 {
		t.Fatalf("want 1 result, got %d", len(result))
	}
	if result[0] != "#bd93f9" {
		t.Errorf("single step should be start color, got %q", result[0])
	}
}

func TestInterpolateRGB_ZeroSteps(t *testing.T) {
	result := InterpolateRGB("#000000", "#ffffff", 0)
	if result != nil {
		t.Errorf("want nil for 0 steps, got %v", result)
	}
}

func TestGradientStops_TwoStops(t *testing.T) {
	stops := GradientStops([]string{"#000000", "#ffffff"}, 10)
	if len(stops) != 10 {
		t.Fatalf("want 10, got %d", len(stops))
	}
	// First color should be near black
	r0, g0, b0 := hexToRGB(stops[0])
	if r0 != 0 || g0 != 0 || b0 != 0 {
		t.Errorf("first stop should be #000000, got %q", stops[0])
	}
	// Last color should be near white
	rL, gL, bL := hexToRGB(stops[9])
	if rL != 255 || gL != 255 || bL != 255 {
		t.Errorf("last stop should be #ffffff, got %q", stops[9])
	}
}

func TestGradientStops_OneStop(t *testing.T) {
	stops := GradientStops([]string{"#bd93f9"}, 5)
	if len(stops) != 5 {
		t.Fatalf("want 5, got %d", len(stops))
	}
	for _, s := range stops {
		if s != "#bd93f9" {
			t.Errorf("want #bd93f9, got %q", s)
		}
	}
}

func TestGradientStops_FourStops(t *testing.T) {
	stops := GradientStops([]string{"#ff0000", "#00ff00", "#0000ff", "#ffffff"}, 20)
	if len(stops) != 20 {
		t.Fatalf("want 20, got %d", len(stops))
	}
}

func TestGradientStops_Empty(t *testing.T) {
	stops := GradientStops([]string{}, 10)
	if stops != nil {
		t.Errorf("want nil for empty stops, got %v", stops)
	}
}

func TestGradientStops_ZeroWidth(t *testing.T) {
	stops := GradientStops([]string{"#ff0000", "#0000ff"}, 0)
	if stops != nil {
		t.Errorf("want nil for zero width, got %v", stops)
	}
}

func TestHexToFG24(t *testing.T) {
	esc := hexToFG24("#bd93f9")
	// Should produce ESC[38;2;189;147;249m
	expected := "\x1b[38;2;189;147;249m"
	if esc != expected {
		t.Errorf("hexToFG24: want %q, got %q", expected, esc)
	}
}

func TestHexToFG256(t *testing.T) {
	esc := hexToFG256("#000000")
	if !strings.HasPrefix(esc, "\x1b[38;5;") {
		t.Errorf("hexToFG256: expected 256-color escape, got %q", esc)
	}
	// Black (#000000) should map to index 16 (start of 6x6x6 cube with ri=gi=bi=0)
	expected := "\x1b[38;5;16m"
	if esc != expected {
		t.Errorf("hexToFG256 black: want %q, got %q", expected, esc)
	}
}

func TestRenderGradientBorder_Basic(t *testing.T) {
	content := "hello\nworld"
	result := RenderGradientBorder(content, []string{"#ff0000", "#0000ff"}, 5, 2)

	// Should contain the box-drawing characters
	if !strings.Contains(result, "╔") {
		t.Error("expected top-left corner ╔")
	}
	if !strings.Contains(result, "╗") {
		t.Error("expected top-right corner ╗")
	}
	if !strings.Contains(result, "╚") {
		t.Error("expected bottom-left corner ╚")
	}
	if !strings.Contains(result, "╝") {
		t.Error("expected bottom-right corner ╝")
	}
	if !strings.Contains(result, "═") {
		t.Error("expected top border with ═")
	}
	if !strings.Contains(result, "║") {
		t.Error("expected side border ║")
	}
	// Should contain reset escapes
	if !strings.Contains(result, "\x1b[0m") {
		t.Error("expected reset escape \\x1b[0m")
	}
}

func TestRenderGradientBorder_EmptyStops(t *testing.T) {
	content := "hello"
	result := RenderGradientBorder(content, []string{}, 5, 1)
	// Should return content unchanged when no stops
	if result != content {
		t.Errorf("expected unchanged content with empty stops, got %q", result)
	}
}
