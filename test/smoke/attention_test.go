//go:build smoke

// attention_test.go drives the AI-first attention + deep-analysis
// ladder end-to-end against a local Ollama and a local opencode
// install, with no Elasticsearch, no collector loop, and no desktop.
// It's the programmatic counterpart to the `glitch attention` CLI:
// the CLI exists so a human can poke one event at a time, and this
// test exists so CI can assert the same pipeline on a known-shape
// synthetic event without needing gh auth or network access.
//
// What it proves:
//
//  1. LoadResearchPrompt resolves the bundled default so a fresh
//     workspace can feed the classifier from day one.
//  2. ClassifyAttention returns a per-event verdict against a live
//     local qwen2.5:7b — not mocked, so regressions in the prompt
//     template or the JSON schema surface here before they hit
//     users.
//  3. AnalyzeOne produces a non-empty artifact for a high-attention
//     event when opencode is installed, exercising the real
//     artifact-mode rubric substitution.
//
// The suite is guarded by the `smoke` build tag (same as
// ask_workspaces_test.go) so `go test ./...` never drags it in.
// Missing dependencies skip the relevant case rather than failing
// so the suite is safe as an informational CI stage.
package smoke

import (
	"context"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/8op-org/gl1tch/pkg/glitchd"
)

// attentionTestWorkspace is the fake workspace id the attention
// smoke cases use. It does not need to exist on disk — the loader
// falls through to the bundled default when no per-workspace file
// is present, which is exactly the "fresh install" path we want to
// validate.
const attentionTestWorkspace = "smoke-attention"

// TestSmokeAttention_LoadResearchPromptBundled proves the research
// prompt loader's fallback chain works end-to-end: no per-workspace
// file, no global override, the bundled default must be returned
// non-empty. Regressions in the prompts directory search path have
// historically been silent (LoadPrompt returns an error, callers
// fall through to "disabled") so an explicit assertion here is
// cheap insurance.
func TestSmokeAttention_LoadResearchPromptBundled(t *testing.T) {
	prompt, err := glitchd.LoadResearchPrompt(attentionTestWorkspace)
	if err != nil {
		t.Fatalf("LoadResearchPrompt: %v", err)
	}
	if strings.TrimSpace(prompt) == "" {
		t.Fatal("research prompt empty — bundled default not found")
	}
	// The bundled default contains a literal "high attention"
	// heading as of 2026-04-08. Checking for it is a tripwire for
	// someone accidentally replacing the template with an empty
	// file or a placeholder.
	if !strings.Contains(strings.ToLower(prompt), "attention") {
		t.Errorf("bundled default should mention attention rules, got:\n%s", prompt)
	}
}

// TestSmokeAttention_ClassifyReviewOnMyPR exercises the classifier
// against a synthetic but realistic event: a human reviewer
// commenting on the test user's PR. The smoke assertion is loose —
// we accept `high` OR `normal`, because qwen2.5:7b is not a
// deterministic oracle — but we DO reject `low`, because
// classifying a direct PR review as low is a clear regression in
// either the classifier prompt or the model's ability to follow it.
//
// The test is deliberately designed to be a strong positive signal
// for the research prompt's "review on my PR" rule. If qwen marks
// this low, the default research template needs sharpening.
func TestSmokeAttention_ClassifyReviewOnMyPR(t *testing.T) {
	requireOllama(t)
	requireModel(t, "qwen2.5:7b")

	// Warm the model so the call doesn't eat the cold-load budget
	// inside a single t.Run. Same pattern as the ask smoke suite.
	warmupOllama(t, "qwen2.5:7b")

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	events := []glitchd.AnalyzableEvent{
		{
			Source: "github",
			Type:   "github.pr_review",
			Repo:   "elastic/ensemble",
			Author: "someone-else",
			Title:  "#1246 Review by @someone-else: COMMENTED",
			// Body is written in the voice of a real reviewer so
			// the classifier has something concrete to anchor on.
			// Referencing "your PR" and a specific file name helps
			// qwen recognize the "review on my PR" rule from the
			// research prompt.
			Body: "I left some comments on your PR. In cloud/update_stack_versions.py, " +
				"the retry loop on line 42 looks off — shouldn't we bail after 3 attempts? " +
				"Also the error message doesn't include the stack name, which makes the " +
				"failure mode hard to debug in prod. Happy to discuss.",
			Timestamp: time.Now().Add(-5 * time.Minute),
		},
	}

	verdicts, err := glitchd.ClassifyAttention(ctx, events, attentionTestWorkspace)
	if err != nil {
		t.Fatalf("ClassifyAttention: %v", err)
	}
	if len(verdicts) != 1 {
		t.Fatalf("want 1 verdict, got %d", len(verdicts))
	}
	v := verdicts[0]
	t.Logf("verdict: level=%q reason=%q", v.Level, v.Reason)

	switch v.Level {
	case glitchd.AttentionHigh, glitchd.AttentionNormal:
		// acceptable — high is the ideal outcome, normal is
		// tolerable when the classifier is cautious.
	case glitchd.AttentionLow:
		t.Errorf("reviewer comment on user PR classified as low — "+
			"research prompt or classifier prompt needs tuning: %q", v.Reason)
	default:
		t.Errorf("unrecognized attention level: %q", v.Level)
	}
	if strings.TrimSpace(v.Reason) == "" {
		t.Error("verdict should come with a reason")
	}
}

// TestSmokeAttention_AnalyzeArtifactMode drives the full ladder —
// classifier + AnalyzeOne — against the same reviewer-comment event
// with --force-high semantics applied directly (Attention stamped
// high on the event before AnalyzeOne). This isolates the artifact
// template from the classifier so a regression in one does not
// mask a regression in the other.
//
// Requires both Ollama (for the coder model opencode wraps) and
// the `opencode` binary on PATH. Skips cleanly when either is
// missing so a dev box without opencode still passes the rest of
// the suite.
//
// Assertion is deliberately loose: we require non-empty markdown
// and presence of at least one of the section headers the
// artifact template asks the model to produce (TL;DR / Artifact /
// How I got here). Exact wording of the artifact is the model's
// call.
func TestSmokeAttention_AnalyzeArtifactMode(t *testing.T) {
	requireOllama(t)
	requireModel(t, "qwen2.5:7b")
	requireOpencode(t)

	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Minute)
	defer cancel()

	ev := glitchd.AnalyzableEvent{
		Source:          "github",
		Type:            "github.pr_review",
		Repo:            "elastic/ensemble",
		Author:          "someone-else",
		Title:           "#1246 Review by @someone-else: COMMENTED",
		Body:            "Your retry loop on line 42 keeps trying forever. Can you cap it at 3 attempts?",
		Identifier:      "1246",
		URL:             "https://github.com/elastic/ensemble/pull/1246",
		WorkspaceID:     attentionTestWorkspace,
		Timestamp:       time.Now().Add(-5 * time.Minute),
		Attention:       glitchd.AttentionHigh,
		AttentionReason: "smoke: forced high to isolate artifact template",
	}

	// cfg=nil is fine: AnalyzeOne falls back to the bundled-in
	// default analysis model when the config is nil, which is the
	// same path a fresh install hits.
	result := glitchd.AnalyzeOne(ctx, ev, nil)
	t.Logf("model=%s exit=%d duration=%s", result.Model, result.ExitCode, result.Duration)

	if strings.TrimSpace(result.Markdown) == "" {
		t.Fatalf("expected non-empty markdown artifact, got empty (exit=%d)", result.ExitCode)
	}

	// The artifact template asks for at least three sections. We
	// accept partial compliance — a small model might skip one —
	// but at least one of them must be present, otherwise the
	// template substitution clearly failed or the model ignored
	// the rubric entirely.
	found := false
	for _, needle := range []string{"TL;DR", "Artifact", "How I got here", "### "} {
		if strings.Contains(result.Markdown, needle) {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("artifact output has no recognizable section headers — "+
			"template substitution or model instruction-following broke.\n\n%s",
			result.Markdown)
	}
}

// requireOpencode skips when the opencode binary is not on PATH.
// Same pattern as requireModel so a machine without the tool-using
// agent stack still runs the classifier-only cases above.
func requireOpencode(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("opencode"); err != nil {
		t.Skipf("opencode not installed: %v", err)
	}
	// Quick liveness check — an installed-but-broken opencode
	// (missing config, unreachable provider) will fail the real
	// test with a confusing error, so probe its --version here so
	// the skip message is actionable.
	cmd := exec.Command("opencode", "--version")
	if err := cmd.Run(); err != nil {
		t.Skipf("opencode --version failed: %v", err)
	}
}

