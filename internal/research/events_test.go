package research

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestLoop_EmitsAttemptAndScorePerIteration drives a two-iteration refine
// scenario through an in-memory sink and asserts that every iteration
// produced one EventTypeAttempt + one EventTypeScore (4 events total) with
// the right shape: attempt has the bundle, score does not.
func TestLoop_EmitsAttemptAndScorePerIteration(t *testing.T) {
	reg := newTwoSourceRegistry(t)
	llm := &scriptedByStage{
		plan: []string{
			`["git-log"]`,
			`["github-prs"]`,
		},
		draft: []string{
			"first draft",
			"second draft with #412",
		},
		critique: []string{
			`[{"text":"x","label":"partial"}]`,
			`[{"text":"#412 open","label":"grounded"}]`,
		},
		judge: []string{"0.4", "0.95"},
	}
	sink := NewMemoryEventSink()
	loop := NewLoop(reg, llm.fn()).
		WithEventSink(sink).
		WithScoreOptions(ScoreOptions{
			Threshold:           0.7,
			SkipSelfConsistency: true,
			ShortCircuit:        false,
		})

	res, err := loop.Run(context.Background(), ResearchQuery{
		ID:       "q-test-1",
		Question: "what's open?",
	}, Budget{MaxIterations: 3, MaxWallclock: 5 * time.Second})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Reason != ReasonAccepted || res.Iterations != 2 {
		t.Fatalf("expected accepted in 2 iters, got %s/%d", res.Reason, res.Iterations)
	}

	events := sink.Events()
	if len(events) != 4 {
		t.Fatalf("expected 4 events (2 iters × 2 events), got %d", len(events))
	}

	// First iter should be attempt+score with iteration=1.
	if events[0].Type != EventTypeAttempt || events[0].Iteration != 1 {
		t.Errorf("event[0] = %+v, want attempt iter=1", events[0])
	}
	if events[0].Bundle == nil {
		t.Errorf("attempt event should carry the bundle")
	}
	if events[0].QueryID != "q-test-1" {
		t.Errorf("event[0].QueryID = %q, want q-test-1", events[0].QueryID)
	}
	if events[1].Type != EventTypeScore || events[1].Iteration != 1 {
		t.Errorf("event[1] = %+v, want score iter=1", events[1])
	}
	if events[1].Bundle != nil {
		t.Errorf("score event should NOT carry the bundle")
	}

	// Second iter.
	if events[2].Type != EventTypeAttempt || events[2].Iteration != 2 {
		t.Errorf("event[2] = %+v, want attempt iter=2", events[2])
	}
	if events[3].Type != EventTypeScore || events[3].Iteration != 2 {
		t.Errorf("event[3] = %+v, want score iter=2", events[3])
	}

	// Score breakdown should be present on the score event.
	if events[3].Score.Composite == 0 {
		t.Errorf("score event composite should be > 0, got %v", events[3].Score)
	}
}

// TestFileEventSink_AppendsAndRotates writes two events with timestamps
// straddling the TTL cutoff, then writes a third — and asserts that the
// expired event is gone after rotation.
func TestFileEventSink_AppendsAndRotates(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "events.jsonl")
	sink := NewFileEventSink(path)
	sink.TTL = 100 * time.Millisecond

	// Write an "old" event by stamping a past timestamp directly.
	oldEvent := Event{
		Type:      EventTypeAttempt,
		Timestamp: time.Now().Add(-1 * time.Hour).Format(time.RFC3339),
		QueryID:   "old",
		Iteration: 1,
	}
	if err := sink.Emit(oldEvent); err != nil {
		t.Fatalf("Emit old: %v", err)
	}

	// Write a "fresh" event with the current timestamp.
	freshEvent := Event{
		Type:      EventTypeAttempt,
		Timestamp: time.Now().Format(time.RFC3339),
		QueryID:   "fresh",
		Iteration: 1,
	}
	if err := sink.Emit(freshEvent); err != nil {
		t.Fatalf("Emit fresh: %v", err)
	}

	// Sleep past TTL, then emit a third event — rotation should drop "fresh".
	time.Sleep(150 * time.Millisecond)
	thirdEvent := Event{
		Type:      EventTypeAttempt,
		Timestamp: time.Now().Format(time.RFC3339),
		QueryID:   "third",
		Iteration: 1,
	}
	if err := sink.Emit(thirdEvent); err != nil {
		t.Fatalf("Emit third: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read file: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")

	// Parse each kept line and check we got only "third" left after rotation.
	var keptIDs []string
	for _, line := range lines {
		if line == "" {
			continue
		}
		var ev Event
		if err := json.Unmarshal([]byte(line), &ev); err != nil {
			t.Fatalf("parse line %q: %v", line, err)
		}
		keptIDs = append(keptIDs, ev.QueryID)
	}

	// "old" should have been rotated out on the second emit, and "fresh"
	// should have been rotated out on the third emit. Only "third" remains.
	if len(keptIDs) != 1 || keptIDs[0] != "third" {
		t.Errorf("kept events = %v, want [third]", keptIDs)
	}
}

// TestNopSink_DoesNothing covers the default-sink path: a Loop with no
// WithEventSink call must not panic, and must not crash a test that
// happens to inspect the (absent) events. This is also a regression guard
// against a future refactor that drops the nopSink default.
func TestNopSink_DoesNothing(t *testing.T) {
	reg := newTwoSourceRegistry(t)
	llm := &scriptedByStage{
		plan:     []string{`["git-log"]`},
		draft:    []string{"draft"},
		critique: []string{`[{"text":"x","label":"grounded"}]`},
		judge:    []string{"0.95"},
	}
	loop := NewLoop(reg, llm.fn()).WithScoreOptions(ScoreOptions{
		Threshold:           0.7,
		SkipSelfConsistency: true,
		ShortCircuit:        false,
	})
	if _, err := loop.Run(context.Background(), ResearchQuery{Question: "q"}, DefaultBudget()); err != nil {
		t.Fatalf("Run: %v", err)
	}
	// Test passes if no panic.
}
