//go:build integration

package router

// Integration tests that run against a real local Ollama instance.
// These verify the full two-stage routing pipeline end-to-end:
//   - nomic-embed-text for embedding similarity
//   - a local LLM (llama3.2 by default) for classification
//
// Run with:
//   go test ./internal/router/... -tags integration -v -timeout 120s
//
// Requires:
//   - ollama serve (running on localhost:11434)
//   - ollama pull nomic-embed-text
//   - ollama pull llama3.2

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/8op-org/gl1tch/internal/executor"
	"github.com/8op-org/gl1tch/internal/pipeline"
)

// integrationModel is the Ollama model used for LLM classification in tests.
// Override with GLITCH_TEST_MODEL env var.
func integrationModel() string {
	if m := os.Getenv("GLITCH_TEST_MODEL"); m != "" {
		return m
	}
	return "llama3.2"
}

// integrationBaseURL is the Ollama base URL for tests.
func integrationBaseURL() string {
	if u := os.Getenv("OLLAMA_BASE_URL"); u != "" {
		return u
	}
	return "http://localhost:11434"
}

// realManager builds an executor.Manager loading the real ollama sidecar from
// ~/.config/glitch/wrappers/ollama.yaml, matching how buildFullManager works.
func realManager(t *testing.T) *executor.Manager {
	t.Helper()

	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("UserHomeDir: %v", err)
	}
	sidecarPath := home + "/.config/glitch/wrappers/ollama.yaml"

	adapter, err := executor.NewCliAdapterFromSidecar(sidecarPath)
	if err != nil {
		t.Fatalf("load ollama sidecar %s: %v", sidecarPath, err)
	}

	mgr := executor.NewManager()
	if err := mgr.Register(adapter); err != nil {
		t.Fatalf("register ollama: %v", err)
	}
	return mgr
}

// realEmbedder returns a real OllamaEmbedder pointed at local Ollama.
func realEmbedder() *OllamaEmbedder {
	return &OllamaEmbedder{
		BaseURL: integrationBaseURL(),
		Model:   DefaultEmbeddingModel,
	}
}

// integrationPipelines mirrors the real ~/.config/glitch/pipelines/ contents.
var integrationPipelines = []pipeline.PipelineRef{
	{
		Name:        "support-digest-dryrun",
		Description: "Analyze and summarize support emails into a digest doc",
	},
	{
		Name:        "clarify-haiku-multistep",
		Description: "Generate a multistep pipeline using claude haiku with clarifying prompts",
	},
	{
		Name:        "test-glab-after",
		Description: "Fetch and display support email content for glab testing",
	},
}

func integrationRouter(t *testing.T) *HybridRouter {
	t.Helper()
	return New(realManager(t), realEmbedder(), Config{
		Model:         integrationModel(),
		OllamaBaseURL: integrationBaseURL(),
		CacheDir:      t.TempDir(),
	})
}

// ── Embedding fast path ───────────────────────────────────────────────────────

func TestIntegration_Embedding_SupportDigest(t *testing.T) {
	// Prompts that are semantically close to "support-digest-dryrun" should
	// hit the embedding fast path (method=="embedding") with real nomic-embed-text.
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	r := integrationRouter(t)

	cases := []string{
		"now re-run the support-digest pipeline",
		"run the support digest",
		"summarize support emails",
		"analyze acme support emails and make a digest",
	}

	for _, prompt := range cases {
		t.Run(prompt, func(t *testing.T) {
			result, err := r.Route(ctx, prompt, integrationPipelines)
			if err != nil {
				t.Fatalf("Route: %v", err)
			}
			if result.Pipeline == nil {
				t.Fatalf("expected match, got nil (method=%s conf=%.2f)", result.Method, result.Confidence)
			}
			if result.Pipeline.Name != "support-digest-dryrun" {
				t.Errorf("pipeline=%q, want 'support-digest-dryrun' (method=%s conf=%.2f)",
					result.Pipeline.Name, result.Method, result.Confidence)
			}
			t.Logf("method=%s confidence=%.3f", result.Method, result.Confidence)
		})
	}
}

func TestIntegration_Embedding_ClarifyMultistep(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	r := integrationRouter(t)

	cases := []string{
		"generate a multistep pipeline to send a birthday card",
		"build me a clarifying multistep pipeline",
		"create a multistep haiku pipeline",
	}

	for _, prompt := range cases {
		t.Run(prompt, func(t *testing.T) {
			result, err := r.Route(ctx, prompt, integrationPipelines)
			if err != nil {
				t.Fatalf("Route: %v", err)
			}
			if result.Pipeline == nil {
				t.Fatalf("expected match, got nil (method=%s conf=%.2f)", result.Method, result.Confidence)
			}
			if result.Pipeline.Name != "clarify-haiku-multistep" {
				t.Errorf("pipeline=%q, want 'clarify-haiku-multistep' (method=%s conf=%.2f)",
					result.Pipeline.Name, result.Method, result.Confidence)
			}
			t.Logf("method=%s confidence=%.3f", result.Method, result.Confidence)
		})
	}
}

// ── LLM classification path ───────────────────────────────────────────────────

func TestIntegration_LLM_CronExtraction(t *testing.T) {
	// Prompts with explicit scheduling language should produce a CronExpr.
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	r := integrationRouter(t)

	cases := []struct {
		prompt   string
		wantCron string // expected 5-field expression
	}{
		{"run the support digest every morning at 9am", "0 9 * * *"},
		{"run the support digest every weekday", "0 9 * * 1-5"},
		{"summarize support emails every 2 hours", "0 */2 * * *"},
		{"run the support digest every day at midnight", "0 0 * * *"},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.prompt, func(t *testing.T) {
			result, err := r.Route(ctx, tc.prompt, integrationPipelines)
			if err != nil {
				t.Fatalf("Route: %v", err)
			}
			t.Logf("method=%s pipeline=%v conf=%.3f cron=%q input=%q",
				result.Method,
				func() string {
					if result.Pipeline != nil {
						return result.Pipeline.Name
					}
					return "nil"
				}(),
				result.Confidence,
				result.CronExpr,
				result.Input,
			)
			if result.CronExpr == "" {
				t.Errorf("expected non-empty CronExpr for %q, got empty", tc.prompt)
			}
			if result.CronExpr != tc.wantCron {
				// Log as warning — LLMs may produce equivalent but differently spelled expressions
				t.Logf("WARN: cron=%q, want %q (may still be semantically correct)", result.CronExpr, tc.wantCron)
			}
		})
	}
}

func TestIntegration_LLM_InputExtraction(t *testing.T) {
	// Prompts with a specific focus should populate result.Input.
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	r := integrationRouter(t)

	cases := []struct {
		prompt    string
		wantInput string // substring that should appear in result.Input
	}{
		{"run the support digest for acme", "acme"},
		{"summarize support emails about billing", "billing"},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.prompt, func(t *testing.T) {
			result, err := r.Route(ctx, tc.prompt, integrationPipelines)
			if err != nil {
				t.Fatalf("Route: %v", err)
			}
			t.Logf("method=%s pipeline=%v conf=%.3f input=%q",
				result.Method,
				func() string {
					if result.Pipeline != nil {
						return result.Pipeline.Name
					}
					return "nil"
				}(),
				result.Confidence,
				result.Input,
			)
			if result.Input == "" {
				t.Errorf("expected non-empty Input for %q, got empty", tc.prompt)
			}
		})
	}
}

// ── No-match cases ────────────────────────────────────────────────────────────

func TestIntegration_NoMatch_GitOps(t *testing.T) {
	// Git operations should never match any of the real pipelines.
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	r := integrationRouter(t)

	// Real prompts from mar30/mar31 archives
	cases := []string{
		"commit and push",
		"comit and push it all up",
		"COMIT AN DPUSH DUMBASS",
		"merge into main",
		"push everything up",
	}

	for _, prompt := range cases {
		t.Run(prompt, func(t *testing.T) {
			result, err := r.Route(ctx, prompt, integrationPipelines)
			if err != nil {
				t.Fatalf("Route: %v", err)
			}
			t.Logf("method=%s pipeline=%v conf=%.3f",
				result.Method,
				func() string {
					if result.Pipeline != nil {
						return result.Pipeline.Name
					}
					return "nil"
				}(),
				result.Confidence,
			)
			if result.Pipeline != nil {
				t.Errorf("expected no match for git op %q, got %q (conf=%.2f)",
					prompt, result.Pipeline.Name, result.Confidence)
			}
		})
	}
}

func TestIntegration_NoMatch_UIComplaints(t *testing.T) {
	// Theme and UI complaints from the archives should not route to any pipeline.
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	r := integrationRouter(t)

	cases := []string{
		"switch to dracula theme",
		"the cron tui still shows the previous theme when i switch it in switch board",
		"nice now set a default theme, i like nord theme, then add gruvbox, dracula, borland",
		"when quitting from cron window it closes the window and switches back to deck",
	}

	for _, prompt := range cases {
		t.Run(prompt, func(t *testing.T) {
			result, err := r.Route(ctx, prompt, integrationPipelines)
			if err != nil {
				t.Fatalf("Route: %v", err)
			}
			t.Logf("method=%s pipeline=%v conf=%.3f",
				result.Method,
				func() string {
					if result.Pipeline != nil {
						return result.Pipeline.Name
					}
					return "nil"
				}(),
				result.Confidence,
			)
			if result.Pipeline != nil {
				t.Errorf("expected no match for UI complaint %q, got %q (conf=%.2f)",
					prompt, result.Pipeline.Name, result.Confidence)
			}
		})
	}
}

func TestIntegration_NoMatch_AmbiguousShort(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	r := integrationRouter(t)

	cases := []string{
		"run it",
		"run it for me",
		"you run it",
		"still does the same thing",
		"works i tested it",
	}

	for _, prompt := range cases {
		t.Run(prompt, func(t *testing.T) {
			result, err := r.Route(ctx, prompt, integrationPipelines)
			if err != nil {
				t.Fatalf("Route: %v", err)
			}
			t.Logf("method=%s pipeline=%v conf=%.3f",
				result.Method,
				func() string {
					if result.Pipeline != nil {
						return result.Pipeline.Name
					}
					return "nil"
				}(),
				result.Confidence,
			)
			if result.Pipeline != nil {
				t.Errorf("expected no match for ambiguous prompt %q, got %q (conf=%.2f)",
					prompt, result.Pipeline.Name, result.Confidence)
			}
		})
	}
}

// ── PR review — no phantom routing ───────────────────────────────────────────

func TestIntegration_NoMatch_PRReview(t *testing.T) {
	// PR review prompts must not silently route to an unrelated pipeline.
	// These contain pipeline-adjacent language ("run", "fix", "update") but
	// are about reviewing/editing code, not running a registered data pipeline.
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	r := integrationRouter(t)

	cases := []string{
		"review the feedback on PR 1246 in elastic/ensemble",
		"what does the reviewer want changed in my PR",
		"fix the review comments on my ensemble pull request",
		"address the code review feedback in ensemble",
		"update scripts/cloud.py to use Command instead of subprocess",
	}

	for _, prompt := range cases {
		t.Run(prompt, func(t *testing.T) {
			result, err := r.Route(ctx, prompt, integrationPipelines)
			if err != nil {
				t.Fatalf("Route: %v", err)
			}
			t.Logf("method=%s pipeline=%v conf=%.3f",
				result.Method,
				func() string {
					if result.Pipeline != nil {
						return result.Pipeline.Name
					}
					return "nil"
				}(),
				result.Confidence,
			)
			if result.Pipeline != nil {
				t.Errorf("PR review prompt %q must not route to pipeline %q (conf=%.2f) — no PR pipeline registered",
					prompt, result.Pipeline.Name, result.Confidence)
			}
		})
	}
}

// ── Confidence logging ────────────────────────────────────────────────────────

func TestIntegration_ConfidenceReport(t *testing.T) {
	// Not a pass/fail test — logs the actual confidence scores produced by the
	// real models for every routing scenario so thresholds can be tuned.
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	r := integrationRouter(t)

	type scenario struct {
		prompt       string
		wantMatch    bool
		wantPipeline string
	}

	scenarios := []scenario{
		// strong matches
		{"now re-run the support-digest pipeline", true, "support-digest-dryrun"},
		{"run the support digest", true, "support-digest-dryrun"},
		{"summarize support emails", true, "support-digest-dryrun"},
		{"generate a multistep pipeline to send a get well card", true, "clarify-haiku-multistep"},

		// should not match
		{"commit and push", false, ""},
		{"switch to dracula theme", false, ""},
		{"run it", false, ""},
		{"comit and push it all up", false, ""},
		{"review the feedback on PR 1246 in elastic/ensemble", false, ""},
		{"fix the review comments on my ensemble pull request", false, ""},
	}

	t.Log("\n  prompt                                                    pipeline                  conf   method")
	t.Log("  " + string(make([]byte, 100)))

	for _, sc := range scenarios {
		result, err := r.Route(ctx, sc.prompt, integrationPipelines)
		if err != nil {
			t.Errorf("Route(%q): %v", sc.prompt, err)
			continue
		}
		name := "nil"
		if result.Pipeline != nil {
			name = result.Pipeline.Name
		}
		pass := "✓"
		if sc.wantMatch && result.Pipeline == nil {
			pass = "✗ expected match"
		} else if !sc.wantMatch && result.Pipeline != nil {
			pass = "✗ unexpected match"
		}
		t.Logf("  %s  %-52s → %-26s %.3f  %s",
			pass, sc.prompt, name, result.Confidence, result.Method)
	}
}
