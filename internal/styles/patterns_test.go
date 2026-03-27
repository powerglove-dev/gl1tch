package styles

import "testing"

func TestTilePattern_Widths(t *testing.T) {
	cases := []struct {
		seq   string
		width int
	}{
		{"≈≈≈~", 40},
		{"≈≈≈~", 80},
		{"▒░▒", 120},
		{"▀▄", 200},
	}
	for _, tc := range cases {
		got := TilePattern(tc.seq, tc.width)
		if runeLen := len([]rune(got)); runeLen != tc.width {
			t.Errorf("TilePattern(%q, %d) rune len = %d, want %d", tc.seq, tc.width, runeLen, tc.width)
		}
	}
}

// TestAllPatterns_DoNotPanic verifies all 8 named patterns tile correctly to
// 80 runes without panicking.
func TestAllPatterns_DoNotPanic(t *testing.T) {
	for name, def := range Patterns {
		_ = name
		got := TilePattern(def.Top, 80)
		if len([]rune(got)) != 80 {
			t.Errorf("pattern %q top: want 80 runes", name)
		}
		got2 := MirrorPattern(def.Bottom, 80)
		if len([]rune(got2)) != 80 {
			t.Errorf("pattern %q bottom: want 80 runes", name)
		}
	}
}

func TestTilePattern_Empty(t *testing.T) {
	got := TilePattern("", 10)
	if len([]rune(got)) != 10 {
		t.Errorf("TilePattern(\"\", 10) rune len = %d, want 10", len([]rune(got)))
	}
}

func TestTilePattern_ZeroWidth(t *testing.T) {
	got := TilePattern("abc", 0)
	if got != "" {
		t.Errorf("TilePattern(\"abc\", 0) = %q, want \"\"", got)
	}
}

func TestMirrorPattern_ReversesAndTiles(t *testing.T) {
	// "ab" reversed is "ba", tiled to 4 = "baba"
	got := MirrorPattern("ab", 4)
	if got != "baba" {
		t.Errorf("MirrorPattern(\"ab\", 4) = %q, want \"baba\"", got)
	}
}
