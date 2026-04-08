package glitchd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// seedArtifactTemplate writes a minimal deep_analysis_artifact.md
// into the override prompts dir so buildAnalysisPrompt's
// RenderPrompt call can find the template in isolated tests.
func seedArtifactTemplate(t *testing.T, promptsDir string) {
	t.Helper()
	tmpl := strings.Join([]string{
		"ARTIFACT MODE",
		"research={{RESEARCH_PROMPT}}",
		"source={{SOURCE}} type={{TYPE}} repo={{REPO}}",
		"author={{AUTHOR}} id={{IDENTIFIER}} url={{URL}}",
		"title={{TITLE}}",
		"reason={{ATTENTION_REASON}}",
		"body=<<{{BODY}}>>",
		"",
	}, "\n")
	path := filepath.Join(promptsDir, "deep_analysis_artifact.md")
	if err := os.WriteFile(path, []byte(tmpl), 0o644); err != nil {
		t.Fatalf("seed artifact template: %v", err)
	}
	ResetPromptCache()
}

func TestBuildAnalysisPrompt_NormalAttentionUsesSummary(t *testing.T) {
	_, _ = withIsolatedPromptEnv(t)
	ev := AnalyzableEvent{
		Source:    "github",
		Type:      "github.pr",
		Repo:      "elastic/ensemble",
		Author:    "someone",
		Title:     "PR #42",
		Body:      "hello",
		Attention: AttentionNormal,
	}
	got := buildAnalysisPrompt(ev)
	// Summary rubric markers.
	if !strings.Contains(got, "### What it is") {
		t.Errorf("normal attention should use summary rubric with '### What it is', got:\n%s", got)
	}
	if strings.Contains(got, "ARTIFACT MODE") {
		t.Errorf("normal attention should NOT pull in the artifact template")
	}
}

func TestBuildAnalysisPrompt_EmptyAttentionUsesSummary(t *testing.T) {
	// Classifier skipped entirely (analysis disabled in config).
	// Should behave identically to normal attention.
	_, _ = withIsolatedPromptEnv(t)
	ev := AnalyzableEvent{
		Source: "github",
		Type:   "github.pr",
		Title:  "PR #42",
		Body:   "hello",
	}
	got := buildAnalysisPrompt(ev)
	if !strings.Contains(got, "### What it is") {
		t.Errorf("empty attention should use summary rubric, got:\n%s", got)
	}
}

func TestBuildAnalysisPrompt_HighAttentionUsesArtifact(t *testing.T) {
	_, promptsDir := withIsolatedPromptEnv(t)
	seedArtifactTemplate(t, promptsDir)

	ev := AnalyzableEvent{
		Source:          "github",
		Type:            "github.pr",
		Repo:            "elastic/ensemble",
		Author:          "amannocci",
		Title:           "#1246 feat: add cloud:update-stack-versions",
		Body:            "review body",
		Identifier:      "1246",
		URL:             "https://github.com/elastic/ensemble/pull/1246",
		Attention:       AttentionHigh,
		AttentionReason: "review on your PR",
		WorkspaceID:     "ws-test",
	}

	got := buildAnalysisPrompt(ev)

	// Template was substituted.
	if !strings.Contains(got, "ARTIFACT MODE") {
		t.Errorf("high attention should use artifact template, got:\n%s", got)
	}
	// Research prompt was spliced in (bundled default contains
	// "bundled default" per withIsolatedPromptEnv).
	if !strings.Contains(got, "bundled default") {
		t.Errorf("artifact prompt should splice in research prompt, got:\n%s", got)
	}
	// Event fields were substituted.
	for _, needle := range []string{
		"source=github",
		"type=github.pr",
		"repo=elastic/ensemble",
		"author=amannocci",
		"id=1246",
		"title=#1246 feat: add cloud:update-stack-versions",
		"reason=review on your PR",
	} {
		if !strings.Contains(got, needle) {
			t.Errorf("artifact prompt missing %q, got:\n%s", needle, got)
		}
	}
	// No leftover placeholders — unknown placeholders are
	// intentional per RenderPrompt's design, but every placeholder
	// in this template should have been filled.
	if strings.Contains(got, "{{RESEARCH_PROMPT}}") ||
		strings.Contains(got, "{{SOURCE}}") ||
		strings.Contains(got, "{{ATTENTION_REASON}}") {
		t.Errorf("artifact prompt has unfilled placeholders, got:\n%s", got)
	}
}

// Note: the "artifact template missing → fall back to summary"
// branch is not unit-tested here because promptSearchDirs walks
// upward from cwd to find the repo's bundled prompts, so any test
// running inside the repo will always locate the real artifact
// template. The fallback is a single slog.Warn + return call that
// is trivial to audit by inspection; a meaningful test would
// require either destructive filesystem manipulation or a new
// injection seam, neither of which is worth the complexity for a
// one-liner safety net.
