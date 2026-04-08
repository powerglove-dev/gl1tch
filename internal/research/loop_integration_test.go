package research

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"
)

// scriptedFn returns an LLMFn that responds to each prompt category
// (plan / draft / critique / judge / self-consistency-compare) with a value
// looked up from the supplied map. The category is identified by a marker
// substring that PlanPrompt / DraftPrompt / CritiquePrompt / JudgePrompt /
// SelfConsistencyPrompt all embed in their text. Unmatched prompts return
// the empty string with no error so a test can drive the loop without
// scripting every single call — the score module's "missing signal becomes
// nil" rule means an empty critique reply just leaves EvidenceCoverage nil.
type scriptedByStage struct {
	plan          []string // queue: one response per plan call
	draft         []string
	critique      []string
	judge         []string
	selfCompare   []string
	mu            sync.Mutex
}

func (s *scriptedByStage) fn() LLMFn {
	return func(_ context.Context, prompt string) (string, error) {
		s.mu.Lock()
		defer s.mu.Unlock()
		switch {
		case strings.Contains(prompt, "planning stage of a research loop"):
			return pop(&s.plan), nil
		case strings.Contains(prompt, "drafting stage of a research loop"):
			return pop(&s.draft), nil
		case strings.Contains(prompt, "critique stage of a research loop"):
			return pop(&s.critique), nil
		case strings.Contains(prompt, "judge stage of a research loop"):
			return pop(&s.judge), nil
		case strings.Contains(prompt, "self-consistency stage of a research loop"):
			return pop(&s.selfCompare), nil
		default:
			return "", nil
		}
	}
}

func pop(q *[]string) string {
	if len(*q) == 0 {
		return ""
	}
	v := (*q)[0]
	*q = (*q)[1:]
	return v
}

func newTwoSourceRegistry(t *testing.T) *Registry {
	t.Helper()
	reg := NewRegistry()
	for _, r := range []pickedResearcher{
		{
			name:     "git-log",
			describe: "recent commits",
			body:     "abc1234 2026-04-08 stokes: refactor router\ndef5678 2026-04-08 stokes: add brain stats",
			refs:     []string{"abc1234", "def5678"},
		},
		{
			name:     "github-prs",
			describe: "open prs",
			body:     "PR #412 refactor router (open) by stokes\nPR #418 brain stats (open) by stokes",
			refs:     []string{"https://github.com/example/repo/pull/412"},
		},
	} {
		if err := reg.Register(r); err != nil {
			t.Fatalf("register %s: %v", r.name, err)
		}
	}
	return reg
}

// TestRun_AcceptOnFirstIteration drives the loop with a planner that picks
// both researchers, a critique that labels every claim grounded, a
// cross-cap of 1.0 (two sources), an EvidenceCoverage of 1.0 (all
// grounded), and a JudgeScore of 0.9 — composite ≈ 0.97 over the default
// 0.7 threshold. SelfConsistency is skipped to keep the script short.
// The test asserts the loop returns ReasonAccepted with iterations=1 and
// the per-signal breakdown is populated.
func TestRun_AcceptOnFirstIteration(t *testing.T) {
	reg := newTwoSourceRegistry(t)
	llm := &scriptedByStage{
		plan:     []string{`["git-log","github-prs"]`},
		draft:    []string{"Two PRs are open: #412 (refactor router) and #418 (brain stats), see commits abc1234 and def5678."},
		critique: []string{`[{"text":"PR #412 is open","label":"grounded"},{"text":"PR #418 is open","label":"grounded"},{"text":"commit abc1234 is the router refactor","label":"grounded"}]`},
		judge:    []string{"0.9"},
	}
	loop := NewLoop(reg, llm.fn()).WithScoreOptions(ScoreOptions{
		Threshold:           0.7,
		SkipSelfConsistency: true,
		ShortCircuit:        false,
	})

	res, err := loop.Run(context.Background(), ResearchQuery{
		Question: "what PRs are open right now?",
	}, DefaultBudget())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	if res.Reason != ReasonAccepted {
		t.Errorf("Reason = %s, want %s", res.Reason, ReasonAccepted)
	}
	if res.Iterations != 1 {
		t.Errorf("Iterations = %d, want 1", res.Iterations)
	}
	if res.Score.EvidenceCoverage == nil || *res.Score.EvidenceCoverage != 1.0 {
		t.Errorf("EvidenceCoverage = %v, want 1.0", res.Score.EvidenceCoverage)
	}
	if res.Score.CrossCapabilityAgree == nil || *res.Score.CrossCapabilityAgree != 1.0 {
		t.Errorf("CrossCapabilityAgree = %v, want 1.0 (two sources)", res.Score.CrossCapabilityAgree)
	}
	if res.Score.JudgeScore == nil || *res.Score.JudgeScore != 0.9 {
		t.Errorf("JudgeScore = %v, want 0.9", res.Score.JudgeScore)
	}
	if !strings.Contains(res.Draft, "#412") {
		t.Errorf("Draft missing expected PR identifier: %q", res.Draft)
	}
}

// TestRun_RefineThenAccept drives a two-iteration scenario: the first
// iteration only picks git-log (one source → cross_cap 0.4), the critique
// labels one claim partial (so EvidenceCoverage = 0.5), JudgeScore 0.6,
// and the composite lands below 0.7. The loop must extract the partial
// claim, feed it back to the planner, and the second iteration picks
// github-prs (which clears the missing claim). Now cross_cap = 1.0,
// EvidenceCoverage = 1.0, JudgeScore = 0.9 → composite ≈ 0.97, accepted.
//
// The test asserts the second plan response was actually consulted (not
// just the first), the bundle merged commits + PRs, and Iterations=2.
func TestRun_RefineThenAccept(t *testing.T) {
	reg := newTwoSourceRegistry(t)
	llm := &scriptedByStage{
		plan: []string{
			`["git-log"]`,                     // iter 1: only commits
			`["github-prs"]`,                  // iter 2: planner picks PRs after refine
		},
		draft: []string{
			"Commits abc1234 and def5678 touched the router.",
			"PRs #412 and #418 are open; commits abc1234 and def5678 are the router refactor work.",
		},
		critique: []string{
			`[{"text":"PR #412 is open","label":"partial"}]`, // iter 1: partial → refine
			`[{"text":"PR #412 is open","label":"grounded"},{"text":"commit abc1234 touched router","label":"grounded"}]`,
		},
		judge: []string{"0.6", "0.9"},
	}
	loop := NewLoop(reg, llm.fn()).WithScoreOptions(ScoreOptions{
		Threshold:           0.7,
		SkipSelfConsistency: true,
		ShortCircuit:        false,
	})

	res, err := loop.Run(context.Background(), ResearchQuery{
		Question: "are there open PRs related to recent commits?",
	}, Budget{MaxIterations: 3, MaxWallclock: 10 * time.Second})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	if res.Reason != ReasonAccepted {
		t.Errorf("Reason = %s, want %s", res.Reason, ReasonAccepted)
	}
	if res.Iterations != 2 {
		t.Errorf("Iterations = %d, want 2", res.Iterations)
	}
	// The bundle must contain BOTH researchers — refine merges, not replaces.
	sources := res.Bundle.Sources()
	if len(sources) != 2 {
		t.Errorf("Bundle.Sources = %v, want both git-log and github-prs", sources)
	}
}

// TestRun_BudgetExceededReturnsBest sets MaxIterations=1 and a critique
// that yields a sub-threshold composite. The loop must return the only
// Result it produced with Reason=ReasonBudgetExceeded — not throw it away,
// not error.
func TestRun_BudgetExceededReturnsBest(t *testing.T) {
	reg := newTwoSourceRegistry(t)
	llm := &scriptedByStage{
		plan:     []string{`["git-log"]`},
		draft:    []string{"Some commits touched the router."},
		critique: []string{`[{"text":"router was touched","label":"partial"}]`},
		judge:    []string{"0.3"},
	}
	loop := NewLoop(reg, llm.fn()).WithScoreOptions(ScoreOptions{
		Threshold:           0.9,
		SkipSelfConsistency: true,
		ShortCircuit:        false,
	})

	res, err := loop.Run(context.Background(), ResearchQuery{
		Question: "what's going on with the router?",
	}, Budget{MaxIterations: 1, MaxWallclock: 10 * time.Second})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	if res.Reason != ReasonBudgetExceeded {
		t.Errorf("Reason = %s, want %s", res.Reason, ReasonBudgetExceeded)
	}
	if res.Draft == "" {
		t.Errorf("budget-exceeded result should still carry the best draft")
	}
	if res.Score.Composite >= 0.9 {
		t.Errorf("test expected sub-threshold composite, got %v", res.Score.Composite)
	}
}

// TestRun_BudgetExceededOnEmptyRefine covers the edge case where the
// critique returns no ungrounded claims (every claim grounded) but the
// composite still didn't clear the bar — typically because cross_cap is
// 0.4 with one source. The loop must NOT loop forever; it must return
// the current Result with ReasonBudgetExceeded immediately.
func TestRun_BudgetExceededOnEmptyRefine(t *testing.T) {
	reg := newTwoSourceRegistry(t)
	llm := &scriptedByStage{
		plan:     []string{`["git-log"]`},
		draft:    []string{"abc1234 touched router."},
		critique: []string{`[{"text":"abc1234 touched router","label":"grounded"}]`},
		judge:    []string{"0.5"},
	}
	loop := NewLoop(reg, llm.fn()).WithScoreOptions(ScoreOptions{
		Threshold:           0.9,
		SkipSelfConsistency: true,
		ShortCircuit:        false,
	})

	res, err := loop.Run(context.Background(), ResearchQuery{Question: "q"},
		Budget{MaxIterations: 5, MaxWallclock: 10 * time.Second})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Reason != ReasonBudgetExceeded {
		t.Errorf("Reason = %s, want %s (no useful refine signal)", res.Reason, ReasonBudgetExceeded)
	}
	if res.Iterations != 1 {
		t.Errorf("Iterations = %d, want 1 (no refine because nothing to refine on)", res.Iterations)
	}
}

// TestRun_WallclockBudgetEnforced sets a tiny wallclock budget and a slow
// LLMFn so the deadline trips before the second iteration. The loop must
// return the iteration-1 best Result with ReasonBudgetExceeded.
func TestRun_WallclockBudgetEnforced(t *testing.T) {
	reg := newTwoSourceRegistry(t)
	var calls int
	llm := func(_ context.Context, prompt string) (string, error) {
		calls++
		// Slow second iteration to push past the deadline.
		if calls > 4 {
			time.Sleep(60 * time.Millisecond)
		}
		switch {
		case strings.Contains(prompt, "planning stage"):
			return `["git-log"]`, nil
		case strings.Contains(prompt, "drafting stage"):
			return "draft", nil
		case strings.Contains(prompt, "critique stage"):
			return `[{"text":"x","label":"partial"}]`, nil
		case strings.Contains(prompt, "judge stage"):
			return "0.3", nil
		}
		return "", nil
	}
	loop := NewLoop(reg, llm).WithScoreOptions(ScoreOptions{
		Threshold:           0.9,
		SkipSelfConsistency: true,
		ShortCircuit:        false,
	})

	res, err := loop.Run(context.Background(), ResearchQuery{Question: "q"},
		Budget{MaxIterations: 5, MaxWallclock: 50 * time.Millisecond})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Reason != ReasonBudgetExceeded {
		t.Errorf("Reason = %s, want %s", res.Reason, ReasonBudgetExceeded)
	}
}
