package research

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
)

// scriptedLLM returns an LLMFn that responds to each call with the next
// canned response from the script. Unmatched calls return an error so the
// test fails loudly when the loop calls the model more times than expected.
type scriptedLLM struct {
	mu       sync.Mutex
	responses []string
	prompts   []string
}

func (s *scriptedLLM) Fn() LLMFn {
	return func(_ context.Context, prompt string) (string, error) {
		s.mu.Lock()
		defer s.mu.Unlock()
		s.prompts = append(s.prompts, prompt)
		if len(s.responses) == 0 {
			return "", errors.New("scriptedLLM: no more responses")
		}
		next := s.responses[0]
		s.responses = s.responses[1:]
		return next, nil
	}
}

// pickedResearcher is a Researcher whose Gather returns a fixed Evidence.
type pickedResearcher struct {
	name     string
	describe string
	body     string
	refs     []string
	err      error
}

func (p pickedResearcher) Name() string     { return p.name }
func (p pickedResearcher) Describe() string { return p.describe }
func (p pickedResearcher) Gather(_ context.Context, _ ResearchQuery, _ EvidenceBundle) (Evidence, error) {
	if p.err != nil {
		return Evidence{}, p.err
	}
	return Evidence{
		Source: p.name,
		Title:  p.name + " evidence",
		Body:   p.body,
		Refs:   p.refs,
	}, nil
}

func TestLoopHappyPath(t *testing.T) {
	reg := NewRegistry()
	_ = reg.Register(pickedResearcher{
		name:     "github-prs",
		describe: "lists open PRs in the current repo",
		body:     "PR #412: refactor router (open)\nPR #418: brain stats (open)",
		refs:     []string{"https://github.com/8op-org/gl1tch/pull/412"},
	})

	llm := &scriptedLLM{
		responses: []string{
			// Plan response: pick github-prs
			`["github-prs"]`,
			// Draft response: a grounded answer that cites the evidence
			"Two open PRs: #412 (refactor router) and #418 (brain stats).",
		},
	}

	loop := NewLoop(reg, llm.Fn()).WithScoreOptions(ScoreOptions{Disabled: true})
	res, err := loop.Run(context.Background(), ResearchQuery{
		Question: "what PRs are open right now?",
	}, DefaultBudget())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Reason != ReasonAccepted {
		t.Errorf("Reason = %s, want %s", res.Reason, ReasonAccepted)
	}
	if res.Bundle.Len() != 1 {
		t.Errorf("Bundle.Len() = %d, want 1", res.Bundle.Len())
	}
	if !strings.Contains(res.Draft, "#412") || !strings.Contains(res.Draft, "#418") {
		t.Errorf("Draft missing expected PR numbers: %q", res.Draft)
	}
	// Two LLM calls expected: plan + draft.
	if len(llm.prompts) != 2 {
		t.Errorf("expected 2 LLM calls, got %d", len(llm.prompts))
	}
}

func TestLoopValidatesPlannerNamesAgainstRegistry(t *testing.T) {
	// Planner emits an unknown name alongside a valid one. The unknown
	// name must be dropped, the valid one must still run.
	reg := NewRegistry()
	_ = reg.Register(pickedResearcher{name: "git", describe: "git", body: "commit a"})

	llm := &scriptedLLM{
		responses: []string{
			`["git", "github-prs-not-registered"]`,
			"answer",
		},
	}
	loop := NewLoop(reg, llm.Fn()).WithScoreOptions(ScoreOptions{Disabled: true})
	res, err := loop.Run(context.Background(), ResearchQuery{Question: "q"}, DefaultBudget())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Bundle.Len() != 1 {
		t.Errorf("Bundle.Len() = %d, want 1 (only the valid researcher)", res.Bundle.Len())
	}
	if res.Bundle.Items[0].Source != "git" {
		t.Errorf("Bundle source = %q, want git", res.Bundle.Items[0].Source)
	}
}

func TestLoopEmptyPlanShortCircuits(t *testing.T) {
	// Planner emits an empty array. The loop must skip gather and go
	// straight to a draft against an empty bundle. The draft prompt is
	// engineered to make the model say "I don't have enough evidence,"
	// which is exactly the safe behaviour we want when no researcher
	// fits — and the polar opposite of the hallucinated-PRs failure mode.
	reg := NewRegistry()
	_ = reg.Register(pickedResearcher{name: "git", describe: "git"})

	llm := &scriptedLLM{
		responses: []string{
			`[]`,
			"I don't have enough evidence to answer that.",
		},
	}
	loop := NewLoop(reg, llm.Fn()).WithScoreOptions(ScoreOptions{Disabled: true})
	res, err := loop.Run(context.Background(), ResearchQuery{
		Question: "what PRs are open in the elastic/ensemble repo?",
	}, DefaultBudget())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Reason != ReasonUnscored {
		t.Errorf("Reason = %s, want %s", res.Reason, ReasonUnscored)
	}
	if res.Bundle.Len() != 0 {
		t.Errorf("Bundle should be empty, got %d items", res.Bundle.Len())
	}
}

func TestLoopGatherErrorIsPartialNotFatal(t *testing.T) {
	// One researcher errors, the other succeeds. The bundle must contain
	// the successful one and the loop must still produce a draft.
	reg := NewRegistry()
	_ = reg.Register(pickedResearcher{name: "git", describe: "git", err: errors.New("git unavailable")})
	_ = reg.Register(pickedResearcher{name: "esearch", describe: "esearch", body: "hit 1"})

	llm := &scriptedLLM{
		responses: []string{
			`["git", "esearch"]`,
			"answer based on hit 1",
		},
	}
	loop := NewLoop(reg, llm.Fn()).WithScoreOptions(ScoreOptions{Disabled: true})
	res, err := loop.Run(context.Background(), ResearchQuery{Question: "q"}, DefaultBudget())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Bundle.Len() != 1 {
		t.Errorf("Bundle.Len() = %d, want 1 (just esearch)", res.Bundle.Len())
	}
	if res.Bundle.Items[0].Source != "esearch" {
		t.Errorf("Bundle source = %q, want esearch", res.Bundle.Items[0].Source)
	}
}

func TestLoopPlanLLMErrorPropagates(t *testing.T) {
	reg := NewRegistry()
	llm := &scriptedLLM{} // no responses → first call errors
	loop := NewLoop(reg, llm.Fn()).WithScoreOptions(ScoreOptions{Disabled: true})
	_, err := loop.Run(context.Background(), ResearchQuery{Question: "q"}, DefaultBudget())
	if err == nil || !strings.Contains(err.Error(), "plan:") {
		t.Errorf("expected plan stage error, got %v", err)
	}
}

func TestLoopRejectsNilLLM(t *testing.T) {
	loop := NewLoop(NewRegistry(), nil)
	_, err := loop.Run(context.Background(), ResearchQuery{}, DefaultBudget())
	if err == nil {
		t.Fatal("expected error for nil llm, got nil")
	}
}

// TestLoopNegativeExampleRegression is the screenshot regression test. It
// asserts that when the registry has no github-prs researcher and the
// planner returns an empty plan, the loop's draft must NOT contain
// fabricated PR numbers from the model's training data.
//
// The test cannot guarantee what a real LLM would say — it stubs the LLM
// with a compliant response — but it does verify that the draft prompt is
// the one that's *asked*, so that as long as the model honours its
// instructions the failure mode is impossible.
func TestLoopNegativeExampleRegression(t *testing.T) {
	reg := NewRegistry() // intentionally empty: no github-prs researcher

	llm := &scriptedLLM{
		responses: []string{
			`[]`, // planner correctly picks nothing
			"I don't have enough evidence to answer that.",
		},
	}
	loop := NewLoop(reg, llm.Fn()).WithScoreOptions(ScoreOptions{Disabled: true})
	res, err := loop.Run(context.Background(), ResearchQuery{
		Question: "there have been recent updates to the pr's, can you verify their statuses?",
	}, DefaultBudget())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	// The loop must have called the model exactly twice and the draft
	// prompt (call 2) must contain the explicit anti-hallucination rules
	// — specifically the "I don't have enough evidence" required-phrase
	// and the prohibition on inventing identifiers.
	if len(llm.prompts) != 2 {
		t.Fatalf("expected 2 LLM calls, got %d", len(llm.prompts))
	}
	draftPrompt := llm.prompts[1]
	for _, required := range []string{
		"only the evidence",
		"never invent",
		"don't have enough evidence",
		"do not say \"you should run\"",
	} {
		if !strings.Contains(strings.ToLower(draftPrompt), strings.ToLower(required)) {
			t.Errorf("draft prompt missing anti-hallucination rule %q", required)
		}
	}

	// And the planner prompt must forbid suggesting commands.
	planPrompt := llm.prompts[0]
	for _, required := range []string{
		"never suggest commands",
		"never invent",
		"only a json array",
	} {
		if !strings.Contains(strings.ToLower(planPrompt), strings.ToLower(required)) {
			t.Errorf("plan prompt missing anti-suggestion rule %q", required)
		}
	}

	// Sanity: the loop produced an honest non-answer, not a hallucination.
	if !strings.Contains(strings.ToLower(res.Draft), "don't have enough evidence") {
		t.Errorf("draft should be the honest non-answer, got %q", res.Draft)
	}
}

func TestParsePlanTolerantOfPreamble(t *testing.T) {
	cases := map[string]struct {
		raw  string
		want []string
	}{
		"clean":            {`["a","b"]`, []string{"a", "b"}},
		"with preamble":    {"sure thing! [\"a\"]", []string{"a"}},
		"with newlines":    {"\n\n[\"git\", \"esearch\"]\n", []string{"git", "esearch"}},
		"empty":            {`[]`, []string{}},
		"deduplicates":     {`["git", "git", "esearch"]`, []string{"git", "esearch"}},
		"trims whitespace": {`["  git  ", "esearch"]`, []string{"git", "esearch"}},
		// Real qwen2.5:7b output observed in glitch threads smoke:
		// the model emits `[\"git-log\"]` (one layer of backslash
		// escaping) instead of `["git-log"]`. ParsePlan strips the
		// escape and retries.
		"escaped quotes":        {`[\"git-log\"]`, []string{"git-log"}},
		"escaped multi-element": {`[\"git\", \"github-prs\"]`, []string{"git", "github-prs"}},
		// Bare-identifier form: the model treats researcher names
		// as barewords without any quoting. ParsePlan's third
		// tolerance pass lexes them out directly.
		"bare identifier":       {`[git-log]`, []string{"git-log"}},
		"bare multi-element":    {`[git-log, github-prs]`, []string{"git-log", "github-prs"}},
		"bare with prose":       {`Sure, [git-log] is the right pick.`, []string{"git-log"}},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			got, err := ParsePlan(tc.raw)
			if err != nil {
				t.Fatalf("ParsePlan: %v", err)
			}
			if len(got) != len(tc.want) {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
			for i := range tc.want {
				if got[i] != tc.want[i] {
					t.Errorf("got[%d] = %q, want %q", i, got[i], tc.want[i])
				}
			}
		})
	}
}

// TestParsePlanRejectsMalformed covers the "truly shapeless input"
// rejection contract. The parser used to reject anything that wasn't
// strict JSON, but the bareword-tolerance pass now legitimately
// extracts identifier-looking tokens from `["unclosed"` and
// `["a", 1]` — those pass through the parser and get dropped at the
// registry-validation step downstream (where the names don't match
// any registered researcher). The parser's job is now "find
// identifier-shaped tokens between brackets if any exist"; the
// registry is the strict validator.
//
// Only inputs with NO bracket at all (or no token-shaped content
// at all) should still error.
func TestParsePlanRejectsMalformed(t *testing.T) {
	cases := []string{
		"no array here",
		"!!!@@@$$$",
	}
	for _, raw := range cases {
		if _, err := ParsePlan(raw); err == nil {
			t.Errorf("ParsePlan(%q) should have errored", raw)
		}
	}
}
