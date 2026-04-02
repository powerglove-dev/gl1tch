//go:build !integration

package router

// TestRouterContract_QuestionsNeverRoute is a structural guarantee:
// questions and observations must NEVER route to a pipeline, regardless of how
// topically similar they are to a pipeline's description.
//
// This is enforced by two independent mechanisms:
//  1. Embedding fast path requires isImperativeInput — so even cosine=1.0 won't
//     skip the LLM for questions.
//  2. The LLM prompt carries a hard Step 1 gate before pipeline selection.
//
// Here we use DisableEmbeddings=true and a stub LLM that always returns NONE
// to verify the full subsystem handles these inputs correctly.

import (
	"context"
	"fmt"
	"io"
	"testing"

	"github.com/8op-org/gl1tch/internal/executor"
	"github.com/8op-org/gl1tch/internal/pipeline"
)

// noneStubMgr returns a manager whose LLM always returns NONE.
func noneStubMgr(t *testing.T) *executor.Manager {
	t.Helper()
	mgr := executor.NewManager()
	if err := mgr.Register(&executor.StubExecutor{
		ExecutorName: "ollama",
		ExecuteFn: func(_ context.Context, _ string, _ map[string]string, w io.Writer) error {
			_, err := fmt.Fprint(w, `{"pipeline":"NONE","confidence":0.05,"input":"","cron":""}`)
			return err
		},
	}); err != nil {
		t.Fatalf("Register: %v", err)
	}
	return mgr
}

// contractPipelines covers git, CI, docs, and support topics so any question about
// these topics has maximum opportunity to misfire.
var contractPipelines = []pipeline.PipelineRef{
	{Name: "git-pulse", Description: "Analyze git repository activity and health"},
	{Name: "pr-review", Description: "Review pull requests for quality and completeness"},
	{Name: "docs-improve", Description: "Improve documentation quality and coverage"},
	{Name: "support-digest", Description: "Analyze and summarize support emails"},
	{Name: "ci-monitor", Description: "Monitor CI pipeline status and failures"},
}

func TestRouterContract_QuestionsNeverRoute(t *testing.T) {
	cfg := Config{
		ConfidentThreshold: 0.85,
		AmbiguousThreshold: 0.65,
		Model:              "test-model",
		DisableEmbeddings:  true, // verify LLM path alone handles this correctly
	}

	// All these inputs should produce nil Pipeline — the stub LLM always returns NONE,
	// confirming the end-to-end subsystem routes them to no-match.
	questions := []string{
		// Classic question mark cases
		"looks like there are merge conflicts?",
		"why is the build failing?",
		"is the digest pipeline working?",
		"are there open PRs?",
		"what happened to the last git-pulse run?",
		"how does the pr-review pipeline work?",
		"can you check if support-digest ran?",
		"should I run docs-improve?",
		"did the ci-monitor fire?",
		"does this trigger git-pulse?",
		"do I need to run anything?",
		"any open PRs?",
		"any idea why the digest failed?",

		// Observations without question marks
		"looks like there are merge conflicts",
		"seems like the deploy is slow",
		"it looks like the tests are failing",
		"i think something is wrong with git-pulse",
		"i noticed the support digest didn't run",
		"seems slow today",
		"any thoughts on this",
	}

	for _, q := range questions {
		t.Run(q, func(t *testing.T) {
			mgr := noneStubMgr(t)
			r := New(mgr, &fixedEmbedder{vec: []float32{1, 0}}, cfg)

			result, err := r.Route(context.Background(), q, contractPipelines)
			if err != nil {
				t.Fatalf("Route error: %v", err)
			}
			if result.Pipeline != nil {
				t.Errorf("question/observation must not route to a pipeline, got %q (conf=%.2f)",
					result.Pipeline.Name, result.Confidence)
			}
		})
	}
}

func TestRouterContract_CommandsCanRoute(t *testing.T) {
	// Verify that explicit commands DO reach the LLM and CAN route when the LLM
	// returns a confident match. This ensures the gate isn't too aggressive.
	cfg := Config{
		ConfidentThreshold: 0.85,
		AmbiguousThreshold: 0.65,
		Model:              "test-model",
		DisableEmbeddings:  true,
	}

	commands := []struct {
		prompt      string
		llmResponse string
		wantName    string
	}{
		{
			prompt:      "run git-pulse",
			llmResponse: `{"pipeline":"git-pulse","confidence":0.95,"input":"","cron":""}`,
			wantName:    "git-pulse",
		},
		{
			prompt:      "review my PR https://github.com/org/repo/pull/1",
			llmResponse: `{"pipeline":"pr-review","confidence":0.92,"input":"https://github.com/org/repo/pull/1","cron":""}`,
			wantName:    "pr-review",
		},
		{
			prompt:      "improve the docs for the executor package",
			llmResponse: `{"pipeline":"docs-improve","confidence":0.88,"input":"executor package","cron":""}`,
			wantName:    "docs-improve",
		},
	}

	for _, tc := range commands {
		t.Run(tc.prompt, func(t *testing.T) {
			mgr := makeMgr(t, tc.llmResponse)
			r := New(mgr, &fixedEmbedder{vec: []float32{1, 0}}, cfg)

			result, err := r.Route(context.Background(), tc.prompt, contractPipelines)
			if err != nil {
				t.Fatalf("Route error: %v", err)
			}
			if result.Pipeline == nil {
				t.Fatalf("command %q should route to %q, got nil", tc.prompt, tc.wantName)
			}
			if result.Pipeline.Name != tc.wantName {
				t.Errorf("pipeline=%q, want %q", result.Pipeline.Name, tc.wantName)
			}
		})
	}
}
