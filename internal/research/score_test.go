package research

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// TestEvidenceCoverage_FormulaMatchesSpec covers the (grounded + 0.5 *
// partial) / total formula from the openspec proposal across the four
// representative cases: all grounded, mixed, all ungrounded, and empty.
func TestEvidenceCoverage_FormulaMatchesSpec(t *testing.T) {
	cases := map[string]struct {
		labels []CritiqueLabel
		want   float64
	}{
		"all grounded":   {[]CritiqueLabel{LabelGrounded, LabelGrounded}, 1.0},
		"all partial":    {[]CritiqueLabel{LabelPartial, LabelPartial}, 0.5},
		"all ungrounded": {[]CritiqueLabel{LabelUngrounded, LabelUngrounded}, 0.0},
		"mixed":          {[]CritiqueLabel{LabelGrounded, LabelPartial, LabelUngrounded}, (1.0 + 0.5) / 3.0},
		"empty":          {nil, 0},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			c := Critique{}
			for _, lbl := range tc.labels {
				c.Claims = append(c.Claims, CritiqueClaim{Text: "x", Label: lbl})
			}
			got := EvidenceCoverage(c)
			if got != tc.want {
				t.Errorf("EvidenceCoverage(%v) = %v, want %v", tc.labels, got, tc.want)
			}
		})
	}
}

// TestCrossCapabilityAgree_ReflectsSourceCount documents the structural
// signal: 0 sources → 0, 1 source → 0.4, ≥2 sources → 1.0. The cutoff is
// the spec's "≥2 researchers independently support the conclusion" rule.
func TestCrossCapabilityAgree_ReflectsSourceCount(t *testing.T) {
	cases := map[string]struct {
		bundle EvidenceBundle
		want   float64
	}{
		"empty":      {EvidenceBundle{}, 0.0},
		"one source": {bundleWith("git"), 0.4},
		"two sources": {
			bundleWith("git", "github-prs"),
			1.0,
		},
		"two items same source": {
			bundleWith("git", "git"),
			0.4,
		},
		"three sources": {
			bundleWith("git", "github-prs", "esearch"),
			1.0,
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			if got := CrossCapabilityAgree(tc.bundle); got != tc.want {
				t.Errorf("CrossCapabilityAgree(%v) = %v, want %v",
					tc.bundle.Sources(), got, tc.want)
			}
		})
	}
}

func bundleWith(sources ...string) EvidenceBundle {
	var b EvidenceBundle
	for _, s := range sources {
		b.Add(Evidence{Source: s, Title: s, Body: s})
	}
	return b
}

// TestParseCritique_AcceptsCommonOutputs covers the three shapes the
// critique parser is designed to tolerate: clean JSON, JSON with surrounding
// prose, and JSON with stray whitespace. Anything outside the {text,label}
// schema is normalised to LabelUngrounded so a noisy small-model output
// fails closed instead of open.
func TestParseCritique_AcceptsCommonOutputs(t *testing.T) {
	t.Run("clean", func(t *testing.T) {
		got, err := ParseCritique(`[{"text":"PR #1 is open","label":"grounded"}]`)
		if err != nil {
			t.Fatalf("ParseCritique: %v", err)
		}
		if len(got.Claims) != 1 || got.Claims[0].Label != LabelGrounded {
			t.Errorf("got %+v", got)
		}
	})
	t.Run("with preamble", func(t *testing.T) {
		raw := "Sure! Here are the labels: [{\"text\":\"x\",\"label\":\"partial\"}] thanks"
		got, err := ParseCritique(raw)
		if err != nil {
			t.Fatalf("ParseCritique: %v", err)
		}
		if len(got.Claims) != 1 || got.Claims[0].Label != LabelPartial {
			t.Errorf("got %+v", got)
		}
	})
	t.Run("unknown label normalises to ungrounded", func(t *testing.T) {
		got, err := ParseCritique(`[{"text":"x","label":"maybe"}]`)
		if err != nil {
			t.Fatalf("ParseCritique: %v", err)
		}
		if got.Claims[0].Label != LabelUngrounded {
			t.Errorf("unknown label should normalise to ungrounded, got %q", got.Claims[0].Label)
		}
	})
	t.Run("malformed errors", func(t *testing.T) {
		if _, err := ParseCritique("not a json array at all"); err == nil {
			t.Error("expected error on malformed input")
		}
	})
}

// TestParseJudgeScore covers the parser's clamp to [0,1], its tolerance of
// surrounding prose, and its rejection of values outside the range.
func TestParseJudgeScore(t *testing.T) {
	cases := map[string]struct {
		raw     string
		want    float64
		wantErr bool
	}{
		"bare":            {"0.85", 0.85, false},
		"with prose":      {"score: 0.42 (good)", 0.42, false},
		"integer one":     {"1", 1.0, false},
		"integer zero":    {"0", 0.0, false},
		"out of range":    {"1.5", 0, true},
		"negative":        {"score is bad", 0, true},
		"empty":           {"", 0, true},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			got, err := ParseJudgeScore(tc.raw)
			if tc.wantErr {
				if err == nil {
					t.Errorf("expected error, got %v", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseJudgeScore: %v", err)
			}
			if got != tc.want {
				t.Errorf("got %v, want %v", got, tc.want)
			}
		})
	}
}

// TestJudgePass_RoundtripsScore wires the prompt builder, an LLM stub, and
// the parser end-to-end so a regression in any of them surfaces here.
func TestJudgePass_RoundtripsScore(t *testing.T) {
	llm := func(_ context.Context, prompt string) (string, error) {
		// Sanity: the prompt must contain the question and the rubric
		// tag — otherwise the parser would happily round-trip a number
		// even if the prompt was malformed.
		if !strings.Contains(prompt, "judge stage") {
			t.Errorf("judge prompt missing rubric framing: %q", prompt)
		}
		return "0.73", nil
	}
	got, err := JudgePass(context.Background(), llm, "q", "draft", EvidenceBundle{})
	if err != nil {
		t.Fatalf("JudgePass: %v", err)
	}
	if got != 0.73 {
		t.Errorf("got %v, want 0.73", got)
	}
}

func TestSelfConsistency_RequiresMinimumN(t *testing.T) {
	llm := func(_ context.Context, _ string) (string, error) { return "1.0", nil }
	redraft := func(_ context.Context) (string, error) { return "alt", nil }
	if _, err := SelfConsistency(context.Background(), llm, "q", "orig", 1, redraft); err == nil {
		t.Error("expected error for n<2")
	}
}

func TestSelfConsistency_PassesAllAlternativesToCompare(t *testing.T) {
	var seenAlternatives int
	llm := func(_ context.Context, prompt string) (string, error) {
		seenAlternatives = strings.Count(prompt, "Alternative ")
		return "0.9", nil
	}
	calls := 0
	redraft := func(_ context.Context) (string, error) {
		calls++
		return "alt-" + string(rune('0'+calls)), nil
	}
	got, err := SelfConsistency(context.Background(), llm, "q", "orig", 4, redraft)
	if err != nil {
		t.Fatalf("SelfConsistency: %v", err)
	}
	if calls != 3 {
		t.Errorf("redraft calls = %d, want 3 (n-1 with n=4)", calls)
	}
	if seenAlternatives != 3 {
		t.Errorf("compare prompt should mention 3 alternatives, got %d", seenAlternatives)
	}
	if got != 0.9 {
		t.Errorf("got %v, want 0.9", got)
	}
}

// TestComposite_SkipsMissingSignals verifies the equal-weight average treats
// nil pointers as "not computed" and divides by the number of present
// signals — not by 4. A missing signal must not pull the composite down.
func TestComposite_SkipsMissingSignals(t *testing.T) {
	cases := map[string]struct {
		score Score
		want  float64
	}{
		"all four 1.0": {Score{
			SelfConsistency:      Ptr(1.0),
			EvidenceCoverage:     Ptr(1.0),
			CrossCapabilityAgree: Ptr(1.0),
			JudgeScore:           Ptr(1.0),
		}, 1.0},
		"only ec":        {Score{EvidenceCoverage: Ptr(0.6)}, 0.6},
		"three of four":  {Score{EvidenceCoverage: Ptr(0.8), CrossCapabilityAgree: Ptr(0.4), JudgeScore: Ptr(0.6)}, (0.8 + 0.4 + 0.6) / 3},
		"none":           {Score{}, 0},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			got := Composite(tc.score)
			if diff := got - tc.want; diff > 1e-9 || diff < -1e-9 {
				t.Errorf("Composite = %v, want %v", got, tc.want)
			}
		})
	}
}

// TestComputeScore_CheapFirstAndShortCircuit covers the two short-circuit
// paths: decisively-above (skip judge + self-consistency) and decisively-
// below (skip likewise). The check is that LLM call counts match the
// expected schedule, not just the final composite.
func TestComputeScore_CheapFirstAndShortCircuit(t *testing.T) {
	t.Run("decisive accept stops after critique", func(t *testing.T) {
		// CrossCap=1.0 (two sources). Critique → all grounded (EC=1.0).
		// Running average after two signals = 1.0; remaining 2 signals
		// at worst 0.0 still leave (1.0+1.0)/4 = 0.5 — that's NOT
		// unreachable from 0.5 threshold, so set threshold low to
		// trigger the decisive-accept path.
		bundle := bundleWith("a", "b")
		var llmCalls int
		llm := func(_ context.Context, _ string) (string, error) {
			llmCalls++
			return `[{"text":"x","label":"grounded"}]`, nil
		}
		opts := DefaultScoreOptions()
		opts.Threshold = 0.4
		opts.SkipSelfConsistency = false
		score, _, err := ComputeScore(context.Background(), ScoreInputs{
			Question: "q",
			Draft:    "d",
			Bundle:   bundle,
			LLM:      llm,
			Options:  opts,
			Redraft: func(context.Context) (string, error) {
				return "alt", nil
			},
		})
		if err != nil {
			t.Fatalf("ComputeScore: %v", err)
		}
		if llmCalls != 1 {
			t.Errorf("expected 1 LLM call (critique only) after decisive accept, got %d", llmCalls)
		}
		if score.JudgeScore != nil {
			t.Errorf("judge score should not be computed after short-circuit, got %v", *score.JudgeScore)
		}
		if score.SelfConsistency != nil {
			t.Errorf("self-consistency should not be computed after short-circuit")
		}
	})

	t.Run("decisive reject stops after cross-capability", func(t *testing.T) {
		// Empty bundle → CrossCap = 0.0. With threshold = 0.9 and
		// short-circuit on, even a perfect 1.0 on the remaining 3
		// signals only yields 3/4 = 0.75 < 0.9, so the loop should
		// stop without calling the LLM at all.
		var llmCalls int
		llm := func(_ context.Context, _ string) (string, error) {
			llmCalls++
			return "0.0", nil
		}
		opts := DefaultScoreOptions()
		opts.Threshold = 0.9
		score, _, err := ComputeScore(context.Background(), ScoreInputs{
			Bundle:  EvidenceBundle{},
			LLM:     llm,
			Options: opts,
		})
		if err != nil {
			t.Fatalf("ComputeScore: %v", err)
		}
		if llmCalls != 0 {
			t.Errorf("expected 0 LLM calls after decisive reject, got %d", llmCalls)
		}
		if score.EvidenceCoverage != nil || score.JudgeScore != nil || score.SelfConsistency != nil {
			t.Errorf("only cross-cap should be set, got %+v", score)
		}
	})
}

// TestComputeScore_SignalErrorsBecomeNilPointers covers the partial-bundle
// rule applied to scoring: an LLM error during critique or judge does not
// fail ComputeScore — it just leaves that signal nil so Composite skips it.
func TestComputeScore_SignalErrorsBecomeNilPointers(t *testing.T) {
	bundle := bundleWith("a")
	llm := func(_ context.Context, _ string) (string, error) {
		return "", errors.New("llm down")
	}
	opts := DefaultScoreOptions()
	opts.SkipSelfConsistency = true
	opts.ShortCircuit = false
	score, _, err := ComputeScore(context.Background(), ScoreInputs{
		Question: "q",
		Draft:    "d",
		Bundle:   bundle,
		LLM:      llm,
		Options:  opts,
	})
	if err != nil {
		t.Fatalf("ComputeScore: %v", err)
	}
	if score.CrossCapabilityAgree == nil || *score.CrossCapabilityAgree != 0.4 {
		t.Errorf("cross-cap should still be 0.4 (one source), got %v", score.CrossCapabilityAgree)
	}
	if score.EvidenceCoverage != nil {
		t.Errorf("evidence_coverage should be nil after critique LLM failure, got %v", *score.EvidenceCoverage)
	}
	if score.JudgeScore != nil {
		t.Errorf("judge_score should be nil after judge LLM failure, got %v", *score.JudgeScore)
	}
	// Composite uses only the present signals: 0.4.
	if score.Composite != 0.4 {
		t.Errorf("composite = %v, want 0.4 (cross-cap only)", score.Composite)
	}
}
