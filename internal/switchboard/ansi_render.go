package switchboard

import (
	"bytes"
	"fmt"
	"strconv"
	"strings"

	"github.com/adam-stokes/orcai/internal/themes"
	"github.com/adam-stokes/orcai/internal/translations"
)

// panelTitles maps panel keys to their plain-text fallback titles.
var panelTitles = map[string]string{
	"pipelines":     "PIPELINES",
	"agent_runner":  "AGENT RUNNER",
	"signal_board":  "SIGNAL BOARD",
	"activity_feed": "ACTIVITY FEED",
	"inbox":         "INBOX",
	"cron":          "CRON JOBS",
}

// spriteWidth returns the visual width of the widest line in ans bytes,
// ignoring ANSI escape sequences.
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

// visibleWidth returns the printable character count of s, stripping ANSI escapes.
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
		// Decode UTF-8 rune and count it as one visible column.
		_, size := decodeRuneAt(s, i)
		w++
		i += size
	}
	return w
}

// decodeRuneAt decodes the UTF-8 rune starting at s[i], returning the rune
// and its byte length.
func decodeRuneAt(s string, i int) (rune, int) {
	runes := []rune(s[i:minInt(i+4, len(s))])
	if len(runes) == 0 {
		return 0, 1
	}
	return runes[0], len(string(runes[0]))
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// RenderHeader returns the translated (or plain-text fallback) title for a panel.
// Used in boxTop() and DynamicHeader() when no ANS sprite is available.
// If a GlobalProvider is set, the panel's translation key is looked up first.
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

// SpriteLines returns the ANS sprite for a panel as individual lines, ready
// to be prepended in place of a boxTop() call.
//
// Bundle.HeaderBytes[panel] is an ordered slice of sprite variants (widest
// first). SpriteLines tries each variant in order and returns the first whose
// visible width fits panelWidth. When panelWidth is 0 the width check is
// skipped and the first variant is used.
//
// Returns nil when the bundle has no sprites for this panel or none fit.
// The last returned line has "\x1b[0m" appended to prevent color bleed.
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
		// Split into non-empty lines.
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
		lines[len(lines)-1] += "\x1b[0m"
		return lines
	}
	return nil
}

// hexToFGSeq converts "#rrggbb" to a 24-bit ANSI foreground sequence.
// Falls back to Dracula purple if the value is not a valid hex color.
func hexToFGSeq(hex string) string {
	hex = strings.TrimPrefix(hex, "#")
	if len(hex) != 6 {
		return "\x1b[38;2;189;147;249m" // Dracula purple
	}
	parse := func(s string) uint8 {
		v, _ := strconv.ParseUint(s, 16, 8)
		return uint8(v)
	}
	r, g, b := parse(hex[0:2]), parse(hex[2:4]), parse(hex[4:6])
	return fmt.Sprintf("\x1b[38;2;%d;%d;%dm", r, g, b)
}

// hexToBGSeq converts "#rrggbb" to a 24-bit ANSI background sequence.
func hexToBGSeq(hex string) string {
	hex = strings.TrimPrefix(hex, "#")
	if len(hex) != 6 {
		return "\x1b[48;2;189;147;249m" // Dracula purple fallback
	}
	parse := func(s string) uint8 {
		v, _ := strconv.ParseUint(s, 16, 8)
		return uint8(v)
	}
	r, g, b := parse(hex[0:2]), parse(hex[2:4]), parse(hex[4:6])
	return fmt.Sprintf("\x1b[48;2;%d;%d;%dm", r, g, b)
}

// DynamicHeader generates a 3-line panel header at exactly width visible
// columns, using colors from bundle.HeaderStyle. The header always fills the
// full panel width regardless of terminal size.
//
// Returns nil when bundle is nil or bundle.HeaderStyle has no entry for panel.
func DynamicHeader(bundle *themes.Bundle, panel string, width int) []string {
	if bundle == nil || width < 4 {
		return nil
	}
	hs := bundle.HeaderStyle
	ps, ok := hs.Panels[panel]
	if !ok {
		return nil
	}

	topChar := "▄"
	if hs.TopChar != "" {
		topChar = hs.TopChar
	}
	botChar := "▀"
	if hs.BotChar != "" {
		botChar = hs.BotChar
	}
	accentSeq := hexToFGSeq(bundle.ResolveRef(ps.Accent))
	accentBGSeq := hexToBGSeq(bundle.ResolveRef(ps.Accent))
	textSeq := hexToFGSeq(bundle.ResolveRef(ps.Text))
	rst := "\x1b[0m"
	bold := "\x1b[1m"

	// Top bar: full-width block chars starting at column 0 (matches boxRow │ position).
	topLine := accentSeq + bold + strings.Repeat(topChar, width) + rst

	// Title line: accent-colored background spanning the full width so the
	// panel title (in ps.Text color) is always legible regardless of terminal bg.
	title := RenderHeader(panel)
	titleRunes := []rune(title)
	pad := width - len(titleRunes)
	if pad < 0 {
		pad = 0
		titleRunes = titleRunes[:width]
	}
	lp := pad / 2
	rp := pad - lp
	titleLine := accentBGSeq + textSeq + bold +
		strings.Repeat(" ", lp) + string(titleRunes) + strings.Repeat(" ", rp) +
		rst

	// Bottom bar: full-width block chars.
	botLine := accentSeq + bold + strings.Repeat(botChar, width) + rst

	return []string{topLine, titleLine, botLine}
}

// PanelHeader returns the best available header for a panel at the given width.
// It tries fixed-width .ans sprites first (SpriteLines), then falls back to
// DynamicHeader which always produces the correct panel width.
// Returns nil only when both sources are unavailable.
func PanelHeader(bundle *themes.Bundle, panel string, width int) []string {
	if lines := SpriteLines(bundle, panel, width); lines != nil {
		return lines
	}
	return DynamicHeader(bundle, panel, width)
}
