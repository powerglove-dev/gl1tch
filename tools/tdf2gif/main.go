// tdf2gif renders "GL1TCH" using the embedded inertia TDF font and writes a
// GIF image to stdout.
//
// Usage: go run ./tools/tdf2gif > site/public/glitch-logo.gif
package main

import (
	"image"
	"image/color"
	"image/gif"
	"os"

	"github.com/8op-org/gl1tch/internal/tdf"
)

// Cell dimensions in pixels.
const (
	cellW   = 12
	cellH   = 24
	padding = 16
)

// tdfPalette maps TDF colour indices 0–15 to RGBA, matching bbs.css variables.
var tdfPalette = color.Palette{
	color.RGBA{0x1c, 0x1c, 0x1c, 0xff}, // 0  black  / bg
	color.RGBA{0x87, 0x87, 0xaf, 0xff}, // 1  blue   → purple
	color.RGBA{0x87, 0xaf, 0x87, 0xff}, // 2  green
	color.RGBA{0x87, 0xaf, 0xaf, 0xff}, // 3  cyan
	color.RGBA{0xaf, 0x5f, 0x5f, 0xff}, // 4  red
	color.RGBA{0xaf, 0x87, 0xaf, 0xff}, // 5  magenta → pink
	color.RGBA{0xd7, 0xaf, 0x5f, 0xff}, // 6  brown   → yellow
	color.RGBA{0xbc, 0xbc, 0xbc, 0xff}, // 7  white   → fg
	color.RGBA{0x44, 0x44, 0x44, 0xff}, // 8  dark gray
	color.RGBA{0x87, 0x87, 0xaf, 0xff}, // 9  bright blue   → purple
	color.RGBA{0x87, 0xaf, 0x87, 0xff}, // 10 bright green
	color.RGBA{0x87, 0xaf, 0xaf, 0xff}, // 11 bright cyan
	color.RGBA{0xaf, 0x5f, 0x5f, 0xff}, // 12 bright red
	color.RGBA{0xaf, 0x87, 0xaf, 0xff}, // 13 bright magenta → pink
	color.RGBA{0xd7, 0xaf, 0x5f, 0xff}, // 14 bright yellow
	color.RGBA{0xbc, 0xbc, 0xbc, 0xff}, // 15 bright white  → fg
}

func main() {
	f, err := tdf.LoadEmbedded("inertia")
	if err != nil {
		os.Stderr.WriteString("tdf2gif: " + err.Error() + "\n")
		os.Exit(1)
	}

	text := []rune("GL1TCH")

	type glyphEntry struct {
		g   *tdf.Glyph
		sep int
	}
	var glyphs []glyphEntry
	for i, r := range text {
		g, ok := f.Glyph(r)
		if !ok {
			continue
		}
		sep := 0
		if i < len(text)-1 {
			sep = int(f.Spacing)
		}
		glyphs = append(glyphs, glyphEntry{g, sep})
	}

	totalCellW := 0
	for _, e := range glyphs {
		totalCellW += int(e.g.Width) + e.sep
	}
	fontH := int(f.Height)

	imgW := totalCellW*cellW + padding*2
	imgH := fontH*cellH + padding*2

	img := image.NewPaletted(image.Rect(0, 0, imgW, imgH), tdfPalette)
	// Fill background (index 0).
	for i := range img.Pix {
		img.Pix[i] = 0
	}

	x := padding
	for _, e := range glyphs {
		g := e.g
		for row := 0; row < fontH; row++ {
			for col := 0; col < int(g.Width); col++ {
				idx := row*int(g.Width) + col
				if idx >= len(g.Cells) {
					continue
				}
				cell := g.Cells[idx]
				px := x + col*cellW
				py := padding + row*cellH
				fg := cell.Color & 0x0f
				bg := (cell.Color >> 4) & 0x07
				drawCell(img, px, py, cell.Ch, fg, bg)
			}
		}
		x += (int(e.g.Width) + e.sep) * cellW
	}

	gif.Encode(os.Stdout, img, &gif.Options{NumColors: 16}) //nolint:errcheck
}

// drawCell renders one TDF cell at pixel position (x, y).
// Block and shade characters are rendered geometrically; others fall back to
// a full foreground block.
func drawCell(img *image.Paletted, x, y int, ch rune, fg, bg uint8) {
	b := img.Bounds()

	fill := func(rx, ry, rw, rh int, ci uint8) {
		for py := ry; py < ry+rh; py++ {
			for px := rx; px < rx+rw; px++ {
				if px < b.Min.X || py < b.Min.Y || px >= b.Max.X || py >= b.Max.Y {
					continue
				}
				img.SetColorIndex(px, py, ci)
			}
		}
	}

	// Ordered 4×4 dither matrix (Bayer) — threshold values 0-15.
	bayer4 := [4][4]int{
		{0, 8, 2, 10},
		{12, 4, 14, 6},
		{3, 11, 1, 9},
		{15, 7, 13, 5},
	}

	shade := func(density int) {
		// density: 4 = 25%, 8 = 50%, 12 = 75%
		for py := y; py < y+cellH; py++ {
			for px := x; px < x+cellW; px++ {
				if px < b.Min.X || py < b.Min.Y || px >= b.Max.X || py >= b.Max.Y {
					continue
				}
				threshold := bayer4[(py-y)%4][(px-x)%4]
				ci := bg
				if threshold < density {
					ci = fg
				}
				img.SetColorIndex(px, py, ci)
			}
		}
	}

	hw, hh := cellW/2, cellH/2

	fill(x, y, cellW, cellH, bg) // background always first

	switch ch {
	case ' ':
		// background only

	// ── Block elements ────────────────────────────────────────────────────────
	case '█': // FULL BLOCK
		fill(x, y, cellW, cellH, fg)
	case '▄': // LOWER HALF BLOCK
		fill(x, y+hh, cellW, cellH-hh, fg)
	case '▀': // UPPER HALF BLOCK
		fill(x, y, cellW, hh, fg)
	case '▌': // LEFT HALF BLOCK
		fill(x, y, hw, cellH, fg)
	case '▐': // RIGHT HALF BLOCK
		fill(x+hw, y, cellW-hw, cellH, fg)
	case '▖': fill(x, y+hh, hw, cellH-hh, fg)
	case '▗': fill(x+hw, y+hh, cellW-hw, cellH-hh, fg)
	case '▘': fill(x, y, hw, hh, fg)
	case '▝': fill(x+hw, y, cellW-hw, hh, fg)
	case '▙':
		fill(x, y, hw, cellH, fg)
		fill(x+hw, y+hh, cellW-hw, cellH-hh, fg)
	case '▛':
		fill(x, y, hw, cellH, fg)
		fill(x+hw, y, cellW-hw, hh, fg)
	case '▜':
		fill(x, y, cellW, hh, fg)
		fill(x+hw, y+hh, cellW-hw, cellH-hh, fg)
	case '▟':
		fill(x+hw, y, cellW-hw, cellH, fg)
		fill(x, y+hh, hw, cellH-hh, fg)

	// ── Shade characters (Bayer dithering) ────────────────────────────────────
	case '░': shade(4)  // LIGHT SHADE   ~25% fg
	case '▒': shade(8)  // MEDIUM SHADE  ~50% fg
	case '▓': shade(12) // DARK SHADE    ~75% fg

	default:
		// Any other visible character: full foreground block.
		fill(x, y, cellW, cellH, fg)
	}
}
