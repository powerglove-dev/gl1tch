package console

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// termSplit describes one pane to open from a /terminal request.
type termSplit struct {
	pct      int    // percentage of the current pane to use (1-99)
	vertical bool   // true → split-window -v (bottom), false → -h (right)
	left     bool   // true → prepend -b (left of current pane)
	cwd      string // working directory; passed as -c; empty = inherit
}

// termSplitArgs converts a termSplit into the tmux split-window argument list.
func (s termSplit) tmuxArgs() []string {
	args := []string{"split-window"}
	if s.vertical {
		args = append(args, "-v")
	} else {
		args = append(args, "-h")
	}
	if s.left {
		args = append(args, "-b")
	}
	args = append(args, "-p", strconv.Itoa(s.pct))
	if s.cwd != "" {
		args = append(args, "-c", s.cwd)
	}
	return args
}

// termSplitsDesc returns the confirmation message shown in the gl1tch chat.
func termSplitsDesc(splits []termSplit) string {
	if len(splits) == 1 {
		return termSplitDesc(splits[0])
	}
	// multiple panes
	cwds := make([]string, 0, len(splits))
	for _, s := range splits {
		if s.cwd != "" {
			cwds = append(cwds, s.cwd)
		}
	}
	pct := splits[0].pct
	var parts []string
	if splits[0].vertical {
		parts = append(parts, "bottom")
	} else if splits[0].left {
		parts = append(parts, "left")
	}
	if pct != 25 {
		parts = append(parts, fmt.Sprintf("%d%%", pct))
	}
	var prefix string
	if len(parts) > 0 {
		prefix = strings.Join(parts, " ") + " "
	}
	if len(cwds) == len(splits) {
		return fmt.Sprintf("opening %d %sterminals: %s", len(splits), prefix, strings.Join(cwds, ", "))
	}
	return fmt.Sprintf("opening %d %sterminals.", len(splits), prefix)
}

func termSplitDesc(s termSplit) string {
	var parts []string
	if s.vertical {
		parts = append(parts, "bottom")
	} else if s.left {
		parts = append(parts, "left")
	}
	if s.pct != 25 {
		parts = append(parts, fmt.Sprintf("%d%%", s.pct))
	}
	var prefix string
	if len(parts) > 0 {
		prefix = strings.Join(parts, " ") + " "
	}
	if s.cwd != "" {
		return fmt.Sprintf("opening %sterminal in %s.", prefix, s.cwd)
	}
	return fmt.Sprintf("opening %sterminal split.", prefix)
}

// parseTerminalNL parses natural-language text following "/terminal " into one
// or more termSplit descriptors. Returns (splits, true) when NL patterns are
// detected, (nil, false) when the text should be treated as a raw shell command.
//
// Recognised patterns (all case-insensitive, order independent):
//
//	size   — "50%", "50 percent", "half", "third", "quarter", "full"
//	count  — "3 shells", "3 terminals", "3 panes"
//	dir    — "bottom", "vertical", "below"   → vertical split
//	        "left"                            → left split
//	cwd    — "cwd to PATH1 PATH2 PATH3"
//	        "in PATH" (single)
//	open   — "open a terminal", "a shell", "terminal", etc. → bare split
func parseTerminalNL(text string) ([]termSplit, bool) {
	lower := strings.ToLower(strings.TrimSpace(text))

	pct := 25
	vertical := false
	left := false
	count := 1
	var cwds []string
	anyNL := false

	// ── size ──────────────────────────────────────────────────────────────────
	if m := regexp.MustCompile(`(\d+)\s*%`).FindStringSubmatch(lower); m != nil {
		if n, err := strconv.Atoi(m[1]); err == nil && n > 0 && n < 100 {
			pct = n
			anyNL = true
		}
	} else if strings.Contains(lower, "half") {
		pct = 50
		anyNL = true
	} else if strings.Contains(lower, "third") {
		pct = 33
		anyNL = true
	} else if strings.Contains(lower, "quarter") {
		pct = 25 // already default; still marks as NL so "open a quarter-width terminal" works
		anyNL = true
	} else if strings.Contains(lower, "full") {
		pct = 90
		anyNL = true
	}

	// ── direction ─────────────────────────────────────────────────────────────
	if regexp.MustCompile(`\b(bottom|vertical|below)\b`).MatchString(lower) {
		vertical = true
		anyNL = true
	}
	if regexp.MustCompile(`\bleft\b`).MatchString(lower) {
		left = true
		anyNL = true
	}

	// ── count ─────────────────────────────────────────────────────────────────
	if m := regexp.MustCompile(`(\d+)\s+(?:shell|terminal|pane)s?`).FindStringSubmatch(lower); m != nil {
		if n, err := strconv.Atoi(m[1]); err == nil && n > 0 && n <= 10 {
			count = n
			anyNL = true
		}
	}

	// ── cwds — "cwd to X Y Z", "in DIR" ──────────────────────────────────────
	// Use FindStringSubmatchIndex so we can slice from the ORIGINAL text and
	// preserve the case of path components.
	if m := regexp.MustCompile(`(?i)(?:cwd|directory|dir)\s+(?:to\s+)?(\S.*)`).FindStringSubmatchIndex(text); m != nil {
		tail := text[m[2]:m[3]]
		// Split on whitespace and "and".
		raw := regexp.MustCompile(`(?i)\s+and\s+|\s+`).Split(strings.TrimSpace(tail), -1)
		for _, p := range raw {
			if p = strings.TrimSpace(p); p != "" {
				cwds = append(cwds, p)
			}
		}
		if len(cwds) > 0 {
			anyNL = true
			if len(cwds) > count {
				count = len(cwds)
			}
		}
	} else if m := regexp.MustCompile(`(?i)\bin\s+([\./~]\S+)`).FindStringSubmatchIndex(text); m != nil {
		// "in ./path" or "in /abs/path" — single cwd shorthand; preserve original case.
		cwds = append(cwds, text[m[2]:m[3]])
		anyNL = true
	}

	// ── "open a terminal" / "a shell" / bare subject nouns ───────────────────
	// If text is purely a generic open-request phrase with no other content,
	// treat it as a bare split regardless.
	genericOpen := regexp.MustCompile(
		`^(?:open|create|start|launch|spawn|new|give me)?\s*` +
			`(?:a|an|me\s+a|me\s+an)?\s*(?:new\s+)?` +
			`(?:terminal|shell|pane)s?\s*$`,
	).MatchString(strings.TrimSpace(text))
	if genericOpen {
		anyNL = true
	}

	if !anyNL {
		return nil, false
	}

	splits := make([]termSplit, count)
	for i := range splits {
		splits[i] = termSplit{pct: pct, vertical: vertical, left: left}
		if i < len(cwds) {
			splits[i].cwd = cwds[i]
		}
	}
	return splits, true
}
