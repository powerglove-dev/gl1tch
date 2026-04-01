package tdf_test

import (
	"testing"

	"github.com/8op-org/gl1tch/internal/tdf"
)

func TestLoadFont_Amnesiax(t *testing.T) {
	f, err := tdf.LoadEmbedded("amnesiax")
	if err != nil {
		t.Fatalf("load amnesiax: %v", err)
	}
	if f.Height == 0 {
		t.Error("font height is 0")
	}
	// amnesiax should have most printable ASCII chars
	for _, c := range "GLITCH" {
		if !f.HasGlyph(byte(c)) {
			t.Errorf("glyph missing for %c", c)
		}
	}
}

func TestLoadFont_InvalidMagic(t *testing.T) {
	_, err := tdf.Load([]byte("not a tdf file"))
	if err == nil {
		t.Error("expected error for invalid magic")
	}
}

func TestLoadFont_GlyphDimensions(t *testing.T) {
	f, err := tdf.LoadEmbedded("amnesiax")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	g, ok := f.Glyph('O')
	if !ok {
		t.Fatal("glyph 'O' missing")
	}
	if g.Width == 0 || g.Height == 0 {
		t.Errorf("glyph O has zero dimension: %dx%d", g.Width, g.Height)
	}
	if len(g.Cells) != int(g.Width)*int(f.Height) {
		t.Errorf("cell count %d != width*height %d", len(g.Cells), int(g.Width)*int(f.Height))
	}
}
