package research

import (
	"context"
	"errors"
	"testing"
	"time"
)

// stubVerifier is a controllable Verifier for tests. Calls counts how many
// times Verify was invoked; the verdict + err returned can be set per
// test. The Name field gives each test fixture a stable identifier so
// emitted EventTypeEscalation events can be asserted on.
type stubVerifier struct {
	name    string
	verdict Verdict
	err     error
	calls   int
}

func (s *stubVerifier) Name() string { return s.name }

func (s *stubVerifier) Verify(_ context.Context, _ VerifyInput) (Verdict, error) {
	s.calls++
	return s.verdict, s.err
}

// TestEscalation_DefaultsOff is the canonical "MaxPaidTokens=0 means
// escalation is OFF, period" test from openspec task 6.4. The loop is
// configured with a verifier and a sub-threshold composite, but the
// budget says zero paid tokens, so the verifier must NOT be called.
func TestEscalation_DefaultsOff(t *testing.T) {
	reg := newTwoSourceRegistry(t)
	llm := &scriptedByStage{
		plan:     []string{`["git-log"]`},
		draft:    []string{"some draft"},
		critique: []string{`[{"text":"x","label":"partial"}]`},
		judge:    []string{"0.3"},
	}
	verifier := &stubVerifier{name: "claude-stub"}
	sink := NewMemoryEventSink()
	loop := NewLoop(reg, llm.fn()).
		WithVerifier(verifier).
		WithEventSink(sink).
		WithScoreOptions(ScoreOptions{
			Threshold:           0.9,
			SkipSelfConsistency: true,
			ShortCircuit:        false,
		})

	res, err := loop.Run(context.Background(), ResearchQuery{Question: "q"},
		Budget{MaxIterations: 1, MaxWallclock: 5 * time.Second, MaxPaidTokens: 0})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	if verifier.calls != 0 {
		t.Errorf("verifier should NOT be called when MaxPaidTokens=0, got %d calls", verifier.calls)
	}
	if res.Reason != ReasonBudgetExceeded {
		t.Errorf("Reason = %s, want %s (escalation off)", res.Reason, ReasonBudgetExceeded)
	}
	for _, ev := range sink.Events() {
		if ev.Type == EventTypeEscalation {
			t.Errorf("no escalation event should be emitted when budget=0, got %+v", ev)
		}
	}
}

// TestEscalation_VerifierConfirms drives the round-trip when escalation is
// enabled and the verifier returns CONFIRM: the loop must keep the local
// draft, change Reason to ReasonEscalated, and emit one
// EventTypeEscalation record with verdict="confirm".
func TestEscalation_VerifierConfirms(t *testing.T) {
	reg := newTwoSourceRegistry(t)
	llm := &scriptedByStage{
		plan:     []string{`["git-log"]`},
		draft:    []string{"local draft mentioning #412"},
		critique: []string{`[{"text":"x","label":"partial"}]`},
		judge:    []string{"0.3"},
	}
	verifier := &stubVerifier{
		name:    "claude-test",
		verdict: Verdict{Verbatim: true, Output: "CONFIRM", PaidTokens: 1234},
	}
	sink := NewMemoryEventSink()
	loop := NewLoop(reg, llm.fn()).
		WithVerifier(verifier).
		WithEventSink(sink).
		WithScoreOptions(ScoreOptions{
			Threshold:           0.9,
			SkipSelfConsistency: true,
			ShortCircuit:        false,
		})

	res, err := loop.Run(context.Background(), ResearchQuery{Question: "q"},
		Budget{MaxIterations: 1, MaxWallclock: 5 * time.Second, MaxPaidTokens: 50_000})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	if verifier.calls != 1 {
		t.Errorf("verifier should be called once, got %d", verifier.calls)
	}
	if res.Reason != ReasonEscalated {
		t.Errorf("Reason = %s, want %s", res.Reason, ReasonEscalated)
	}
	if res.Draft != "local draft mentioning #412" {
		t.Errorf("CONFIRM verdict should preserve local draft, got %q", res.Draft)
	}

	var found bool
	for _, ev := range sink.Events() {
		if ev.Type == EventTypeEscalation {
			found = true
			if ev.PaidModel != "claude-test" {
				t.Errorf("event PaidModel = %q, want claude-test", ev.PaidModel)
			}
			if ev.Verdict != "confirm" {
				t.Errorf("event Verdict = %q, want confirm", ev.Verdict)
			}
			if ev.PaidTokens != 1234 {
				t.Errorf("event PaidTokens = %d, want 1234", ev.PaidTokens)
			}
		}
	}
	if !found {
		t.Errorf("expected one EventTypeEscalation, got events: %+v", sink.Events())
	}
}

// TestEscalation_VerifierRewrites covers the path where the paid model
// supersedes the local draft. The loop must replace Draft with the
// verifier's output and stamp ReasonEscalated.
func TestEscalation_VerifierRewrites(t *testing.T) {
	reg := newTwoSourceRegistry(t)
	llm := &scriptedByStage{
		plan:     []string{`["git-log"]`},
		draft:    []string{"local draft, possibly wrong"},
		critique: []string{`[{"text":"x","label":"partial"}]`},
		judge:    []string{"0.2"},
	}
	verifier := &stubVerifier{
		name: "claude-test",
		verdict: Verdict{
			Output:     "rewritten draft grounded in evidence",
			PaidTokens: 2000,
		},
	}
	loop := NewLoop(reg, llm.fn()).
		WithVerifier(verifier).
		WithScoreOptions(ScoreOptions{
			Threshold:           0.9,
			SkipSelfConsistency: true,
			ShortCircuit:        false,
		})

	res, err := loop.Run(context.Background(), ResearchQuery{Question: "q"},
		Budget{MaxIterations: 1, MaxWallclock: 5 * time.Second, MaxPaidTokens: 50_000})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Draft != "rewritten draft grounded in evidence" {
		t.Errorf("Draft = %q, want the verifier's rewrite", res.Draft)
	}
	if res.Reason != ReasonEscalated {
		t.Errorf("Reason = %s, want %s", res.Reason, ReasonEscalated)
	}
}

// TestEscalation_VerifierErrorFallsBackToBest covers the failure path:
// the verifier errors out, the loop logs it and returns the local
// best-effort answer with ReasonBudgetExceeded — never an error to the
// caller.
func TestEscalation_VerifierErrorFallsBackToBest(t *testing.T) {
	reg := newTwoSourceRegistry(t)
	llm := &scriptedByStage{
		plan:     []string{`["git-log"]`},
		draft:    []string{"local draft"},
		critique: []string{`[{"text":"x","label":"partial"}]`},
		judge:    []string{"0.3"},
	}
	verifier := &stubVerifier{
		name: "claude-test",
		err:  errors.New("verifier offline"),
	}
	loop := NewLoop(reg, llm.fn()).
		WithVerifier(verifier).
		WithScoreOptions(ScoreOptions{
			Threshold:           0.9,
			SkipSelfConsistency: true,
			ShortCircuit:        false,
		})

	res, err := loop.Run(context.Background(), ResearchQuery{Question: "q"},
		Budget{MaxIterations: 1, MaxWallclock: 5 * time.Second, MaxPaidTokens: 50_000})
	if err != nil {
		t.Fatalf("verifier error must NOT propagate to Run: %v", err)
	}
	if res.Reason != ReasonBudgetExceeded {
		t.Errorf("Reason = %s, want %s (verifier offline → best-effort)", res.Reason, ReasonBudgetExceeded)
	}
	if res.Draft != "local draft" {
		t.Errorf("Draft should be the local best-effort answer, got %q", res.Draft)
	}
}

// TestParseVerdict covers the CONFIRM-vs-rewrite parser.
func TestParseVerdict(t *testing.T) {
	cases := map[string]struct {
		raw      string
		want     Verdict
		wantErr  bool
	}{
		"confirm exact":     {"CONFIRM", Verdict{Verbatim: true, Output: "CONFIRM"}, false},
		"confirm leading":   {"  CONFIRM\n", Verdict{Verbatim: true, Output: "CONFIRM"}, false},
		"confirm lowercase": {"confirm.", Verdict{Verbatim: true, Output: "confirm."}, false},
		"rewrite":           {"Here's a better draft.", Verdict{Output: "Here's a better draft."}, false},
		"empty":             {"   ", Verdict{}, true},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			got, err := ParseVerdict(tc.raw)
			if tc.wantErr {
				if err == nil {
					t.Error("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseVerdict: %v", err)
			}
			if got != tc.want {
				t.Errorf("got %+v, want %+v", got, tc.want)
			}
		})
	}
}
