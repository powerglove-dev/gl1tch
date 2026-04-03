//go:build !integration

package router

// TestRouter_RealWorldPrompts uses actual prompts extracted from session archives
// (~/Projects/prompts/orcai/2026-03-30.md, 2026-03-31.md) paired with the real
// pipeline names from ~/.config/glitch/pipelines/ to verify routing behavior.
//
// Each case documents: the real prompt text, what the LLM stub returns, and
// what the router is expected to do with it.

import (
	"context"
	"testing"

	"github.com/8op-org/gl1tch/internal/pipeline"
)

// realPipelines mirrors the actual pipelines in ~/.config/glitch/pipelines/
// (names and descriptions as gl1tch would discover them).
var realPipelines = []pipeline.PipelineRef{
	{
		Name:        "support-digest-dryrun",
		Description: "Analyze and summarize support emails into a digest doc",
		Path:        "/Users/stokes/.config/glitch/pipelines/support-digest-dryrun.pipeline.yaml",
	},
	{
		Name:        "clarify-haiku-multistep",
		Description: "Generate a multistep pipeline using claude haiku with clarifying prompts",
		Path:        "/Users/stokes/.config/glitch/pipelines/clarify-haiku-multistep.pipeline.yaml",
	},
	{
		Name:        "test-glab-after",
		Description: "Fetch and display support email content for glab testing",
		Path:        "/Users/stokes/.config/glitch/pipelines/test-glab-after.pipeline.yaml",
	},
}

func TestRouter_RealWorldPrompts_LLMPath(t *testing.T) {
	// All cases disable embeddings so the LLM stub is the only classifier.
	// The stub returns a fixed JSON response per case — this lets us verify
	// that the router's extract/validate/sanitize logic works on real prompts.

	cases := []struct {
		// Real prompt from session archive
		prompt string
		// What the LLM returns (crafted to match what a well-prompted model would say)
		llmJSON string
		// Expected routing outcome
		wantPipeline string // "" means no match
		wantInput    string
		wantCron     string
		wantMethod   string
	}{
		{
			// mar30 prompt #43: explicitly names the pipeline — starts with "re-run "
			prompt:       "re-run the support-digest pipeline",
			llmJSON:      `{"pipeline":"support-digest-dryrun","confidence":0.91,"input":"","cron":""}`,
			wantPipeline: "support-digest-dryrun",
			wantMethod:   "llm",
		},
		{
			// mar30 prompt: re-run with a focus extracted
			prompt:       "run the support digest for acme",
			llmJSON:      `{"pipeline":"support-digest-dryrun","confidence":0.88,"input":"acme","cron":""}`,
			wantPipeline: "support-digest-dryrun",
			wantInput:    "acme",
			wantMethod:   "llm",
		},
		{
			// mar30 prompt: scheduling intent — LLM extracts cron
			prompt:       "run the support digest every morning at 9am",
			llmJSON:      `{"pipeline":"support-digest-dryrun","confidence":0.87,"input":"","cron":"0 9 * * *"}`,
			wantPipeline: "support-digest-dryrun",
			wantCron:     "0 9 * * *",
			wantMethod:   "llm",
		},
		{
			// mar30 prompt: "run it for me" — too ambiguous, no confident match
			prompt:       "run it for me",
			llmJSON:      `{"pipeline":"NONE","confidence":0.0,"input":"","cron":""}`,
			wantPipeline: "",
			wantMethod:   "llm",
		},
		{
			// mar31 prompt: git operation — LLM returns NONE (not an explicit pipeline invocation)
			prompt:       "commit and push",
			llmJSON:      `{"pipeline":"NONE","confidence":0.0,"input":"","cron":""}`,
			wantPipeline: "",
			wantMethod:   "llm",
		},
		{
			// mar31 prompt: git operation, typo variant — LLM returns NONE
			prompt:       "comit and push it all up",
			llmJSON:      `{"pipeline":"NONE","confidence":0.0,"input":"","cron":""}`,
			wantPipeline: "",
			wantMethod:   "llm",
		},
		{
			// mar30 prompt: state cleanup — LLM returns NONE (below AmbiguousThreshold)
			prompt:       "clean my state again",
			llmJSON:      `{"pipeline":"NONE","confidence":0.45,"input":"","cron":""}`,
			wantPipeline: "",
			wantMethod:   "llm",
		},
		{
			// mar31 prompt: ambiguous short query
			prompt:       "run it",
			llmJSON:      `{"pipeline":"support-digest-dryrun","confidence":0.55,"input":"","cron":""}`,
			wantPipeline: "", // 0.55 < 0.65 → rejected even though it named a pipeline
			wantMethod:   "llm",
		},
		{
			// mar31 prompt: compound with cron ("daily" → midnight every day)
			prompt:       "run the support digest every day and keep me updated",
			llmJSON:      `{"pipeline":"support-digest-dryrun","confidence":0.86,"input":"","cron":"0 0 * * *"}`,
			wantPipeline: "support-digest-dryrun",
			wantCron:     "0 0 * * *",
			wantMethod:   "llm",
		},
		{
			// mar30 prompt: LLM hallucinates a pipeline not in the list
			prompt:       "run the git push pipeline",
			llmJSON:      `{"pipeline":"git-push","confidence":0.90,"input":"","cron":""}`,
			wantPipeline: "", // "git-push" not in realPipelines → hallucination → nil
			wantMethod:   "llm",
		},
		{
			// mar31 prompt: "generate" is not an explicit invocation verb; real LLM returns NONE
			prompt:       "generate a multistep pipeline to send a birthday card",
			llmJSON:      `{"pipeline":"NONE","confidence":0.05,"input":"","cron":""}`,
			wantPipeline: "",
			wantMethod:   "llm",
		},
		{
			// "summarize" is not an explicit invocation verb; real LLM returns NONE
			prompt:       "summarize support emails every 2 hours",
			llmJSON:      `{"pipeline":"NONE","confidence":0.05,"input":"","cron":""}`,
			wantPipeline: "",
			wantMethod:   "llm",
		},
		{
			// Invalid cron from LLM (4 fields) — validateCron rejects it → empty
			prompt:       "run the digest every hour",
			llmJSON:      `{"pipeline":"support-digest-dryrun","confidence":0.85,"input":"","cron":"0 * * *"}`,
			wantPipeline: "support-digest-dryrun",
			wantCron:     "", // 4-field cron rejected
			wantMethod:   "llm",
		},
	}

	for _, tc := range cases {
		t.Run(tc.prompt, func(t *testing.T) {
			resp := tc.llmJSON
			mgr := makeMgr(t, resp)

			cfg := Config{
				ConfidentThreshold: 0.85,
				AmbiguousThreshold: 0.65,
				Model:              "test-model",
				DisableEmbeddings:  true, // exercise LLM stage directly
			}
			r := New(mgr, &fixedEmbedder{vec: []float32{1, 0}}, cfg)

			result, err := r.Route(context.Background(), tc.prompt, realPipelines)
			if err != nil {
				t.Fatalf("Route error: %v", err)
			}

			if tc.wantPipeline == "" {
				if result.Pipeline != nil {
					t.Errorf("expected no match, got %q", result.Pipeline.Name)
				}
			} else {
				if result.Pipeline == nil {
					t.Fatalf("expected match %q, got nil", tc.wantPipeline)
				}
				if result.Pipeline.Name != tc.wantPipeline {
					t.Errorf("pipeline = %q, want %q", result.Pipeline.Name, tc.wantPipeline)
				}
			}

			if result.Method != tc.wantMethod {
				t.Errorf("method = %q, want %q", result.Method, tc.wantMethod)
			}
			if result.Input != tc.wantInput {
				t.Errorf("input = %q, want %q", result.Input, tc.wantInput)
			}
			if result.CronExpr != tc.wantCron {
				t.Errorf("cron = %q, want %q", result.CronExpr, tc.wantCron)
			}
		})
	}
}

func TestRouter_RealWorldPrompts_EmbeddingPath(t *testing.T) {
	// Verify that high-similarity prompts bypass the LLM entirely.
	// Uses real pipeline descriptions — the query vector is crafted to be
	// close to "support-digest-dryrun".

	mgr := makeMgr(t, "")
	_ = mgr // override below

	// We'll use funcEmbedder where the query is identical to support-digest-dryrun's
	// description vector — simulating nomic-embed-text returning similar embeddings
	// for semantically close text.
	descVecs := map[string][]float32{
		"Analyze and summarize support emails into a digest doc":      {1, 0, 0},
		"Generate a multistep pipeline using claude haiku with clarifying prompts": {0, 1, 0},
		"Fetch and display support email content for glab testing":                 {0, 0, 1},
	}

	// Real session prompts that are semantically close to support-digest-dryrun
	// AND start with an explicit pipeline-invocation verb (required for fast path).
	prompts := []struct {
		text string
		vec  []float32 // simulated embedding
	}{
		// Explicit re-run invocation — fast path eligible
		{"re-run the support-digest pipeline", []float32{0.99, 0.05, 0.05}},
		// "run the support digest for acme" — close
		{"run the support digest for acme", []float32{0.97, 0.1, 0.05}},
		// Explicit trigger invocation — fast path eligible
		{"trigger the support-digest-dryrun pipeline", []float32{0.96, 0.05, 0.1}},
	}

	for _, p := range prompts {
		t.Run(p.text, func(t *testing.T) {
			queryVec := p.vec
			embedFn := func(text string) []float32 {
				if v, ok := descVecs[text]; ok {
					return v
				}
				return queryVec
			}

			cfg := Config{
				ConfidentThreshold: 0.85,
				AmbiguousThreshold: 0.65,
				Model:              "test-model",
			}
			r := New(makeMgr(t, `{"pipeline":"NONE","confidence":0.0,"input":"","cron":""}`), &funcEmbedder{fn: embedFn}, cfg)

			result, err := r.Route(context.Background(), p.text, realPipelines)
			if err != nil {
				t.Fatalf("Route error: %v", err)
			}

			if result.Pipeline == nil {
				t.Fatalf("expected support-digest-dryrun match, got nil (cosine too low?)")
			}
			if result.Pipeline.Name != "support-digest-dryrun" {
				t.Errorf("expected support-digest-dryrun, got %q", result.Pipeline.Name)
			}
			if result.Method != "embedding" {
				t.Errorf("method = %q, want 'embedding' — LLM should not be called for close matches", result.Method)
			}
		})
	}
}
