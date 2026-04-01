package console_test

import (
	"strings"
	"testing"

	"github.com/powerglove-dev/gl1tch/internal/console"
	"github.com/powerglove-dev/gl1tch/internal/themes"
)

// ── RenderHeader (plain-text fallback) ───────────────────────────────────────

func TestRenderHeader_KnownPanel(t *testing.T) {
	cases := []struct{ panel, want string }{
		{"pipelines", "PIPELINES"},
		{"agent_runner", "AGENT RUNNER"},
		{"signal_board", "SIGNAL BOARD"},
		{"activity_feed", "ACTIVITY FEED"},
	}
	for _, c := range cases {
		got := console.RenderHeader(c.panel)
		if got != c.want {
			t.Errorf("RenderHeader(%q) = %q, want %q", c.panel, got, c.want)
		}
	}
}

func TestRenderHeader_UnknownPanel(t *testing.T) {
	got := console.RenderHeader("my_panel")
	if got != "MY_PANEL" {
		t.Errorf("RenderHeader(my_panel) = %q, want %q", got, "MY_PANEL")
	}
}

// ── SpriteLines ──────────────────────────────────────────────────────────────

func TestSpriteLines_NilBundle(t *testing.T) {
	if got := console.SpriteLines(nil, "pipelines", 200); got != nil {
		t.Errorf("SpriteLines(nil, ...) = %v, want nil", got)
	}
}

func TestSpriteLines_NilHeaderBytes(t *testing.T) {
	b := &themes.Bundle{}
	if got := console.SpriteLines(b, "pipelines", 200); got != nil {
		t.Errorf("SpriteLines(no bytes, ...) = %v, want nil", got)
	}
}

func TestSpriteLines_EmptyVariants(t *testing.T) {
	b := &themes.Bundle{HeaderBytes: map[string][][]byte{"pipelines": {}}}
	if got := console.SpriteLines(b, "pipelines", 200); got != nil {
		t.Errorf("SpriteLines(empty variants, ...) = %v, want nil", got)
	}
}

func TestSpriteLines_EmptyBytes(t *testing.T) {
	b := &themes.Bundle{HeaderBytes: map[string][][]byte{"pipelines": {{}}}}
	if got := console.SpriteLines(b, "pipelines", 200); got != nil {
		t.Errorf("SpriteLines(empty bytes, ...) = %v, want nil", got)
	}
}

func TestSpriteLines_NarrowPanel_ReturnsNil(t *testing.T) {
	// Sprite is 80 visible chars wide; panel is only 40. No fallback variant.
	sprite := strings.Repeat("X", 80) + "\n"
	b := &themes.Bundle{HeaderBytes: map[string][][]byte{"pipelines": {[]byte(sprite)}}}
	if got := console.SpriteLines(b, "pipelines", 40); got != nil {
		t.Errorf("SpriteLines(narrow=40, sprite=80) should be nil, got %v", got)
	}
}

func TestSpriteLines_FallsBackToNarrowVariant(t *testing.T) {
	// Wide variant (80 cols) doesn't fit, narrow variant (30 cols) should be used.
	wide := strings.Repeat("X", 80)
	narrow := strings.Repeat("Y", 30)
	b := &themes.Bundle{HeaderBytes: map[string][][]byte{
		"pipelines": {[]byte(wide), []byte(narrow)},
	}}
	lines := console.SpriteLines(b, "pipelines", 40)
	if lines == nil {
		t.Fatal("SpriteLines should fall back to narrow variant")
	}
	if !strings.Contains(lines[0], "Y") {
		t.Errorf("expected narrow variant (Y), got: %q", lines[0])
	}
}

func TestSpriteLines_WidePanel_ReturnsFirstVariant(t *testing.T) {
	content := "\x1b[35mPIPELINES\x1b[0m"
	b := &themes.Bundle{HeaderBytes: map[string][][]byte{"pipelines": {[]byte(content)}}}
	lines := console.SpriteLines(b, "pipelines", 200)
	if lines == nil {
		t.Fatal("SpriteLines(wide=200, sprite=9) should not be nil")
	}
	if len(lines) == 0 {
		t.Fatal("SpriteLines returned empty slice")
	}
	last := lines[len(lines)-1]
	if !strings.HasSuffix(last, "\x1b[0m") {
		t.Errorf("last line should end with reset, got: %q", last)
	}
}

func TestSpriteLines_ZeroWidth_Unconstrained(t *testing.T) {
	// panelWidth=0 means no width constraint — first variant is used.
	wide := strings.Repeat("X", 200)
	b := &themes.Bundle{HeaderBytes: map[string][][]byte{"pipelines": {[]byte(wide)}}}
	if got := console.SpriteLines(b, "pipelines", 0); got == nil {
		t.Error("SpriteLines(panelWidth=0) should not return nil (unconstrained)")
	}
}

func TestSpriteLines_MultiLine_PreservesLines(t *testing.T) {
	// Three-line sprite: only non-empty lines should be returned.
	content := "line1\nline2\nline3\n"
	b := &themes.Bundle{HeaderBytes: map[string][][]byte{"pipelines": {[]byte(content)}}}
	lines := console.SpriteLines(b, "pipelines", 200)
	if len(lines) != 3 {
		t.Errorf("SpriteLines 3-line sprite: got %d lines, want 3", len(lines))
	}
}

func TestSpriteLines_UnknownPanel_ReturnsNil(t *testing.T) {
	b := &themes.Bundle{HeaderBytes: map[string][][]byte{"pipelines": {[]byte("X")}}}
	if got := console.SpriteLines(b, "other_panel", 200); got != nil {
		t.Errorf("SpriteLines(unknown panel) = %v, want nil", got)
	}
}
