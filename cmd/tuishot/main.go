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
	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/math/fixed"
)

// Character cell dimensions for basicfont.Face7x13.
const (
	charW = 7
	charH = 13
	padX  = 32
	padY  = 24
)

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
			// Consume until a final byte (0x40–0x7e)
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
				// All other CSI sequences (cursor movement etc.) are silently dropped.
			}
			continue
		}
		// ESC alone or other ESC sequences — skip
		if b == 0x1b {
			i++
			if i < len(line) {
				i++ // consume one more byte
			}
			continue
		}
		// Control characters (except tab) — skip
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

// fillRect draws a filled rectangle [x,y,x+w,y+h) in colour c, clipped to [px,py,px+charW,py+charH).
func fillRect(img *image.RGBA, x, y, w, h, px, py int, c color.RGBA) {
	x0, y0 := x, y
	x1, y1 := x+w, y+h
	if x0 < px {
		x0 = px
	}
	if y0 < py {
		y0 = py
	}
	if x1 > px+charW {
		x1 = px + charW
	}
	if y1 > py+charH {
		y1 = py + charH
	}
	draw.Draw(img, image.Rect(x0, y0, x1, y1), &image.Uniform{c}, image.Point{}, draw.Src)
}

// drawHLine draws a 1-px horizontal line at row y from x=x0 to x=x1 (inclusive).
func drawHLine(img *image.RGBA, x0, x1, y int, c color.RGBA) {
	for x := x0; x <= x1; x++ {
		img.SetRGBA(x, y, c)
	}
}

// drawVLine draws a 1-px vertical line at col x from y=y0 to y=y1 (inclusive).
func drawVLine(img *image.RGBA, y0, y1, x int, c color.RGBA) {
	for y := y0; y <= y1; y++ {
		img.SetRGBA(x, y, c)
	}
}

// drawSpecialChar renders Unicode block elements (U+2580–U+259F), box drawing
// (U+2500–U+257F), and a handful of other common TUI glyphs geometrically.
// Returns true if the rune was handled (caller should skip basicfont).
func drawSpecialChar(img *image.RGBA, r rune, fg color.RGBA, px, py int) bool {
	w, h := charW, charH
	mx := px + w/2
	my := py + h/2

	// ── Block elements U+2580–U+259F ─────────────────────────────────────────
	switch r {
	case 0x2580: // ▀ UPPER HALF BLOCK
		fillRect(img, px, py, w, h/2, px, py, fg)
		return true
	case 0x2581: // ▁ LOWER ONE EIGHTH BLOCK
		fillRect(img, px, py+h*7/8, w, h-h*7/8, px, py, fg)
		return true
	case 0x2582: // ▂ LOWER ONE QUARTER BLOCK
		fillRect(img, px, py+h*3/4, w, h-h*3/4, px, py, fg)
		return true
	case 0x2583: // ▃ LOWER THREE EIGHTHS BLOCK
		fillRect(img, px, py+h*5/8, w, h-h*5/8, px, py, fg)
		return true
	case 0x2584: // ▄ LOWER HALF BLOCK
		fillRect(img, px, py+(h+1)/2, w, h-(h+1)/2, px, py, fg)
		return true
	case 0x2585: // ▅ LOWER FIVE EIGHTHS BLOCK
		fillRect(img, px, py+h*3/8, w, h-h*3/8, px, py, fg)
		return true
	case 0x2586: // ▆ LOWER THREE QUARTERS BLOCK
		fillRect(img, px, py+h/4, w, h-h/4, px, py, fg)
		return true
	case 0x2587: // ▇ LOWER SEVEN EIGHTHS BLOCK
		fillRect(img, px, py+h/8, w, h-h/8, px, py, fg)
		return true
	case 0x2588: // █ FULL BLOCK
		fillRect(img, px, py, w, h, px, py, fg)
		return true
	case 0x2589: // ▉ LEFT SEVEN EIGHTHS BLOCK
		fillRect(img, px, py, w*7/8, h, px, py, fg)
		return true
	case 0x258A: // ▊ LEFT THREE QUARTERS BLOCK
		fillRect(img, px, py, w*3/4, h, px, py, fg)
		return true
	case 0x258B: // ▋ LEFT FIVE EIGHTHS BLOCK
		fillRect(img, px, py, w*5/8, h, px, py, fg)
		return true
	case 0x258C: // ▌ LEFT HALF BLOCK
		fillRect(img, px, py, (w+1)/2, h, px, py, fg)
		return true
	case 0x258D: // ▍ LEFT THREE EIGHTHS BLOCK
		fillRect(img, px, py, w*3/8, h, px, py, fg)
		return true
	case 0x258E: // ▎ LEFT ONE QUARTER BLOCK
		fillRect(img, px, py, w/4, h, px, py, fg)
		return true
	case 0x258F: // ▏ LEFT ONE EIGHTH BLOCK
		fillRect(img, px, py, max1(w/8, 1), h, px, py, fg)
		return true
	case 0x2590: // ▐ RIGHT HALF BLOCK
		fillRect(img, px+w/2, py, w-w/2, h, px, py, fg)
		return true
	case 0x2591: // ░ LIGHT SHADE (~25%)
		for dy := py; dy < py+h; dy++ {
			for dx := px; dx < px+w; dx++ {
				if (dx%2 == 0) == (dy%2 == 0) {
					img.SetRGBA(dx, dy, fg)
				}
			}
		}
		return true
	case 0x2592: // ▒ MEDIUM SHADE (~50%)
		for dy := py; dy < py+h; dy++ {
			for dx := px; dx < px+w; dx++ {
				if (dx+dy)%2 == 0 {
					img.SetRGBA(dx, dy, fg)
				}
			}
		}
		return true
	case 0x2593: // ▓ DARK SHADE (~75%)
		for dy := py; dy < py+h; dy++ {
			for dx := px; dx < px+w; dx++ {
				if (dx%2 != 0) || (dy%2 != 0) {
					img.SetRGBA(dx, dy, fg)
				}
			}
		}
		return true
	case 0x2594: // ▔ UPPER ONE EIGHTH BLOCK
		fillRect(img, px, py, w, max1(h/8, 1), px, py, fg)
		return true
	case 0x2595: // ▕ RIGHT ONE EIGHTH BLOCK
		fillRect(img, px+w*7/8, py, w-w*7/8, h, px, py, fg)
		return true
	case 0x2596: // ▖ LOWER LEFT QUADRANT
		fillRect(img, px, py+(h+1)/2, (w+1)/2, h-(h+1)/2, px, py, fg)
		return true
	case 0x2597: // ▗ LOWER RIGHT QUADRANT
		fillRect(img, px+w/2, py+(h+1)/2, w-w/2, h-(h+1)/2, px, py, fg)
		return true
	case 0x2598: // ▘ UPPER LEFT QUADRANT
		fillRect(img, px, py, (w+1)/2, h/2, px, py, fg)
		return true
	case 0x2599: // ▙ UPPER LEFT AND LOWER HALF
		fillRect(img, px, py, (w+1)/2, h/2, px, py, fg)
		fillRect(img, px, py+(h+1)/2, w, h-(h+1)/2, px, py, fg)
		return true
	case 0x259A: // ▚ UPPER LEFT AND LOWER RIGHT QUADRANT
		fillRect(img, px, py, (w+1)/2, h/2, px, py, fg)
		fillRect(img, px+w/2, py+(h+1)/2, w-w/2, h-(h+1)/2, px, py, fg)
		return true
	case 0x259B: // ▛ UPPER HALF AND LOWER LEFT QUADRANT
		fillRect(img, px, py, w, h/2, px, py, fg)
		fillRect(img, px, py+(h+1)/2, (w+1)/2, h-(h+1)/2, px, py, fg)
		return true
	case 0x259C: // ▜ UPPER HALF AND LOWER RIGHT QUADRANT
		fillRect(img, px, py, w, h/2, px, py, fg)
		fillRect(img, px+w/2, py+(h+1)/2, w-w/2, h-(h+1)/2, px, py, fg)
		return true
	case 0x259D: // ▝ UPPER RIGHT QUADRANT
		fillRect(img, px+w/2, py, w-w/2, h/2, px, py, fg)
		return true
	case 0x259E: // ▞ UPPER RIGHT AND LOWER LEFT QUADRANT
		fillRect(img, px+w/2, py, w-w/2, h/2, px, py, fg)
		fillRect(img, px, py+(h+1)/2, (w+1)/2, h-(h+1)/2, px, py, fg)
		return true
	case 0x259F: // ▟ LOWER HALF AND UPPER RIGHT QUADRANT
		fillRect(img, px+w/2, py, w-w/2, h/2, px, py, fg)
		fillRect(img, px, py+(h+1)/2, w, h-(h+1)/2, px, py, fg)
		return true
	}

	// ── Box drawing U+2500–U+257F ─────────────────────────────────────────────
	if r >= 0x2500 && r <= 0x257F {
		// Classify into H / V / corner / T / cross groups.
		switch {
		case r == 0x2500 || r == 0x2501 || r == 0x254C || r == 0x254D: // ─ ━ ╌ ╍
			drawHLine(img, px, px+w-1, my, fg)
		case r == 0x2502 || r == 0x2503 || r == 0x254E || r == 0x254F: // │ ┃ ╎ ╏
			drawVLine(img, py, py+h-1, mx, fg)
		case r >= 0x250C && r <= 0x250F: // ┌ ┍ ┎ ┏ — top-left corner
			drawHLine(img, mx, px+w-1, my, fg)
			drawVLine(img, my, py+h-1, mx, fg)
		case r >= 0x2510 && r <= 0x2513: // ┐ ┑ ┒ ┓ — top-right corner
			drawHLine(img, px, mx, my, fg)
			drawVLine(img, my, py+h-1, mx, fg)
		case r >= 0x2514 && r <= 0x2517: // └ ┕ ┖ ┗ — bottom-left corner
			drawHLine(img, mx, px+w-1, my, fg)
			drawVLine(img, py, my, mx, fg)
		case r >= 0x2518 && r <= 0x251B: // ┘ ┙ ┚ ┛ — bottom-right corner
			drawHLine(img, px, mx, my, fg)
			drawVLine(img, py, my, mx, fg)
		case r >= 0x251C && r <= 0x2523: // ├ — left T
			drawHLine(img, mx, px+w-1, my, fg)
			drawVLine(img, py, py+h-1, mx, fg)
		case r >= 0x2524 && r <= 0x252B: // ┤ — right T
			drawHLine(img, px, mx, my, fg)
			drawVLine(img, py, py+h-1, mx, fg)
		case r >= 0x252C && r <= 0x2533: // ┬ — top T
			drawHLine(img, px, px+w-1, my, fg)
			drawVLine(img, my, py+h-1, mx, fg)
		case r >= 0x2534 && r <= 0x253B: // ┴ — bottom T
			drawHLine(img, px, px+w-1, my, fg)
			drawVLine(img, py, my, mx, fg)
		case r >= 0x253C && r <= 0x254B: // ┼ — cross
			drawHLine(img, px, px+w-1, my, fg)
			drawVLine(img, py, py+h-1, mx, fg)
		// Double-line variants U+2550–U+256C
		case r == 0x2550: // ═ DOUBLE HORIZONTAL
			drawHLine(img, px, px+w-1, my-1, fg)
			drawHLine(img, px, px+w-1, my+1, fg)
		case r == 0x2551: // ║ DOUBLE VERTICAL
			drawVLine(img, py, py+h-1, mx-1, fg)
			drawVLine(img, py, py+h-1, mx+1, fg)
		case r >= 0x2552 && r <= 0x2561: // mixed double/single corners & T pieces
			drawHLine(img, px, px+w-1, my, fg)
			drawVLine(img, py, py+h-1, mx, fg)
		case r >= 0x2562 && r <= 0x256C: // more double variants
			drawHLine(img, px, px+w-1, my, fg)
			drawVLine(img, py, py+h-1, mx, fg)
		// Diagonal U+2571–U+2573
		case r == 0x2571: // ╱
			for i := 0; i < h; i++ {
				x := px + (w-1)*(h-1-i)/(h-1)
				img.SetRGBA(x, py+i, fg)
			}
		case r == 0x2572: // ╲
			for i := 0; i < h; i++ {
				x := px + (w-1)*i/(h-1)
				img.SetRGBA(x, py+i, fg)
			}
		case r == 0x2573: // ╳
			for i := 0; i < h; i++ {
				img.SetRGBA(px+(w-1)*i/(h-1), py+i, fg)
				img.SetRGBA(px+(w-1)*(h-1-i)/(h-1), py+i, fg)
			}
		}
		return true
	}

	// ── Misc common TUI glyphs ────────────────────────────────────────────────
	switch r {
	case 0x2022, 0x25CF, 0x25C9, 0x2219: // ● • ◉ ∙ — bullet / circle
		// Filled circle approximation: 3×5 in a 7×13 cell.
		cx, cy := mx, my
		for dy := -2; dy <= 2; dy++ {
			for dx := -2; dx <= 2; dx++ {
				if dx*dx+dy*dy <= 5 {
					img.SetRGBA(cx+dx, cy+dy, fg)
				}
			}
		}
		return true
	case 0x25CB, 0x25CC: // ○ ◌ — open circle
		cx, cy := mx, my
		for dy := -2; dy <= 2; dy++ {
			for dx := -2; dx <= 2; dx++ {
				d := dx*dx + dy*dy
				if d >= 4 && d <= 6 {
					img.SetRGBA(cx+dx, cy+dy, fg)
				}
			}
		}
		return true
	case 0x2026: // … HORIZONTAL ELLIPSIS — three dots
		for _, dx := range []int{-2, 0, 2} {
			img.SetRGBA(mx+dx, my, fg)
			img.SetRGBA(mx+dx, my+1, fg)
		}
		return true
	case 0x00B7, 0x2027: // · ‧ middle dot
		img.SetRGBA(mx, my, fg)
		img.SetRGBA(mx+1, my, fg)
		return true
	case 0x25B6, 0x25BA: // ▶ ► right-pointing triangle
		for i := 0; i < w/2; i++ {
			drawVLine(img, my-i, my+i, px+i, fg)
		}
		return true
	case 0x25C0, 0x25C4: // ◀ ◄ left-pointing triangle
		for i := 0; i < w/2; i++ {
			drawVLine(img, my-i, my+i, px+w-1-i, fg)
		}
		return true
	}

	return false
}

func max1(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// render draws the cell grid onto a new RGBA image.
func render(grid [][]cell) *image.RGBA {
	cols := 0
	for _, row := range grid {
		if len(row) > cols {
			cols = len(row)
		}
	}
	rows := len(grid)
	if rows == 0 || cols == 0 {
		return image.NewRGBA(image.Rect(0, 0, 1, 1))
	}

	w := cols*charW + 2*padX
	h := rows*charH + 2*padY
	img := image.NewRGBA(image.Rect(0, 0, w, h))

	// Fill default background.
	draw.Draw(img, img.Bounds(), &image.Uniform{defaultBg}, image.Point{}, draw.Src)

	face := basicfont.Face7x13

	for y, row := range grid {
		for x, c := range row {
			px := padX + x*charW
			py := padY + y*charH

			// Non-default cell background.
			if c.bg != defaultBg {
				bgRect := image.Rect(px, py, px+charW, py+charH)
				draw.Draw(img, bgRect, &image.Uniform{c.bg}, image.Point{}, draw.Src)
			}

			if c.ch == ' ' || c.ch == 0 || c.ch == utf8.RuneError {
				continue
			}

			// Geometric rendering for block/box/special Unicode chars.
			if drawSpecialChar(img, c.ch, c.fg, px, py) {
				continue
			}

			// ASCII fallback via basicfont.
			if c.ch < 0x20 || c.ch > 0x7e {
				continue // outside basicfont range — skip rather than render garbage
			}

			d := &font.Drawer{
				Dst:  img,
				Src:  &image.Uniform{c.fg},
				Face: face,
				Dot:  fixed.P(px, py+charH-2),
			}
			d.DrawString(string(c.ch))
		}
	}
	return img
}

func main() {
	outPath := flag.String("o", "out.png", "output PNG file path")
	flag.Parse()

	grid := parse(os.Stdin)
	img := render(grid)

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
