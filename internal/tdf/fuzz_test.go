//go:build go1.18

package tdf

import "testing"

func FuzzParse(f *testing.F) {
	// Seed corpus: valid minimal TDF header.
	f.Add([]byte("\x13TheDraw FONTS FILE\x1aplaceholder\x00\x00\x00\x00\x00\x00\x00\x00\x00\x08\x00"))
	// Seed corpus: empty input.
	f.Add([]byte{})
	// Seed corpus: clearly non-TDF data.
	f.Add([]byte("not a tdf file"))
	// Seed corpus: magic-only (no sub marker).
	f.Add([]byte("\x13TheDraw FONTS FILE"))
	// Seed corpus: magic + sub + truncated name.
	f.Add([]byte("\x13TheDraw FONTS FILE\x1aABC"))

	f.Fuzz(func(t *testing.T, data []byte) {
		// Must never panic regardless of input.
		font, _ := Parse(data)
		if font != nil {
			_, _ = font.Render("TEST", 200)
			_ = font.MeasureWidth("TEST")
		}
	})
}
