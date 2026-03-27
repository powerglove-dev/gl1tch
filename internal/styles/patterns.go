package styles

import "strings"

// PatternDef defines a tileable decorative header pattern.
type PatternDef struct {
	Name   string
	Top    string // character sequence for top row (tiled left-to-right)
	Bottom string // character sequence for bottom row (tiled left-to-right); defaults to Top reversed
}

// Patterns is the named pattern library.
// Characters are Unicode equivalents of classic CP437 block/line drawing characters
// that render correctly in UTF-8 terminals.
var Patterns = map[string]PatternDef{
	"waves": {
		Name:   "waves",
		Top:    "≈≈≈~≈≈≈~",
		Bottom: "~≈≈≈~≈≈≈",
	},
	"texture": {
		Name:   "texture",
		Top:    "▒░▒▒░▒▒░",
		Bottom: "░▒░░▒░░▒",
	},
	"checkerboard": {
		Name:   "checkerboard",
		Top:    "▀▄▀▄▀▄▀▄",
		Bottom: "▄▀▄▀▄▀▄▀",
	},
	"double-line": {
		Name:   "double-line",
		Top:    "══════════",
		Bottom: "══════════",
	},
	"shadow": {
		Name:   "shadow",
		Top:    "▓▒░▓▒░▓▒░",
		Bottom: "░▒▓░▒▓░▒",
	},
	"noise": {
		Name:   "noise",
		Top:    "·∙•·∙•·∙•",
		Bottom: "•∙·•∙·•∙",
	},
	"brick": {
		Name:   "brick",
		Top:    "▄█▄██▄█▄█",
		Bottom: "▀█▀██▀█▀█",
	},
	"scanline": {
		Name:   "scanline",
		Top:    "─ ─ ─ ─ ─",
		Bottom: "─ ─ ─ ─ ─",
	},
}

// TilePattern takes a sequence string and repeats it to fill exactly width
// visible characters. Width is counted in runes (not bytes) since these are
// multi-byte UTF-8 characters.
func TilePattern(seq string, width int) string {
	if seq == "" || width <= 0 {
		return strings.Repeat(" ", width)
	}
	runes := []rune(seq)
	out := make([]rune, 0, width)
	for len(out) < width {
		out = append(out, runes...)
	}
	return string(out[:width])
}

// MirrorPattern is like TilePattern but reverses the rune sequence first,
// producing a mirrored tile that can be used for a bottom row complementing
// the top row.
func MirrorPattern(seq string, width int) string {
	runes := []rune(seq)
	for i, j := 0, len(runes)-1; i < j; i, j = i+1, j-1 {
		runes[i], runes[j] = runes[j], runes[i]
	}
	return TilePattern(string(runes), width)
}
