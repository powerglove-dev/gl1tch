package switchboard

import (
	"bytes"
	"strings"

	"github.com/adam-stokes/orcai/internal/themes"
)

// panelTitles maps panel keys to their plain-text fallback titles.
var panelTitles = map[string]string{
	"pipelines":     "PIPELINES",
	"agent_runner":  "AGENT RUNNER",
	"signal_board":  "SIGNAL BOARD",
	"activity_feed": "ACTIVITY FEED",
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

// RenderHeader returns the header string for a panel.
// If the bundle has ANS bytes for this panel and the terminal is wide enough,
// the sprite is returned (with a trailing reset). Otherwise the plain-text
// title is returned.
func RenderHeader(bundle *themes.Bundle, panel string, termWidth int) string {
	if bundle != nil && bundle.HeaderBytes != nil {
		if ans, ok := bundle.HeaderBytes[panel]; ok && len(ans) > 0 {
			sw := spriteWidth(ans)
			if termWidth == 0 || termWidth >= sw {
				return strings.TrimRight(string(ans), "\n") + "\x1b[0m"
			}
		}
	}
	if title, ok := panelTitles[panel]; ok {
		return title
	}
	return strings.ToUpper(panel)
}
