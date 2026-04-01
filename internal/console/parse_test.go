package console

import (
	"testing"
	"time"
)

// ── parseStepStatus (task 3.7) ────────────────────────────────────────────────

func TestParseStepStatus_Valid(t *testing.T) {
	stepID, status, ok := parseStepStatus("[step:fetch] status:running")
	if !ok {
		t.Fatal("expected ok=true for valid line")
	}
	if stepID != "fetch" {
		t.Errorf("expected stepID %q, got %q", "fetch", stepID)
	}
	if status != "running" {
		t.Errorf("expected status %q, got %q", "running", status)
	}
}

func TestParseStepStatus_EmptyStepID(t *testing.T) {
	_, _, ok := parseStepStatus("[step:] status:running")
	if ok {
		t.Error("expected ok=false for empty step ID")
	}
}

func TestParseStepStatus_EmptyStatus(t *testing.T) {
	_, _, ok := parseStepStatus("[step:fetch] status:")
	if ok {
		t.Error("expected ok=false for empty status")
	}
}

func TestParseStepStatus_NonMatchingLine(t *testing.T) {
	_, _, ok := parseStepStatus("hello world")
	if ok {
		t.Error("expected ok=false for non-matching line")
	}
}

func TestParseStepStatus_PartialPrefix(t *testing.T) {
	_, _, ok := parseStepStatus("[step:fetch]")
	if ok {
		t.Error("expected ok=false for partial prefix without status")
	}
}

// ── StepStatusMsg update (task 3.8) ───────────────────────────────────────────

func TestStepStatusMsg_UpdatesExistingStep(t *testing.T) {
	m := New()
	// Add a feed entry with a known step pre-populated.
	entry := feedEntry{
		id:     "job1",
		title:  "pipeline: test",
		status: FeedRunning,
		ts:     time.Now(),
		steps:  []StepInfo{{id: "fetch", status: "pending"}},
	}
	m.feed = append([]feedEntry{entry}, m.feed...)

	// Send a StepStatusMsg to update the step.
	m2, _ := m.Update(StepStatusMsg{FeedID: "job1", StepID: "fetch", Status: "running"})
	updated := m2.(Model)

	// Verify the step status was updated.
	if len(updated.feed) == 0 {
		t.Fatal("feed is empty after update")
	}
	e := updated.feed[0]
	if len(e.steps) == 0 {
		t.Fatal("feed entry has no steps after update")
	}
	if e.steps[0].status != "running" {
		t.Errorf("expected step status %q, got %q", "running", e.steps[0].status)
	}
}

func TestStepStatusMsg_AppendsNewStep(t *testing.T) {
	m := New()
	// Add a feed entry with no steps.
	entry := feedEntry{
		id:     "job2",
		title:  "pipeline: test",
		status: FeedRunning,
		ts:     time.Now(),
	}
	m.feed = append([]feedEntry{entry}, m.feed...)

	// Send a StepStatusMsg for a step not yet in the list.
	m2, _ := m.Update(StepStatusMsg{FeedID: "job2", StepID: "process", Status: "done"})
	updated := m2.(Model)

	if len(updated.feed) == 0 {
		t.Fatal("feed is empty after update")
	}
	e := updated.feed[0]
	if len(e.steps) != 1 {
		t.Fatalf("expected 1 step appended, got %d", len(e.steps))
	}
	if e.steps[0].id != "process" || e.steps[0].status != "done" {
		t.Errorf("unexpected step: id=%q status=%q", e.steps[0].id, e.steps[0].status)
	}
}
