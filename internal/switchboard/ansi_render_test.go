package switchboard_test

import (
	"strings"
	"testing"

	"github.com/adam-stokes/orcai/internal/switchboard"
	"github.com/adam-stokes/orcai/internal/themes"
)

// TestRenderHeader_NilBundle falls back to plain text.
func TestRenderHeader_NilBundle(t *testing.T) {
	got := switchboard.RenderHeader(nil, "pipelines", 200)
	if got != "PIPELINES" {
		t.Errorf("RenderHeader(nil, pipelines, 200) = %q, want %q", got, "PIPELINES")
	}
}

// TestRenderHeader_NilHeaderBytes falls back to plain text.
func TestRenderHeader_NilHeaderBytes(t *testing.T) {
	b := &themes.Bundle{}
	got := switchboard.RenderHeader(b, "pipelines", 200)
	if got != "PIPELINES" {
		t.Errorf("RenderHeader(nilHeaderBytes, pipelines, 200) = %q, want %q", got, "PIPELINES")
	}
}

// TestRenderHeader_NarrowTerminal falls back to plain text when terminal is narrower than sprite.
func TestRenderHeader_NarrowTerminal(t *testing.T) {
	// Create a fake sprite that is 80 visible characters wide.
	// We'll use a simple ASCII string for the sprite (no ANSI).
	sprite := strings.Repeat("X", 80) + "\n"
	b := &themes.Bundle{
		HeaderBytes: map[string][]byte{
			"pipelines": []byte(sprite),
		},
	}
	// Terminal width of 40 < sprite width of 80 → should fall back.
	got := switchboard.RenderHeader(b, "pipelines", 40)
	if got != "PIPELINES" {
		t.Errorf("RenderHeader(narrow=40, sprite=80) = %q, want plain text %q", got, "PIPELINES")
	}
}

// TestRenderHeader_WideTerminal returns sprite bytes + reset suffix.
func TestRenderHeader_WideTerminal(t *testing.T) {
	spriteContent := "\x1b[35mPIPELINES\x1b[0m"
	b := &themes.Bundle{
		HeaderBytes: map[string][]byte{
			"pipelines": []byte(spriteContent),
		},
	}
	got := switchboard.RenderHeader(b, "pipelines", 200)
	if !strings.HasSuffix(got, "\x1b[0m") {
		t.Errorf("RenderHeader should end with reset, got: %q", got)
	}
	if !strings.Contains(got, "PIPELINES") {
		t.Errorf("RenderHeader should contain sprite content, got: %q", got)
	}
}

// TestRenderHeader_WithSprite tests the sprite path with a full bundle.
func TestRenderHeader_WithSprite(t *testing.T) {
	b := &themes.Bundle{
		HeaderBytes: map[string][]byte{
			"pipelines": []byte("\x1b[35mPIPELINES\x1b[0m"),
		},
	}
	got := switchboard.RenderHeader(b, "pipelines", 200)
	if !strings.HasSuffix(got, "\x1b[0m") {
		t.Errorf("RenderHeader should end with reset, got: %q", got)
	}
}

// TestRenderHeader_UnknownPanel falls back to uppercase panel name.
func TestRenderHeader_UnknownPanel(t *testing.T) {
	got := switchboard.RenderHeader(nil, "my_panel", 200)
	if got != "MY_PANEL" {
		t.Errorf("RenderHeader(nil, my_panel, 200) = %q, want %q", got, "MY_PANEL")
	}
}

// TestRenderHeader_EmptyHeaderBytes falls back to plain text.
func TestRenderHeader_EmptyHeaderBytes(t *testing.T) {
	b := &themes.Bundle{
		HeaderBytes: map[string][]byte{
			"pipelines": []byte{},
		},
	}
	got := switchboard.RenderHeader(b, "pipelines", 200)
	if got != "PIPELINES" {
		t.Errorf("RenderHeader(empty bytes, pipelines, 200) = %q, want %q", got, "PIPELINES")
	}
}

// TestRenderHeader_ZeroTermWidth treats zero as "no width constraint" (any terminal fits).
func TestRenderHeader_ZeroTermWidth(t *testing.T) {
	spriteContent := strings.Repeat("X", 200) + "\n"
	b := &themes.Bundle{
		HeaderBytes: map[string][]byte{
			"pipelines": []byte(spriteContent),
		},
	}
	// termWidth=0 means unconstrained — should return the sprite.
	got := switchboard.RenderHeader(b, "pipelines", 0)
	if got == "PIPELINES" {
		t.Error("RenderHeader with termWidth=0 should not fall back to plain text")
	}
}
