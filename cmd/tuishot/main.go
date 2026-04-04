// tuishot reads raw ANSI terminal output from stdin (e.g. tmux capture-pane -p -e)
// and renders it as a PNG image with full 24-bit color support.
//
// Usage:
//
//	tmux capture-pane -p -e -t <pane> | tuishot -o out.png
package main

import (
	"bufio"
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"io"
	"os"
	"strconv"
	"strings"
	"unicode/utf8"

	"golang.org/x/image/font"
	"golang.org/x/image/font/gofont/gomono"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"
)

// Font metrics — set in init() from the Go Mono face.
var (
	monoFace font.Face
	charW    int // cell width in pixels
	charH    int // cell height in pixels
	baseline int // offset from cell top to text baseline
)

const (
	fontPt = 13.0
	fontDPI = 96.0
	padX   = 32
	padY   = 24
)

func init() {
	f, err := opentype.Parse(gomono.TTF)
	if err != nil {
		panic("tuishot: parse gomono: " + err.Error())
	}
	face, err := opentype.NewFace(f, &opentype.FaceOptions{
		Size:    fontPt,
		DPI:     fontDPI,
		Hinting: font.HintingFull,
	})
	if err != nil {
		panic("tuishot: create face: " + err.Error())
	}
	monoFace = face

	m := face.Metrics()
	charH = (m.Ascent + m.Descent).Ceil()
	baseline = m.Ascent.Ceil()

	// Go Mono is monospace — all glyphs share the same advance. Measure 'M'.
	adv, ok := face.GlyphAdvance('M')
	if !ok {
		panic("tuishot: cannot measure glyph advance")
	}
	charW = adv.Ceil()
}

// Dracula palette defaults — matches gl1tch theme.
var (
	defaultBg = color.RGBA{0x28, 0x2a, 0x36, 0xff} // #282a36
	defaultFg = color.RGBA{0xf8, 0xf8, 0xf2, 0xff} // #f8f8f2
)

// Standard 16 ANSI colors mapped to Dracula palette.
var ansi16 = [16]color.RGBA{
	{0x21, 0x22, 0x2c, 0xff}, // 0  black
	{0xff, 0x55, 0x55, 0xff}, // 1  red
	{0x50, 0xfa, 0x7b, 0xff}, // 2  green
	{0xf1, 0xfa, 0x8c, 0xff}, // 3  yellow
	{0xbd, 0x93, 0xf9, 0xff}, // 4  blue (Dracula purple)
	{0xff, 0x79, 0xc6, 0xff}, // 5  magenta
	{0x8b, 0xe9, 0xfd, 0xff}, // 6  cyan
	{0xf8, 0xf8, 0xf2, 0xff}, // 7  white
	{0x62, 0x72, 0xa4, 0xff}, // 8  bright black
	{0xff, 0x6e, 0x6e, 0xff}, // 9  bright red
	{0x69, 0xff, 0x94, 0xff}, // 10 bright green
	{0xff, 0xff, 0xa5, 0xff}, // 11 bright yellow
	{0xd6, 0xac, 0xff, 0xff}, // 12 bright blue
	{0xff, 0x92, 0xdf, 0xff}, // 13 bright magenta
	{0xa4, 0xff, 0xff, 0xff}, // 14 bright cyan
	{0xff, 0xff, 0xff, 0xff}, // 15 bright white
}

func ansi256(n int) color.RGBA {
	if n < 16 {
		return ansi16[n]
	}
	if n < 232 {
		n -= 16
		scale := func(v int) uint8 {
			if v == 0 {
				return 0
			}
			return uint8(55 + v*40)
		}
		return color.RGBA{scale(n / 36), scale((n / 6) % 6), scale(n % 6), 0xff}
	}
	g := uint8(8 + (n-232)*10)
	return color.RGBA{g, g, g, 0xff}
}

// cell is a single rendered terminal character.
type cell struct {
	ch rune
	fg color.RGBA
	bg color.RGBA
}

// renderState tracks current SGR attributes.
type renderState struct {
	fg, bg color.RGBA
}

func newState() renderState { return renderState{fg: defaultFg, bg: defaultBg} }

// applySGR applies a semicolon-separated SGR parameter string to the state.
func applySGR(params string, s *renderState) {
	if params == "" || params == "0" {
		*s = newState()
		return
	}
	parts := strings.Split(params, ";")
	for i := 0; i < len(parts); i++ {
		n, err := strconv.Atoi(strings.TrimSpace(parts[i]))
		if err != nil {
			continue
		}
		switch {
		case n == 0:
			*s = newState()
		case n >= 30 && n <= 37:
			s.fg = ansi16[n-30]
		case n == 39:
			s.fg = defaultFg
		case n >= 40 && n <= 47:
			s.bg = ansi16[n-40]
		case n == 49:
			s.bg = defaultBg
		case n >= 90 && n <= 97:
			s.fg = ansi16[n-90+8]
		case n >= 100 && n <= 107:
			s.bg = ansi16[n-100+8]
		case n == 38:
			if i+1 >= len(parts) {
				break
			}
			mode, _ := strconv.Atoi(parts[i+1])
			switch mode {
			case 2:
				if i+4 < len(parts) {
					r, _ := strconv.Atoi(parts[i+2])
					g, _ := strconv.Atoi(parts[i+3])
					b, _ := strconv.Atoi(parts[i+4])
					s.fg = color.RGBA{uint8(r), uint8(g), uint8(b), 0xff}
					i += 4
				}
			case 5:
				if i+2 < len(parts) {
					idx, _ := strconv.Atoi(parts[i+2])
					s.fg = ansi256(idx)
					i += 2
				}
			}
		case n == 48:
			if i+1 >= len(parts) {
				break
			}
			mode, _ := strconv.Atoi(parts[i+1])
			switch mode {
			case 2:
				if i+4 < len(parts) {
					r, _ := strconv.Atoi(parts[i+2])
					g, _ := strconv.Atoi(parts[i+3])
					b, _ := strconv.Atoi(parts[i+4])
					s.bg = color.RGBA{uint8(r), uint8(g), uint8(b), 0xff}
					i += 4
				}
			case 5:
				if i+2 < len(parts) {
					idx, _ := strconv.Atoi(parts[i+2])
					s.bg = ansi256(idx)
					i += 2
				}
			}
		}
	}
}

// parseLine converts one ANSI-encoded line into a slice of cells.
func parseLine(line string, s *renderState) []cell {
	var cells []cell
	i := 0
	for i < len(line) {
		b := line[i]
		// ESC [ — CSI sequence
		if b == 0x1b && i+1 < len(line) && line[i+1] == '[' {
			i += 2
			start := i
			for i < len(line) && (line[i] < 0x40 || line[i] > 0x7e) {
				i++
			}
			if i < len(line) {
				final := line[i]
				params := line[start:i]
				i++
				if final == 'm' {
					applySGR(params, s)
				}
			}
			continue
		}
		if b == 0x1b {
			i++
			if i < len(line) {
				i++
			}
			continue
		}
		if b < 0x20 && b != '\t' {
			i++
			continue
		}
		r, size := utf8.DecodeRuneInString(line[i:])
		if (r == utf8.RuneError && size == 1) || r == 0 {
			i++
			continue
		}
		cells = append(cells, cell{ch: r, fg: s.fg, bg: s.bg})
		i += size
	}
	return cells
}

// parse reads ANSI lines from r and returns a 2-D grid of cells.
func parse(r io.Reader) [][]cell {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 4<<20), 4<<20)
	s := newState()
	var grid [][]cell
	for scanner.Scan() {
		grid = append(grid, parseLine(scanner.Text(), &s))
	}
	return grid
}

// fillRect draws a clipped filled rectangle.
func fillRect(img *image.RGBA, x, y, w, h, px, py int, c color.RGBA) {
	x0, y0 := x, y
	x1, y1 := x+w, y+h
	if x0 < px { x0 = px }
	if y0 < py { y0 = py }
	if x1 > px+charW { x1 = px + charW }
	if y1 > py+charH { y1 = py + charH }
	if x0 >= x1 || y0 >= y1 { return }
	draw.Draw(img, image.Rect(x0, y0, x1, y1), &image.Uniform{c}, image.Point{}, draw.Src)
}

func drawHLine(img *image.RGBA, x0, x1, y int, c color.RGBA) {
	for x := x0; x <= x1; x++ {
		img.SetRGBA(x, y, c)
	}
}

func drawVLine(img *image.RGBA, y0, y1, x int, c color.RGBA) {
	for y := y0; y <= y1; y++ {
		img.SetRGBA(x, y, c)
	}
}

// drawSpecialChar renders Unicode block elements, box drawing, and common TUI
// glyphs as filled geometry. Returns true if handled.
func drawSpecialChar(img *image.RGBA, r rune, fg color.RGBA, px, py int) bool {
	w, h := charW, charH
	mx := px + w/2
	my := py + h/2

	switch r {
	// ── Block elements U+2580–U+259F ─────────────────────────────────────────
	case 0x2580: fillRect(img, px, py, w, h/2, px, py, fg);                     return true // ▀
	case 0x2581: fillRect(img, px, py+h*7/8, w, h-h*7/8, px, py, fg);           return true // ▁
	case 0x2582: fillRect(img, px, py+h*3/4, w, h-h*3/4, px, py, fg);           return true // ▂
	case 0x2583: fillRect(img, px, py+h*5/8, w, h-h*5/8, px, py, fg);           return true // ▃
	case 0x2584: fillRect(img, px, py+(h+1)/2, w, h-(h+1)/2, px, py, fg);       return true // ▄
	case 0x2585: fillRect(img, px, py+h*3/8, w, h-h*3/8, px, py, fg);           return true // ▅
	case 0x2586: fillRect(img, px, py+h/4, w, h-h/4, px, py, fg);               return true // ▆
	case 0x2587: fillRect(img, px, py+h/8, w, h-h/8, px, py, fg);               return true // ▇
	case 0x2588: fillRect(img, px, py, w, h, px, py, fg);                        return true // █
	case 0x2589: fillRect(img, px, py, w*7/8, h, px, py, fg);                   return true // ▉
	case 0x258A: fillRect(img, px, py, w*3/4, h, px, py, fg);                   return true // ▊
	case 0x258B: fillRect(img, px, py, w*5/8, h, px, py, fg);                   return true // ▋
	case 0x258C: fillRect(img, px, py, (w+1)/2, h, px, py, fg);                 return true // ▌
	case 0x258D: fillRect(img, px, py, w*3/8, h, px, py, fg);                   return true // ▍
	case 0x258E: fillRect(img, px, py, w/4, h, px, py, fg);                     return true // ▎
	case 0x258F: fillRect(img, px, py, imax(w/8, 1), h, px, py, fg);            return true // ▏
	case 0x2590: fillRect(img, px+w/2, py, w-w/2, h, px, py, fg);               return true // ▐
	case 0x2591: // ░ Light shade
		for dy := py; dy < py+h; dy++ {
			for dx := px; dx < px+w; dx++ {
				if (dx%2 == 0) == (dy%2 == 0) { img.SetRGBA(dx, dy, fg) }
			}
		}
		return true
	case 0x2592: // ▒ Medium shade
		for dy := py; dy < py+h; dy++ {
			for dx := px; dx < px+w; dx++ {
				if (dx+dy)%2 == 0 { img.SetRGBA(dx, dy, fg) }
			}
		}
		return true
	case 0x2593: // ▓ Dark shade
		for dy := py; dy < py+h; dy++ {
			for dx := px; dx < px+w; dx++ {
				if (dx%2 != 0) || (dy%2 != 0) { img.SetRGBA(dx, dy, fg) }
			}
		}
		return true
	case 0x2594: fillRect(img, px, py, w, imax(h/8, 1), px, py, fg);            return true // ▔
	case 0x2595: fillRect(img, px+w*7/8, py, w-w*7/8, h, px, py, fg);           return true // ▕
	case 0x2596: fillRect(img, px, py+(h+1)/2, (w+1)/2, h-(h+1)/2, px, py, fg); return true // ▖
	case 0x2597: fillRect(img, px+w/2, py+(h+1)/2, w-w/2, h-(h+1)/2, px, py, fg); return true // ▗
	case 0x2598: fillRect(img, px, py, (w+1)/2, h/2, px, py, fg);               return true // ▘
	case 0x2599: // ▙
		fillRect(img, px, py, (w+1)/2, h/2, px, py, fg)
		fillRect(img, px, py+(h+1)/2, w, h-(h+1)/2, px, py, fg)
		return true
	case 0x259A: // ▚
		fillRect(img, px, py, (w+1)/2, h/2, px, py, fg)
		fillRect(img, px+w/2, py+(h+1)/2, w-w/2, h-(h+1)/2, px, py, fg)
		return true
	case 0x259B: // ▛
		fillRect(img, px, py, w, h/2, px, py, fg)
		fillRect(img, px, py+(h+1)/2, (w+1)/2, h-(h+1)/2, px, py, fg)
		return true
	case 0x259C: // ▜
		fillRect(img, px, py, w, h/2, px, py, fg)
		fillRect(img, px+w/2, py+(h+1)/2, w-w/2, h-(h+1)/2, px, py, fg)
		return true
	case 0x259D: fillRect(img, px+w/2, py, w-w/2, h/2, px, py, fg);             return true // ▝
	case 0x259E: // ▞
		fillRect(img, px+w/2, py, w-w/2, h/2, px, py, fg)
		fillRect(img, px, py+(h+1)/2, (w+1)/2, h-(h+1)/2, px, py, fg)
		return true
	case 0x259F: // ▟
		fillRect(img, px+w/2, py, w-w/2, h/2, px, py, fg)
		fillRect(img, px, py+(h+1)/2, w, h-(h+1)/2, px, py, fg)
		return true
	}

	// ── Box drawing U+2500–U+257F ─────────────────────────────────────────────
	if r >= 0x2500 && r <= 0x257F {
		switch {
		case r == 0x2500 || r == 0x2501 || r == 0x254C || r == 0x254D:
			drawHLine(img, px, px+w-1, my, fg)
		case r == 0x2502 || r == 0x2503 || r == 0x254E || r == 0x254F:
			drawVLine(img, py, py+h-1, mx, fg)
		case r >= 0x250C && r <= 0x250F: // ┌ top-left
			drawHLine(img, mx, px+w-1, my, fg); drawVLine(img, my, py+h-1, mx, fg)
		case r >= 0x2510 && r <= 0x2513: // ┐ top-right
			drawHLine(img, px, mx, my, fg); drawVLine(img, my, py+h-1, mx, fg)
		case r >= 0x2514 && r <= 0x2517: // └ bottom-left
			drawHLine(img, mx, px+w-1, my, fg); drawVLine(img, py, my, mx, fg)
		case r >= 0x2518 && r <= 0x251B: // ┘ bottom-right
			drawHLine(img, px, mx, my, fg); drawVLine(img, py, my, mx, fg)
		case r >= 0x251C && r <= 0x2523: // ├
			drawHLine(img, mx, px+w-1, my, fg); drawVLine(img, py, py+h-1, mx, fg)
		case r >= 0x2524 && r <= 0x252B: // ┤
			drawHLine(img, px, mx, my, fg); drawVLine(img, py, py+h-1, mx, fg)
		case r >= 0x252C && r <= 0x2533: // ┬
			drawHLine(img, px, px+w-1, my, fg); drawVLine(img, my, py+h-1, mx, fg)
		case r >= 0x2534 && r <= 0x253B: // ┴
			drawHLine(img, px, px+w-1, my, fg); drawVLine(img, py, my, mx, fg)
		case r >= 0x253C && r <= 0x254B: // ┼
			drawHLine(img, px, px+w-1, my, fg); drawVLine(img, py, py+h-1, mx, fg)
		case r == 0x2550: // ═
			drawHLine(img, px, px+w-1, my-1, fg); drawHLine(img, px, px+w-1, my+1, fg)
		case r == 0x2551: // ║
			drawVLine(img, py, py+h-1, mx-1, fg); drawVLine(img, py, py+h-1, mx+1, fg)
		case r >= 0x2552 && r <= 0x256C:
			drawHLine(img, px, px+w-1, my, fg); drawVLine(img, py, py+h-1, mx, fg)
		case r == 0x2571:
			for i := 0; i < h; i++ { img.SetRGBA(px+(w-1)*(h-1-i)/(h-1), py+i, fg) }
		case r == 0x2572:
			for i := 0; i < h; i++ { img.SetRGBA(px+(w-1)*i/(h-1), py+i, fg) }
		case r == 0x2573:
			for i := 0; i < h; i++ {
				img.SetRGBA(px+(w-1)*i/(h-1), py+i, fg)
				img.SetRGBA(px+(w-1)*(h-1-i)/(h-1), py+i, fg)
			}
		}
		return true
	}

	// ── Misc TUI glyphs ───────────────────────────────────────────────────────
	switch r {
	case 0x2022, 0x25CF, 0x2219: // ● • ∙ — bullet
		for dy := -2; dy <= 2; dy++ {
			for dx := -2; dx <= 2; dx++ {
				if dx*dx+dy*dy <= 5 { img.SetRGBA(mx+dx, my+dy, fg) }
			}
		}
		return true
	case 0x25CB, 0x25CC: // ○ ◌ open circle
		for dy := -2; dy <= 2; dy++ {
			for dx := -2; dx <= 2; dx++ {
				d := dx*dx + dy*dy
				if d >= 4 && d <= 6 { img.SetRGBA(mx+dx, my+dy, fg) }
			}
		}
		return true
	case 0x2026: // … horizontal ellipsis
		for _, dx := range []int{-2, 0, 2} {
			img.SetRGBA(mx+dx, my, fg); img.SetRGBA(mx+dx, my+1, fg)
		}
		return true
	case 0x00B7, 0x2027: // · middle dot
		img.SetRGBA(mx, my, fg); img.SetRGBA(mx+1, my, fg)
		return true
	case 0x25B6, 0x25BA: // ▶ ► right triangle
		for i := 0; i < w/2; i++ { drawVLine(img, my-i, my+i, px+i, fg) }
		return true
	case 0x25C0, 0x25C4: // ◀ ◄ left triangle
		for i := 0; i < w/2; i++ { drawVLine(img, my-i, my+i, px+w-1-i, fg) }
		return true
	}

	return false
}

func imax(a, b int) int {
	if a > b { return a }
	return b
}

// render draws the cell grid onto a new RGBA image.
// minCols and minRows force a minimum canvas size; 0 means use actual content size.
func render(grid [][]cell, minCols, minRows int) *image.RGBA {
	cols := 0
	for _, row := range grid {
		if len(row) > cols {
			cols = len(row)
		}
	}
	rows := len(grid)
	if minCols > cols {
		cols = minCols
	}
	if minRows > rows {
		rows = minRows
	}
	if rows == 0 || cols == 0 {
		return image.NewRGBA(image.Rect(0, 0, 1, 1))
	}

	w := cols*charW + 2*padX
	h := rows*charH + 2*padY
	img := image.NewRGBA(image.Rect(0, 0, w, h))

	draw.Draw(img, img.Bounds(), &image.Uniform{defaultBg}, image.Point{}, draw.Src)

	for y, row := range grid {
		for x, c := range row {
			px := padX + x*charW
			py := padY + y*charH

			if c.bg != defaultBg {
				draw.Draw(img,
					image.Rect(px, py, px+charW, py+charH),
					&image.Uniform{c.bg}, image.Point{}, draw.Src)
			}

			if c.ch == ' ' || c.ch == 0 || c.ch == utf8.RuneError {
				continue
			}

			if drawSpecialChar(img, c.ch, c.fg, px, py) {
				continue
			}

			// Go Mono for everything else (covers full Latin + most Unicode).
			d := &font.Drawer{
				Dst:  img,
				Src:  &image.Uniform{c.fg},
				Face: monoFace,
				Dot:  fixed.P(px, py+baseline),
			}
			d.DrawString(string(c.ch))
		}
	}
	return img
}

func main() {
	outPath := flag.String("o", "out.png", "output PNG file path")
	minCols := flag.Int("cols", 200, "minimum canvas width in terminal columns (0 = auto)")
	minRows := flag.Int("rows", 50, "minimum canvas height in terminal rows (0 = auto)")
	flag.Parse()

	grid := parse(os.Stdin)
	img := render(grid, *minCols, *minRows)

	f, err := os.Create(*outPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "tuishot:", err)
		os.Exit(1)
	}
	defer f.Close() //nolint:errcheck

	if err := png.Encode(f, img); err != nil {
		fmt.Fprintln(os.Stderr, "tuishot:", err)
		os.Exit(1)
	}
}
