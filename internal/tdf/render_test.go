package tdf_test

import (
	"strings"
	"testing"

	"github.com/8op-org/gl1tch/internal/tdf"
)

func TestRenderString_LineCount(t *testing.T) {
	f, err := tdf.LoadEmbedded("amnesiax")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	lines := tdf.RenderString("GL1TCH", f)
	if len(lines) != int(f.Height) {
		t.Errorf("expected %d lines, got %d", f.Height, len(lines))
	}
}

func TestRenderString_NonEmpty(t *testing.T) {
	f, err := tdf.LoadEmbedded("amnesiax")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	lines := tdf.RenderString("HI", f)
	for i, l := range lines {
		if strings.TrimSpace(tdf.StripANSI(l)) == "" {
			t.Errorf("line %d is blank", i)
		}
	}
}

func TestRenderString_SkipsMissingGlyphs(t *testing.T) {
	f, err := tdf.LoadEmbedded("amnesiax")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	// Should not panic on chars outside the font
	lines := tdf.RenderString("A\x01B", f)
	if len(lines) == 0 {
		t.Error("expected non-empty output")
	}
}

func TestCP437_KnownValues(t *testing.T) {
	cases := []struct {
		b    byte
		want rune
	}{
		{0x20, ' '},
		{0x41, 'A'},
		{0xDB, '█'},
		{0xDC, '▄'},
		{0xDF, '▀'},
		{0xB0, '░'},
		{0xB1, '▒'},
		{0xB2, '▓'},
		{0xC4, '─'},
		{0xCD, '═'},
		{0xFF, ' '},
	}
	for _, c := range cases {
		got := tdf.CP437ToRune(c.b)
		if got != c.want {
			t.Errorf("CP437[0x%02X]: got %q (%U), want %q (%U)", c.b, got, got, c.want, c.want)
		}
	}
}
