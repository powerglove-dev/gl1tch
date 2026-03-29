package switchboard

import (
	"strings"
	"testing"

	"github.com/adam-stokes/orcai/internal/store"
	"github.com/adam-stokes/orcai/internal/styles"
)

func testPalette() styles.ANSIPalette {
	return styles.ANSIPalette{}
}

func TestBuildRunContent_StepOutputLastFiveLines(t *testing.T) {
	lines := []string{
		"TRUNCATED-LINE-ONE",
		"TRUNCATED-LINE-TWO",
		"TRUNCATED-LINE-THREE",
		"VISIBLE-LINE-FOUR",
		"VISIBLE-LINE-FIVE",
		"VISIBLE-LINE-SIX",
		"VISIBLE-LINE-SEVEN",
		"VISIBLE-LINE-EIGHT",
	}
	output := strings.Join(lines, "\n")
	run := store.Run{
		StartedAt: 1000,
		Steps: []store.StepRecord{
			{ID: "fetch", Status: "done", Output: map[string]any{"value": output}},
		},
	}
	got := buildRunContent(run, testPalette(), false, 80)
	// Only last 5 lines should appear
	for _, line := range lines[:3] {
		if strings.Contains(got, line) {
			t.Errorf("expected line %q to be truncated but found in output", line)
		}
	}
	for _, line := range lines[3:] {
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
