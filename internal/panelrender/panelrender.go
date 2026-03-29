// Package panelrender provides shared ANSI box-drawing and panel-header
// rendering utilities used by both switchboard and standalone sub-TUIs
// (crontui, jumpwindow, etc.).
package panelrender

import (
	"bytes"
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"
	"github.com/muesli/ansi"
	"github.com/muesli/reflow/truncate"

	"github.com/adam-stokes/orcai/internal/styles"
	"github.com/adam-stokes/orcai/internal/themes"
	"github.com/adam-stokes/orcai/internal/translations"
)

// RST is the ANSI reset escape sequence.
const RST = "\x1b[0m"

// BLD is the ANSI bold escape sequence.
const BLD = "\x1b[1m"

// panelTitles maps panel keys to their plain-text fallback titles.
var panelTitles = map[string]string{
	"pipelines":     "PIPELINES",
	"agent_runner":  "AGENT RUNNER",
	"signal_board":  "SIGNAL BOARD",
	"activity_feed": "ACTIVITY FEED",
	"inbox":         "INBOX",
	"cron":          "CRON JOBS",
}

// RenderHeader returns the translated (or plain-text fallback) title for a panel.
// Used in BoxTop and DynamicHeader when no ANS sprite is available.
func RenderHeader(panel string) string {
	title := panelTitles[panel]
	if title == "" {
		title = strings.ToUpper(panel)
	}
	if p := translations.GlobalProvider(); p != nil {
		key := panel + "_panel_title"
		return p.T(key, title)
	}
	return title
}

// ── Box drawing ───────────────────────────────────────────────────────────────

// BoxTop renders the top border of a box. If title is non-empty it is centered
// in the border using borderColor and labelColor.
func BoxTop(w int, title, borderColor, labelColor string) string {
	if title == "" {
		return borderColor + "┌" + strings.Repeat("─", pmax(w-2, 0)) + "┐" + RST
	}
	label := " " + title + " "
	dashes := pmax(w-2-lipgloss.Width(label), 0)
	left := dashes / 2
	right := dashes - left
	return borderColor + "┌" + strings.Repeat("─", left) + labelColor + label + borderColor + strings.Repeat("─", right) + "┐" + RST
}

// BoxBot renders the bottom border of a box.
func BoxBot(w int, borderColor string) string {
	return borderColor + "└" + strings.Repeat("─", pmax(w-2, 0)) + "┘" + RST
}

// BoxRow renders a single content row padded to fill the inner box width.
func BoxRow(content string, w int, borderColor string) string {
	inner := w - 2
	pad := pmax(inner-lipgloss.Width(content), 0)
	return borderColor + "│" + RST + content + strings.Repeat(" ", pad) + borderColor + "│" + RST
}

// ── Panel headers ─────────────────────────────────────────────────────────────

// SpriteLines returns the ANS sprite for a panel as individual lines, ready to
// be prepended in place of a BoxTop call.
//
// Bundle.HeaderBytes[panel] is an ordered slice of sprite variants (widest
// first). SpriteLines tries each variant in order and returns the first whose
// visible width fits panelWidth. When panelWidth is 0 the width check is
// skipped and the first variant is used.
//
// Returns nil when the bundle has no sprites for this panel or none fit.
func SpriteLines(bundle *themes.Bundle, panel string, panelWidth int) []string {
	if bundle == nil || bundle.HeaderBytes == nil {
		return nil
	}
	variants, ok := bundle.HeaderBytes[panel]
	if !ok || len(variants) == 0 {
		return nil
	}
	for _, ans := range variants {
		if len(ans) == 0 {
			continue
		}
		if panelWidth > 0 && spriteWidth(ans) > panelWidth {
			continue // too wide — try next variant
		}
		var lines []string
		for _, raw := range bytes.Split(ans, []byte("\n")) {
			s := strings.TrimRight(string(raw), "\r")
			if visibleWidth(s) > 0 {
				lines = append(lines, s)
			}
		}
		if len(lines) == 0 {
			continue
		}
		lines[len(lines)-1] += RST
		return lines
	}
	return nil
}

// DynamicHeader generates a single-line panel header at exactly width visible
// columns in the style:  ----| PANEL TITLE |----
// Uses accent color for the dashes and brackets, text color for the title.
//
// Returns nil when bundle is nil or bundle.HeaderStyle has no entry for panel.
func DynamicHeader(bundle *themes.Bundle, panel string, width int, _ string) []string {
	if bundle == nil || width < 4 {
		return nil
	}
	hs := bundle.HeaderStyle
	ps, ok := hs.Panels[panel]
	if !ok {
		return nil
	}
	accentSeq := hexToFGSeq(bundle.ResolveRef(ps.Accent))
	textSeq := hexToFGSeq(bundle.ResolveRef(ps.Text))

	title := RenderHeader(panel)
	inner := "┌┤ " + title + " ├┐"
	dashes := pmax(width-lipgloss.Width(inner), 0)
	left := dashes / 2
	right := dashes - left
	line := accentSeq + "┌" + strings.Repeat("─", left) + "┤ " + textSeq + BLD + title + RST + accentSeq + " ├" + strings.Repeat("─", right) + "┐" + RST
	return []string{line}
}

// PanelHeader returns the best available header for a panel at the given width.
// It tries fixed-width .ans sprites first (SpriteLines), then falls back to
// DynamicHeader which always produces the correct panel width.
// Returns nil only when both sources are unavailable.
func PanelHeader(bundle *themes.Bundle, panel string, width int, borderColor string) []string {
	if lines := SpriteLines(bundle, panel, width); lines != nil {
		return lines
	}
	return DynamicHeader(bundle, panel, width, borderColor)
}

// ── Private helpers ───────────────────────────────────────────────────────────

func hexToFGSeq(hex string) string {
	hex = strings.TrimPrefix(hex, "#")
	if len(hex) != 6 {
		return "\x1b[38;2;189;147;249m" // Dracula purple fallback
	}
	parse := func(s string) uint8 {
		v, _ := strconv.ParseUint(s, 16, 8)
		return uint8(v)
	}
	r, g, b := parse(hex[0:2]), parse(hex[2:4]), parse(hex[4:6])
	return fmt.Sprintf("\x1b[38;2;%d;%d;%dm", r, g, b)
}


func spriteWidth(ans []byte) int {
	maxW := 0
	for _, line := range bytes.Split(ans, []byte("\n")) {
		w := visibleWidth(string(line))
		if w > maxW {
			maxW = w
		}
	}
	return maxW
}

// VisibleWidth returns the printable character count of s, stripping ANSI escapes.
func VisibleWidth(s string) int { return visibleWidth(s) }

func visibleWidth(s string) int {
	inEsc := false
	w := 0
	i := 0
	for i < len(s) {
		b := s[i]
		if b == 0x1b && i+1 < len(s) && s[i+1] == '[' {
			inEsc = true
			i += 2
			continue
		}
		if inEsc {
			if (b >= 'A' && b <= 'Z') || (b >= 'a' && b <= 'z') {
				inEsc = false
			}
			i++
			continue
		}
		_, size := decodeRuneAt(s, i)
		w++
		i += size
	}
	return w
}

func decodeRuneAt(s string, i int) (rune, int) {
	end := i + 4
	if end > len(s) {
		end = len(s)
	}
	runes := []rune(s[i:end])
	if len(runes) == 0 {
		return 0, 1
	}
	return runes[0], len(string(runes[0]))
}


func pmax(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// ── Hint bar ──────────────────────────────────────────────────────────────────

// Hint is a key/description pair for a panel action-hint footer.
type Hint struct {
	Key  string
	Desc string
}

// HintBar formats a row of key/description hint pairs for display inside a
// panel's border box. Returns "" when hints is nil or empty — callers should
// skip the footer row entirely in that case.
//
// width is the maximum visible width (typically panelWidth-2 for the inner box
// area); content wider than width is truncated.
//
// Typical usage: BoxRow(HintBar(hints, w-2, pal), w, borderColor)
func HintBar(hints []Hint, width int, pal styles.ANSIPalette) string {
	if len(hints) == 0 {
		return ""
	}
	sep := pal.Dim + " · " + RST
	parts := make([]string, len(hints))
	for i, h := range hints {
		parts[i] = pal.Accent + h.Key + pal.Dim + " " + h.Desc + RST
	}
	bar := "  " + strings.Join(parts, sep)
	if width > 0 && lipgloss.Width(bar) > width {
		bar = truncate.String(bar, uint(width)) + RST
	}
	return bar
}

// QuitConfirmBox renders a reusable ANSI box-style quit confirmation modal.
// title is the box header, message is the body line (e.g. "Quit ORCAI?" or
// a running-jobs warning). An empty message defaults to "Are you sure?".
func QuitConfirmBox(pal styles.ANSIPalette, title, message string, screenW int) string {
	boxW := 52
	if screenW > 0 && screenW < boxW+4 {
		boxW = screenW - 4
	}
	if message == "" {
		message = "Are you sure?"
	}
	hints := HintBar([]Hint{
		{Key: "y", Desc: "confirm quit"},
		{Key: "n / esc", Desc: "cancel"},
	}, boxW-2, pal)
	rows := []string{
		BoxTop(boxW, title, pal.Border, pal.Accent),
		BoxRow("", boxW, pal.Border),
		BoxRow(pal.FG+"  "+message+RST, boxW, pal.Border),
		BoxRow("", boxW, pal.Border),
		BoxRow(hints, boxW, pal.Border),
		BoxBot(boxW, pal.Border),
	}
	return strings.Join(rows, "\n")
}

// OverlayCenter draws overlayContent centered over base, splicing each overlay
// line into the corresponding base line so the background content remains
// visible on either side of the floating box.
func OverlayCenter(base, overlay string, w, h int) string {
	baseLines := strings.Split(base, "\n")
	overlayLines := strings.Split(overlay, "\n")

	popW := 0
	for _, l := range overlayLines {
		if vl := lipgloss.Width(l); vl > popW {
			popW = vl
		}
	}
	popH := len(overlayLines)
	startRow := pmax((h-popH)/2, 0)
	startCol := pmax((w-popW)/2, 0)

	for i, oLine := range overlayLines {
		row := startRow + i
		for len(baseLines) <= row {
			baseLines = append(baseLines, "")
		}
		left := ansiTruncAt(baseLines[row], startCol)
		right := ansiFromCol(baseLines[row], startCol+popW)
		baseLines[row] = left + oLine + right
	}
	return strings.Join(baseLines, "\n")
}

// ansiTruncAt truncates s at visible column n, respecting ANSI escape sequences.
func ansiTruncAt(s string, n int) string {
	if n <= 0 {
		return ""
	}
	return truncate.String(s, uint(n))
}

// ansiFromCol returns the portion of s starting at visible column n.
func ansiFromCol(s string, n int) string {
	if n <= 0 {
		return s
	}
	vis := 0
	i := 0
	inEsc := false
	for i < len(s) {
		r, size := utf8.DecodeRuneInString(s[i:])
		if r == ansi.Marker {
			inEsc = true
			i += size
			continue
		}
		if inEsc {
			if ansi.IsTerminator(r) {
				inEsc = false
			}
			i += size
			continue
		}
		if vis >= n {
			return "\x1b[0m" + s[i:]
		}
		vis += runewidth.RuneWidth(r)
		i += size
	}
	return ""
}
