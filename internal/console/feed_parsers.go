package console

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/8op-org/gl1tch/internal/styles"
)

// FeedLineParser processes a raw output line and returns zero or more display lines.
// If matched is false the line is passed to the next parser (or rendered as plain text).
// raw is already ANSI-stripped; expanded controls whether multi-line output is shown.
// When expanded is true the first returned line is the header and subsequent lines are
// the expanded content (appended visually without advancing the logical cursor index).
type FeedLineParser func(raw string, width int, pal styles.ANSIPalette, expanded bool) (lines []string, matched bool)

// feedParsers is the ordered registry of active parsers. First match wins.
var feedParsers = []FeedLineParser{
	jsonFeedLineParser,
}

// runFeedLineParsers runs raw through the registered parsers in order.
// Returns the display lines from the first match, or (nil, false) if none matched.
func runFeedLineParsers(raw string, width int, pal styles.ANSIPalette, expanded bool) ([]string, bool) {
	for _, p := range feedParsers {
		if lines, ok := p(raw, width, pal, expanded); ok {
			return lines, true
		}
	}
	return nil, false
}

// jsonIndicator is the prefix used in feedRawLines to tag JSON summary lines.
// The enter key handler checks for this prefix to identify expandable lines.
const jsonIndicator = "▸"

// jsonFeedLineParser detects lines that are valid JSON objects or arrays.
// Collapsed (expanded=false): returns a single summary row with ▸ indicator.
// Expanded (expanded=true): returns a header row (▾) followed by pretty-print lines.
func jsonFeedLineParser(raw string, width int, pal styles.ANSIPalette, expanded bool) ([]string, bool) {
	trimmed := strings.TrimSpace(raw)
	if len(trimmed) == 0 {
		return nil, false
	}
	first := trimmed[0]
	if first != '{' && first != '[' {
		return nil, false
	}
	if !json.Valid([]byte(trimmed)) {
		return nil, false
	}

	var collapsedSummary string
	indicator := jsonIndicator
	if expanded {
		indicator = "▾"
	}
	if first == '{' {
		var obj map[string]json.RawMessage
		if err := json.Unmarshal([]byte(trimmed), &obj); err == nil {
			collapsedSummary = fmt.Sprintf("%s%s%s %s{ … } (%d keys)%s", pal.Accent, indicator, aRst, pal.Dim, len(obj), aRst)
		} else {
			collapsedSummary = fmt.Sprintf("%s%s%s %s{ … }%s", pal.Accent, indicator, aRst, pal.Dim, aRst)
		}
	} else {
		var arr []json.RawMessage
		if err := json.Unmarshal([]byte(trimmed), &arr); err == nil {
			collapsedSummary = fmt.Sprintf("%s%s%s %s[ %d items ]%s", pal.Accent, indicator, aRst, pal.Dim, len(arr), aRst)
		} else {
			collapsedSummary = fmt.Sprintf("%s%s%s %s[ … ]%s", pal.Accent, indicator, aRst, pal.Dim, aRst)
		}
	}

	if !expanded {
		return []string{collapsedSummary}, true
	}

	// Expanded: header + pretty-printed lines.
	result := []string{collapsedSummary}
	result = append(result, prettyPrintJSON(trimmed, width, pal)...)
	return result, true
}

// prettyPrintJSON pretty-prints a JSON string with basic syntax highlighting.
// Returns display lines capped at 20; adds "… N more lines" if truncated.
func prettyPrintJSON(raw string, _ int, pal styles.ANSIPalette) []string {
	var v interface{}
	if err := json.Unmarshal([]byte(strings.TrimSpace(raw)), &v); err != nil {
		return nil
	}
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return nil
	}
	jsonLines := strings.Split(string(b), "\n")
	const maxLines = 20
	overflow := 0
	if len(jsonLines) > maxLines {
		overflow = len(jsonLines) - maxLines
		jsonLines = jsonLines[:maxLines]
	}
	result := make([]string, 0, len(jsonLines)+1)
	for _, l := range jsonLines {
		result = append(result, colorJSONLine(l, pal))
	}
	if overflow > 0 {
		result = append(result, pal.Dim+fmt.Sprintf("… %d more lines", overflow)+aRst)
	}
	return result
}

// colorJSONLine applies simple syntax highlighting to a single pretty-printed JSON line.
func colorJSONLine(line string, pal styles.ANSIPalette) string {
	// Lines with "key": value — color key in accent, value styled separately.
	colonIdx := strings.Index(line, `": `)
	if colonIdx != -1 {
		key := line[:colonIdx+1]   // includes leading indent + closing quote
		rest := line[colonIdx+1:]  // ": value..."
		return pal.Accent + key + pal.FG + ":" + colorJSONValuePart(rest[1:], pal) + aRst
	}
	// Structural lines: {, }, [, ], or bare values.
	return pal.FG + line + aRst
}

// colorJSONValuePart styles the value portion of a JSON line (after the colon+space).
func colorJSONValuePart(val string, pal styles.ANSIPalette) string {
	trimmed := strings.TrimLeft(val, " ")
	switch {
	case strings.HasPrefix(trimmed, `"`):
		return " " + pal.Dim + trimmed + aRst
	case strings.HasPrefix(trimmed, "true"), strings.HasPrefix(trimmed, "false"), strings.HasPrefix(trimmed, "null"):
		return " " + pal.Success + trimmed + aRst
	default:
		return " " + pal.FG + trimmed + aRst
	}
}
