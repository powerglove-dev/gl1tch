package console

import (
	"strings"
	"testing"

	"github.com/powerglove-dev/gl1tch/internal/styles"
)

var testPal = styles.ANSIPalette{
	Accent:  "\x1b[35m",
	Dim:     "\x1b[2m",
	Success: "\x1b[32m",
	FG:      "\x1b[97m",
}

// stripAnsiSimple strips ANSI escape sequences for test comparison.
func stripAnsiSimple(s string) string {
	return stripANSI(s)
}

func TestJsonFeedLineParser_CollapsedObject(t *testing.T) {
	raw := `{"name":"foo","count":3}`
	lines, ok := jsonFeedLineParser(raw, 80, testPal, false)
	if !ok {
		t.Fatal("expected match")
	}
	if len(lines) != 1 {
		t.Fatalf("expected 1 collapsed line, got %d", len(lines))
	}
	plain := stripAnsiSimple(lines[0])
	if !strings.HasPrefix(plain, "▸") {
		t.Errorf("collapsed line should start with ▸, got: %q", plain)
	}
	if !strings.Contains(plain, "2 keys") {
		t.Errorf("should show key count, got: %q", plain)
	}
}

func TestJsonFeedLineParser_CollapsedArray(t *testing.T) {
	raw := `[1,2,3,4,5]`
	lines, ok := jsonFeedLineParser(raw, 80, testPal, false)
	if !ok {
		t.Fatal("expected match")
	}
	if len(lines) != 1 {
		t.Fatalf("expected 1 collapsed line, got %d", len(lines))
	}
	plain := stripAnsiSimple(lines[0])
	if !strings.Contains(plain, "5 items") {
		t.Errorf("should show item count, got: %q", plain)
	}
}

func TestJsonFeedLineParser_Expanded(t *testing.T) {
	raw := `{"a":1,"b":2}`
	lines, ok := jsonFeedLineParser(raw, 80, testPal, true)
	if !ok {
		t.Fatal("expected match")
	}
	// Should have header + at least body lines
	if len(lines) < 2 {
		t.Fatalf("expected at least 2 lines when expanded, got %d", len(lines))
	}
	plain := stripAnsiSimple(lines[0])
	if !strings.HasPrefix(plain, "▾") {
		t.Errorf("expanded header should start with ▾, got: %q", plain)
	}
}

func TestJsonFeedLineParser_ExpandedCappedAt20(t *testing.T) {
	// Build a JSON object with 25 keys — pretty-print will exceed 20 lines.
	parts := make([]string, 25)
	for i := range parts {
		parts[i] = `"` + string(rune('a'+i)) + `":` + string(rune('0'+i%10))
	}
	raw := `{` + strings.Join(parts, ",") + `}`

	lines, ok := jsonFeedLineParser(raw, 80, testPal, true)
	if !ok {
		t.Fatal("expected match")
	}
	// header + 20 content lines + 1 overflow = 22
	if len(lines) > 22 {
		t.Errorf("expanded JSON should be capped; got %d lines", len(lines))
	}
	// last line should mention "more lines"
	last := stripAnsiSimple(lines[len(lines)-1])
	if !strings.Contains(last, "more lines") {
		t.Errorf("expected overflow indicator, got: %q", last)
	}
}

func TestJsonFeedLineParser_NonJSON(t *testing.T) {
	cases := []string{
		"hello world",
		"not json at all",
		"",
		"  ",
		`{"broken": json}`,
	}
	for _, raw := range cases {
		_, ok := jsonFeedLineParser(raw, 80, testPal, false)
		if ok {
			t.Errorf("expected no match for %q", raw)
		}
	}
}

func TestRunFeedLineParsers_FirstMatchWins(t *testing.T) {
	// Register a custom parser that matches everything and returns "custom"
	called := false
	custom := FeedLineParser(func(raw string, width int, pal styles.ANSIPalette, expanded bool) ([]string, bool) {
		called = true
		return []string{"custom"}, true
	})

	orig := feedParsers
	feedParsers = []FeedLineParser{custom}
	defer func() { feedParsers = orig }()

	lines, ok := runFeedLineParsers("anything", 80, testPal, false)
	if !ok || len(lines) != 1 || lines[0] != "custom" {
		t.Errorf("expected custom parser result, got ok=%v lines=%v", ok, lines)
	}
	if !called {
		t.Error("custom parser was not called")
	}
}

func TestRunFeedLineParsers_NoMatchFallthrough(t *testing.T) {
	noMatch := FeedLineParser(func(raw string, width int, pal styles.ANSIPalette, expanded bool) ([]string, bool) {
		return nil, false
	})

	orig := feedParsers
	feedParsers = []FeedLineParser{noMatch}
	defer func() { feedParsers = orig }()

	_, ok := runFeedLineParsers("hello", 80, testPal, false)
	if ok {
		t.Error("expected no match")
	}
}

func TestJsonIndicator_PrefixConsistency(t *testing.T) {
	// feedRawLines tags JSON summary lines with jsonIndicator so the enter
	// handler can detect them. Verify the parser always uses that constant.
	raw := `{"x":1}`
	lines, _ := jsonFeedLineParser(raw, 80, testPal, false)
	plain := stripAnsiSimple(lines[0])
	if !strings.HasPrefix(plain, jsonIndicator) {
		t.Errorf("collapsed JSON summary must start with jsonIndicator %q, got %q", jsonIndicator, plain)
	}
}
