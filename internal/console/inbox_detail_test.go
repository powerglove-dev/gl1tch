package console

import (
	"strings"
	"testing"

	"github.com/8op-org/gl1tch/internal/store"
	"github.com/8op-org/gl1tch/internal/styles"
)

func testPalette() styles.ANSIPalette {
	return styles.ANSIPalette{}
}

func TestBuildRunContent_StepOutputAllLinesShown(t *testing.T) {
	lines := []string{
		"LINE-ONE",
		"LINE-TWO",
		"LINE-THREE",
		"LINE-FOUR",
		"LINE-FIVE",
		"LINE-SIX",
		"LINE-SEVEN",
		"LINE-EIGHT",
	}
	output := strings.Join(lines, "\n")
	run := store.Run{
		StartedAt: 1000,
		Steps: []store.StepRecord{
			{ID: "fetch", Status: "done", Output: map[string]any{"value": output}},
		},
	}
	got := buildRunContent(run, testPalette(), false, 80)
	// All lines should appear — truncation was removed in favour of showing full step output.
	for _, line := range lines {
		if !strings.Contains(got, line) {
			t.Errorf("expected line %q in output but not found", line)
		}
	}
}

func TestBuildRunContent_StepOutputEmptyShowsNoLines(t *testing.T) {
	run := store.Run{
		StartedAt: 1000,
		Steps: []store.StepRecord{
			{ID: "fetch", Status: "done", Output: map[string]any{"value": ""}},
		},
	}
	got := buildRunContent(run, testPalette(), false, 80)
	// Step should appear, but no extra output lines
	if !strings.Contains(got, "fetch") {
		t.Error("expected step ID 'fetch' in output")
	}
}

func TestBuildRunContent_StepNoOutputKey(t *testing.T) {
	run := store.Run{
		StartedAt: 1000,
		Steps: []store.StepRecord{
			{ID: "fetch", Status: "done", Output: map[string]any{}},
		},
	}
	got := buildRunContent(run, testPalette(), false, 80)
	if !strings.Contains(got, "fetch") {
		t.Error("expected step ID 'fetch' in output")
	}
}

func TestBuildRunContent_StepOutputExactlyFiveLines(t *testing.T) {
	lines := []string{"x", "y", "z", "w", "v"}
	output := strings.Join(lines, "\n")
	run := store.Run{
		StartedAt: 1000,
		Steps: []store.StepRecord{
			{ID: "check", Status: "done", Output: map[string]any{"value": output}},
		},
	}
	got := buildRunContent(run, testPalette(), false, 80)
	for _, line := range lines {
		if !strings.Contains(got, line) {
			t.Errorf("expected line %q in output but not found", line)
		}
	}
}
