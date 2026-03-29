package switchboard

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/adam-stokes/orcai/internal/busd/topics"
)

// ── splitStepOutput ───────────────────────────────────────────────────────────

func TestSplitStepOutput_Empty(t *testing.T) {
	if got := splitStepOutput(""); len(got) != 0 {
		t.Errorf("expected empty slice for empty input, got %v", got)
	}
}

func TestSplitStepOutput_WhitespaceOnly(t *testing.T) {
	if got := splitStepOutput("   \n  \n"); len(got) != 0 {
		t.Errorf("expected empty slice for whitespace-only input, got %v", got)
	}
}

func TestSplitStepOutput_TruncatesToLastFive(t *testing.T) {
	value := "line1\nline2\nline3\nline4\nline5\nline6\nline7"
	got := splitStepOutput(value)
	if len(got) != 5 {
		t.Fatalf("expected 5 lines, got %d: %v", len(got), got)
	}
	if got[0] != "line3" || got[4] != "line7" {
		t.Errorf("expected last 5 lines (line3–line7), got %v", got)
	}
}

func TestSplitStepOutput_SkipsBlankLines(t *testing.T) {
	got := splitStepOutput("a\n\nb\n\nc")
	if len(got) != 3 {
		t.Errorf("expected 3 non-empty lines, got %d: %v", len(got), got)
	}
}

// ── appendStepLines via handlePipelineBusEvent ────────────────────────────────

func makeStepDoneEvent(runID int64, step, value string) pipelineBusEventMsg {
	payload, _ := json.Marshal(map[string]any{
		"run_id": runID,
		"step":   step,
		"output": map[string]any{"value": value},
	})
	return pipelineBusEventMsg{topic: topics.StepDone, payload: payload}
}

func TestAppendStepLines_PopulatesViaStepDoneEvent(t *testing.T) {
	// Build a minimal model with one running feed entry and one step.
	m := Model{}
	entryID := fmt.Sprintf("run-%d", int64(42))
	m.feed = []feedEntry{
		{
			id:    entryID,
			title: "test-pipeline",
			steps: []StepInfo{{id: "review", status: "running"}},
		},
	}

	// Dispatch a StepDone event with multi-line output.
	evt := makeStepDoneEvent(42, "review", "line1\nline2\nline3")
	m = m.handlePipelineBusEvent(evt)

	// Step status should be updated to done.
	if m.feed[0].steps[0].status != "done" {
		t.Errorf("expected step status 'done', got %q", m.feed[0].steps[0].status)
	}
	// Step lines should be populated.
	if len(m.feed[0].steps[0].lines) != 3 {
		t.Fatalf("expected 3 step lines, got %d: %v", len(m.feed[0].steps[0].lines), m.feed[0].steps[0].lines)
	}
	if m.feed[0].steps[0].lines[0] != "line1" {
		t.Errorf("expected first line 'line1', got %q", m.feed[0].steps[0].lines[0])
	}
}

func TestAppendStepLines_EmptyOutputNoLines(t *testing.T) {
	m := Model{}
	entryID := fmt.Sprintf("run-%d", int64(7))
	m.feed = []feedEntry{
		{
			id:    entryID,
			steps: []StepInfo{{id: "fetch", status: "running"}},
		},
	}

	evt := makeStepDoneEvent(7, "fetch", "")
	m = m.handlePipelineBusEvent(evt)

	if len(m.feed[0].steps[0].lines) != 0 {
		t.Errorf("expected no step lines for empty output, got %v", m.feed[0].steps[0].lines)
	}
}
