package research

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/8op-org/gl1tch/internal/capability"
)

// TestLoopCapability_StreamsDraftAndScore drives a happy-path Loop through
// the LoopCapability adapter and asserts that Invoke streams the draft and
// the trailing evidence/confidence block as Stream events, with no Error
// events on the channel.
func TestLoopCapability_StreamsDraftAndScore(t *testing.T) {
	reg := newTwoSourceRegistry(t)
	llm := &scriptedByStage{
		plan:     []string{`["git-log","github-prs"]`},
		draft:    []string{"Two PRs are open: #412 and #418."},
		critique: []string{`[{"text":"#412 open","label":"grounded"}]`},
		judge:    []string{"0.9"},
	}
	loop := NewLoop(reg, llm.fn()).WithScoreOptions(ScoreOptions{
		Threshold:           0.7,
		SkipSelfConsistency: true,
		ShortCircuit:        false,
	})
	cap := &LoopCapability{Loop: loop}

	// Manifest sanity: name defaults to "research", on-demand trigger,
	// stream sink. The category is consumed by the assistant's pick
	// prompt to filter capabilities by intent.
	m := cap.Manifest()
	if m.Name != "research" {
		t.Errorf("Manifest.Name = %q, want research", m.Name)
	}
	if m.Trigger.Mode != capability.TriggerOnDemand {
		t.Errorf("Manifest.Trigger.Mode = %v, want on-demand", m.Trigger.Mode)
	}
	if !m.Sink.Stream {
		t.Errorf("Manifest.Sink.Stream should be true")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	ch, err := cap.Invoke(ctx, capability.Input{Stdin: "what's open?"})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}

	var combined strings.Builder
	for ev := range ch {
		switch ev.Kind {
		case capability.EventStream:
			combined.WriteString(ev.Text)
		case capability.EventError:
			t.Errorf("unexpected error event: %v", ev.Err)
		case capability.EventDoc:
			t.Errorf("LoopCapability should not emit doc events")
		}
	}

	out := combined.String()
	for _, want := range []string{"#412", "#418", "evidence:", "confidence:", "composite="} {
		if !strings.Contains(out, want) {
			t.Errorf("stream output missing %q\nfull output:\n%s", want, out)
		}
	}
}

// TestLoopCapability_RejectsEmptyQuestion covers the contract that a
// research call without a question is a programming error, not silent
// failure. The wrapper returns an error from Invoke directly (not a
// channel error event) so callers can fail fast.
func TestLoopCapability_RejectsEmptyQuestion(t *testing.T) {
	cap := &LoopCapability{Loop: NewLoop(NewRegistry(), func(context.Context, string) (string, error) { return "", nil })}
	if _, err := cap.Invoke(context.Background(), capability.Input{Stdin: "   "}); err == nil {
		t.Error("expected error on empty question")
	}
}

// TestLoopCapability_NameOverride covers the test/A-B path: a caller can
// register multiple loops side by side under different names without
// modifying the underlying Loop type.
func TestLoopCapability_NameOverride(t *testing.T) {
	cap := &LoopCapability{
		Loop:         NewLoop(NewRegistry(), func(context.Context, string) (string, error) { return "", nil }),
		NameOverride: "research-experimental",
	}
	if got := cap.Manifest().Name; got != "research-experimental" {
		t.Errorf("Manifest.Name = %q, want research-experimental", got)
	}
}
