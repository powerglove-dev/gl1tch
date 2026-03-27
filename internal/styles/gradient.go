package styles

import (
	"fmt"
	"math"
	"os"
	"strings"
)

// InterpolateRGB interpolates linearly between two hex colors over `steps` steps.
// Returns a slice of `steps` hex color strings like "#rrggbb".
// When steps <= 0, returns nil. When steps == 1, returns a slice with just `from`.
func InterpolateRGB(from, to string, steps int) []string {
	if steps <= 0 {
		return nil
	}
	fr, fg, fb := hexToRGB(from)
	tr, tg, tb := hexToRGB(to)

	out := make([]string, steps)
	for i := 0; i < steps; i++ {
		var t float64
		if steps == 1 {
			t = 0
		} else {
			t = float64(i) / float64(steps-1)
		}
		r := uint8(math.Round(float64(fr) + t*float64(int(tr)-int(fr))))
		g := uint8(math.Round(float64(fg) + t*float64(int(tg)-int(fg))))
		b := uint8(math.Round(float64(fb) + t*float64(int(tb)-int(fb))))
		out[i] = fmt.Sprintf("#%02x%02x%02x", r, g, b)
	}
	return out
}

// GradientStops expands 2–4 color stops into a slice of `width` hex colors,
// one per character position.
func GradientStops(stops []string, width int) []string {
	if len(stops) == 0 || width <= 0 {
		return nil
	}
	if len(stops) == 1 {
		out := make([]string, width)
		for i := range out {
			out[i] = stops[0]
		}
		return out
	}

	numSegments := len(stops) - 1
	out := make([]string, 0, width)

	for seg := 0; seg < numSegments; seg++ {
		// Compute how many colors belong to this segment.
		// Distribute width as evenly as possible across segments, giving
		// extra columns to earlier segments when not evenly divisible.
		segStart := seg * width / numSegments
		segEnd := (seg + 1) * width / numSegments
		segLen := segEnd - segStart
		if segLen <= 0 {
			continue
		}
		// Don't include the last color of each segment (except the final segment)
		// to avoid repeating the shared stop color.
		if seg < numSegments-1 {
			// interpolate segLen steps from stops[seg] to stops[seg+1],
			// but only take the first segLen (exclude the endpoint).
			colors := InterpolateRGB(stops[seg], stops[seg+1], segLen+1)
			out = append(out, colors[:segLen]...)
		} else {
			// last segment: include both endpoints
			colors := InterpolateRGB(stops[seg], stops[seg+1], segLen)
			out = append(out, colors...)
		}
	}

	// Ensure exact length (handle any off-by-one from integer division).
	for len(out) < width {
		out = append(out, stops[len(stops)-1])
	}
	if len(out) > width {
		out = out[:width]
	}

	return out
}

// HexToFGEsc returns the ANSI foreground escape for a hex color.
// Uses 24-bit true-color unless TERM starts with "screen" (tmux), in which
// case it falls back to 256-color approximation.
func HexToFGEsc(hex string) string {
	if isTmux() {
		return hexToFG256(hex)
	}
	return hexToFG24(hex)
}

// hexToFGEsc is the unexported alias for internal use within the package.
func hexToFGEsc(hex string) string {
	return HexToFGEsc(hex)
}

// isTmux returns true when the terminal is running inside tmux.
func isTmux() bool {
	t := os.Getenv("TERM")
	return strings.HasPrefix(t, "screen") || os.Getenv("TERM_PROGRAM") == "tmux"
}

func hexToFG24(hex string) string {
	r, g, b := hexToRGB(hex)
	return fmt.Sprintf("\x1b[38;2;%d;%d;%dm", r, g, b)
}

func hexToFG256(hex string) string {
	r, g, b := hexToRGB(hex)
	n := rgbTo256(r, g, b)
	return fmt.Sprintf("\x1b[38;5;%dm", n)
}

func rgbTo256(r, g, b uint8) int {
	// Map to the 6x6x6 color cube (indices 16–231).
	ri := int(math.Round(float64(r) / 255.0 * 5))
	gi := int(math.Round(float64(g) / 255.0 * 5))
	bi := int(math.Round(float64(b) / 255.0 * 5))
	return 16 + 36*ri + 6*gi + bi
}

// RenderGradientBorder wraps content with a gradient border using double-line
// box-drawing characters. The gradient runs left→right on horizontal edges and
// top→bottom on vertical edges (reversed on the right side).
//
//	╔════════════════════╗   ← top border, gradient left→right
//	║  content line 1   ║   ← sides gradient top→bottom
//	║  content line 2   ║
//	╚════════════════════╝   ← bottom border, gradient left→right (reversed)
//
// stops: hex color stops (1–4). w/h are the inner content dimensions.
// Each line ends with \x1b[0m to prevent color bleed.
func RenderGradientBorder(content string, stops []string, w, h int) string {
	if len(stops) == 0 || w <= 0 {
		return content
	}

	reset := "\x1b[0m"
	// Total border width = inner width + 2 (for left and right border chars)
	totalW := w + 2

	// Generate horizontal gradient (left→right) across totalW positions.
	hColors := GradientStops(stops, totalW)

	// Generate vertical gradient (top→bottom) across (h + 2) positions (top row + content rows + bottom row).
	totalH := h + 2
	vColors := GradientStops(stops, totalH)

	// Helper: get color escape for a horizontal position.
	hEsc := func(pos int) string {
		if pos < 0 || pos >= len(hColors) {
			return hexToFGEsc(stops[0])
		}
		return hexToFGEsc(hColors[pos])
	}

	// Helper: get color escape for a vertical position.
	vEsc := func(pos int) string {
		if pos < 0 || pos >= len(vColors) {
			return hexToFGEsc(stops[0])
		}
		return hexToFGEsc(vColors[pos])
	}

	// Reversed horizontal gradient for bottom border.
	reversedStops := make([]string, len(stops))
	for i, s := range stops {
		reversedStops[len(stops)-1-i] = s
	}
	hColorsRev := GradientStops(reversedStops, totalW)
	hEscRev := func(pos int) string {
		if pos < 0 || pos >= len(hColorsRev) {
			return hexToFGEsc(stops[len(stops)-1])
		}
		return hexToFGEsc(hColorsRev[pos])
	}

	var sb strings.Builder

	// Top border: ╔ + (w × ═) + ╗
	topLine := strings.Builder{}
	topLine.WriteString(hEsc(0))
	topLine.WriteRune('╔')
	for i := 1; i <= w; i++ {
		topLine.WriteString(hEsc(i))
		topLine.WriteRune('═')
	}
	topLine.WriteString(hEsc(totalW - 1))
	topLine.WriteRune('╗')
	topLine.WriteString(reset)
	sb.WriteString(topLine.String())
	sb.WriteByte('\n')

	// Content lines with side borders.
	contentLines := strings.Split(content, "\n")
	for row := 0; row < h; row++ {
		vRow := row + 1 // offset by 1 because row 0 is the top border
		var line string
		if row < len(contentLines) {
			line = contentLines[row]
		}

		// Left border ║
		sb.WriteString(vEsc(vRow))
		sb.WriteRune('║')

		// Content
		sb.WriteString(reset)
		sb.WriteString(line)

		// Right border ║ (use reversed vertical gradient for symmetry)
		sb.WriteString(vEsc(totalH - 1 - vRow))
		sb.WriteRune('║')
		sb.WriteString(reset)
		sb.WriteByte('\n')
	}

	// Bottom border: ╚ + (w × ═) + ╝ (reversed gradient)
	botLine := strings.Builder{}
	botLine.WriteString(hEscRev(0))
	botLine.WriteRune('╚')
	for i := 1; i <= w; i++ {
		botLine.WriteString(hEscRev(i))
		botLine.WriteRune('═')
	}
	botLine.WriteString(hEscRev(totalW - 1))
	botLine.WriteRune('╝')
	botLine.WriteString(reset)
	sb.WriteString(botLine.String())

	return sb.String()
}
