package router

import (
	"context"
	"fmt"
	"io"
	"strings"
	"testing"

	"github.com/8op-org/gl1tch/internal/executor"
	"github.com/8op-org/gl1tch/internal/pipeline"
)

// ── parseClassifyResponse ─────────────────────────────────────────────────────

func TestParseClassifyResponse_ValidJSON(t *testing.T) {
	raw := `{"pipeline":"git-push","confidence":0.91,"input":"main branch","cron":""}`
	resp, err := parseClassifyResponse(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Pipeline != "git-push" {
		t.Errorf("pipeline: want %q, got %q", "git-push", resp.Pipeline)
	}
	if resp.Confidence != 0.91 {
		t.Errorf("confidence: want 0.91, got %f", resp.Confidence)
	}
	if resp.Input != "main branch" {
		t.Errorf("input: want %q, got %q", "main branch", resp.Input)
	}
}

func TestParseClassifyResponse_JSONEmbeddedInText(t *testing.T) {
	// Model adds explanation text around the JSON
	raw := `Sure! Here is my answer:
{"pipeline":"docs-improve","confidence":0.88,"input":"","cron":"0 * * * *"}
That's my best guess.`
	resp, err := parseClassifyResponse(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Pipeline != "docs-improve" {
		t.Errorf("pipeline: want %q, got %q", "docs-improve", resp.Pipeline)
	}
	if resp.Cron != "0 * * * *" {
		t.Errorf("cron: want %q, got %q", "0 * * * *", resp.Cron)
	}
}

func TestParseClassifyResponse_MalformedJSON(t *testing.T) {
	_, err := parseClassifyResponse("not json at all")
	if err == nil {
		t.Error("expected error for malformed JSON")
	}
}

func TestParseClassifyResponse_ClampsConfidence(t *testing.T) {
	cases := []struct {
		name  string
		raw   string
		want  float64
	}{
		{"above 1", `{"pipeline":"x","confidence":1.5,"input":"","cron":""}`, 1.0},
		{"below 0", `{"pipeline":"x","confidence":-0.3,"input":"","cron":""}`, 0.0},
		{"in range", `{"pipeline":"x","confidence":0.7,"input":"","cron":""}`, 0.7},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resp, err := parseClassifyResponse(tc.raw)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if resp.Confidence != tc.want {
				t.Errorf("confidence: want %f, got %f", tc.want, resp.Confidence)
			}
		})
	}
}

// ── validateCron ──────────────────────────────────────────────────────────────

func TestValidateCron_ValidFiveField(t *testing.T) {
	got := validateCron("0 * * * *")
	if got != "0 * * * *" {
		t.Errorf("want %q, got %q", "0 * * * *", got)
	}
}

func TestValidateCron_InvalidFieldCount(t *testing.T) {
	cases := []string{"* * * *", "* * * * * *", "", "not-cron"}
	for _, c := range cases {
		t.Run(c, func(t *testing.T) {
			got := validateCron(c)
			if got != "" {
				t.Errorf("want empty string for %q, got %q", c, got)
			}
		})
	}
}

func TestValidateCron_NONE(t *testing.T) {
	got := validateCron("NONE")
	if got != "" {
		t.Errorf("want empty for NONE, got %q", got)
	}
}

// ── sanitizeFocus ─────────────────────────────────────────────────────────────

func TestSanitizeFocus_StripsPunctuation(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{`"themes documentation"`, "themes documentation"},
		{`executor docs.`, "executor docs"},
		{`'my topic'`, "my topic"},
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			got := sanitizeFocus(tc.in)
			if got != tc.want {
				t.Errorf("want %q, got %q", tc.want, got)
			}
		})
	}
}

func TestSanitizeFocus_NONE(t *testing.T) {
	cases := []string{"NONE", "none", "None", ""}
	for _, c := range cases {
		t.Run(c, func(t *testing.T) {
			got := sanitizeFocus(c)
			if got != "" {
				t.Errorf("want empty for %q, got %q", c, got)
			}
		})
	}
}

// ── buildPrompt ───────────────────────────────────────────────────────────────

func TestBuildPrompt_ContainsPipelines(t *testing.T) {
	pipelines := []pipeline.PipelineRef{
		{Name: "git-push", Description: "Push changes to git"},
		{Name: "docs-improve", Description: "Improve documentation quality"},
	}
	prompt := buildPrompt("help me push my code", pipelines)
	for _, p := range pipelines {
		if !strings.Contains(prompt, p.Name) {
			t.Errorf("prompt missing pipeline name %q", p.Name)
		}
		if !strings.Contains(prompt, p.Description) {
			t.Errorf("prompt missing pipeline description %q", p.Description)
		}
	}
}

func TestBuildPrompt_ContainsUserRequest(t *testing.T) {
	userReq := "please push my changes to main"
	prompt := buildPrompt(userReq, nil)
	if !strings.Contains(prompt, userReq) {
		t.Errorf("prompt missing user request %q", userReq)
	}
}

func TestBuildPrompt_HasFewShotExamples(t *testing.T) {
	prompt := buildPrompt("test request", nil)
	// Should include at least 3 JSON examples
	count := strings.Count(prompt, `"pipeline"`)
	if count < 3 {
		t.Errorf("expected at least 3 few-shot JSON examples, found %d", count)
	}
}

// ── LLMClassifier ─────────────────────────────────────────────────────────────

func makeLLMClassifier(t *testing.T, responseJSON string) (*LLMClassifier, *executor.Manager) {
	t.Helper()
	mgr := executor.NewManager()
	stub := &executor.StubExecutor{
		ExecutorName: "ollama",
		ExecuteFn: func(_ context.Context, _ string, _ map[string]string, w io.Writer) error {
			_, err := fmt.Fprint(w, responseJSON)
			return err
		},
	}
	if err := mgr.Register(stub); err != nil {
		t.Fatalf("Register stub: %v", err)
	}
	cfg := Config{
		Model:              "test-model",
		AmbiguousThreshold: 0.65,
	}
	return NewLLMClassifier(mgr, cfg), mgr
}

func TestLLMClassifier_MatchesPipeline(t *testing.T) {
	pipelines := []pipeline.PipelineRef{
		{Name: "git-push", Description: "Push changes to git"},
	}
	cls, _ := makeLLMClassifier(t, `{"pipeline":"git-push","confidence":0.91,"input":"","cron":""}`)
	result, err := cls.Classify(context.Background(), "push my code to git", pipelines)
	if err != nil {
		t.Fatalf("Classify error: %v", err)
	}
	if result.Pipeline == nil {
		t.Fatal("expected pipeline match, got nil")
	}
	if result.Pipeline.Name != "git-push" {
		t.Errorf("expected git-push, got %q", result.Pipeline.Name)
	}
	if result.Confidence != 0.91 {
		t.Errorf("expected confidence 0.91, got %f", result.Confidence)
	}
}

func TestLLMClassifier_ReturnsNONE(t *testing.T) {
	pipelines := []pipeline.PipelineRef{
		{Name: "git-push", Description: "Push changes to git"},
	}
	cls, _ := makeLLMClassifier(t, `{"pipeline":"NONE","confidence":0.0,"input":"","cron":""}`)
	result, err := cls.Classify(context.Background(), "what is the weather", pipelines)
	if err != nil {
		t.Fatalf("Classify error: %v", err)
	}
	if result.Pipeline != nil {
		t.Errorf("expected nil pipeline for NONE, got %q", result.Pipeline.Name)
	}
}

func TestLLMClassifier_CaseInsensitiveMatch(t *testing.T) {
	pipelines := []pipeline.PipelineRef{
		{Name: "Git-Push", Description: "Push changes to git"},
	}
	// LLM returns lowercase version
	cls, _ := makeLLMClassifier(t, `{"pipeline":"git-push","confidence":0.90,"input":"","cron":""}`)
	result, err := cls.Classify(context.Background(), "push my code", pipelines)
	if err != nil {
		t.Fatalf("Classify error: %v", err)
	}
	if result.Pipeline == nil {
		t.Fatal("expected pipeline match (case-insensitive), got nil")
	}
	if result.Pipeline.Name != "Git-Push" {
		t.Errorf("expected Git-Push, got %q", result.Pipeline.Name)
	}
}

func TestLLMClassifier_HallucinatedPipeline(t *testing.T) {
	pipelines := []pipeline.PipelineRef{
		{Name: "git-push", Description: "Push changes to git"},
	}
	// LLM returns a pipeline name not in the list
	cls, _ := makeLLMClassifier(t, `{"pipeline":"nonexistent-pipeline","confidence":0.90,"input":"","cron":""}`)
	result, err := cls.Classify(context.Background(), "do something", pipelines)
	if err != nil {
		t.Fatalf("Classify error: %v", err)
	}
	if result.Pipeline != nil {
		t.Errorf("expected nil pipeline for hallucinated name, got %q", result.Pipeline.Name)
	}
}

func TestLLMClassifier_BelowAmbiguousThreshold(t *testing.T) {
	pipelines := []pipeline.PipelineRef{
		{Name: "git-push", Description: "Push changes to git"},
	}
	// Confidence 0.5 < AmbiguousThreshold 0.65
	cls, _ := makeLLMClassifier(t, `{"pipeline":"git-push","confidence":0.50,"input":"","cron":""}`)
	result, err := cls.Classify(context.Background(), "maybe push?", pipelines)
	if err != nil {
		t.Fatalf("Classify error: %v", err)
	}
	if result.Pipeline != nil {
		t.Errorf("expected nil pipeline below ambiguous threshold, got %q", result.Pipeline.Name)
	}
}

func TestLLMClassifier_ExtractsCronAndInput(t *testing.T) {
	pipelines := []pipeline.PipelineRef{
		{Name: "docs-improve", Description: "Improve docs"},
	}
	cls, _ := makeLLMClassifier(t, `{"pipeline":"docs-improve","confidence":0.88,"input":"executor package","cron":"0 * * * *"}`)
	result, err := cls.Classify(context.Background(), "improve executor docs every hour", pipelines)
	if err != nil {
		t.Fatalf("Classify error: %v", err)
	}
	if result.Input != "executor package" {
		t.Errorf("input: want %q, got %q", "executor package", result.Input)
	}
	if result.CronExpr != "0 * * * *" {
		t.Errorf("cron: want %q, got %q", "0 * * * *", result.CronExpr)
	}
}

func TestLLMClassifier_EmptyModelReturnsError(t *testing.T) {
	// Regression: buildPanelRouter was constructing router.Config without Model,
	// causing the classify step to fail with "model is required". Verify that an
	// empty Config.Model propagates an error from the executor back to the caller.
	mgr := executor.NewManager()
	stub := &executor.StubExecutor{
		ExecutorName: "ollama",
		ExecuteFn: func(_ context.Context, _ string, vars map[string]string, _ io.Writer) error {
			if vars["model"] == "" {
				return fmt.Errorf("model is required: set --model flag or GLITCH_MODEL environment variable")
			}
			return nil
		},
	}
	if err := mgr.Register(stub); err != nil {
		t.Fatalf("Register stub: %v", err)
	}
	cls := NewLLMClassifier(mgr, Config{AmbiguousThreshold: 0.65}) // Model intentionally empty
	pipelines := []pipeline.PipelineRef{{Name: "git-push", Description: "push"}}
	_, err := cls.Classify(context.Background(), "push my code", pipelines)
	if err == nil {
		t.Fatal("expected error when Config.Model is empty, got nil")
	}
	if !strings.Contains(err.Error(), "model is required") {
		t.Errorf("expected 'model is required' in error, got: %v", err)
	}
}
