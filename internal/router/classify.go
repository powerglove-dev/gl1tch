package router

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/8op-org/gl1tch/internal/executor"
	"github.com/8op-org/gl1tch/internal/pipeline"
)

// classifyResponse is the structured JSON the LLM must return.
type classifyResponse struct {
	Pipeline   string  `json:"pipeline"`
	Confidence float64 `json:"confidence"`
	Input      string  `json:"input"`
	Cron       string  `json:"cron"`
}

// LLMClassifier runs a single structured pipeline.Run call to classify a prompt.
type LLMClassifier struct {
	mgr *executor.Manager
	cfg Config
}

// NewLLMClassifier creates an LLMClassifier.
func NewLLMClassifier(mgr *executor.Manager, cfg Config) *LLMClassifier {
	return &LLMClassifier{mgr: mgr, cfg: cfg}
}

// Classify sends the prompt to the LLM and returns a RouteResult.
// Errors from the LLM are surfaced so callers can choose fallback behavior.
func (c *LLMClassifier) Classify(ctx context.Context, prompt string, pipelines []pipeline.PipelineRef) (*RouteResult, error) {
	llmPrompt := buildPrompt(prompt, pipelines)

	classifyPipeline := &pipeline.Pipeline{
		Name:    "router-classify",
		Version: "1",
		Steps: []pipeline.Step{
			{
				ID:       "classify",
				Executor: "ollama",
				Model:    c.cfg.Model,
				Prompt:   llmPrompt,
			},
		},
	}

	raw, err := pipeline.Run(ctx, classifyPipeline, c.mgr, "", pipeline.WithSilentStatus(), pipeline.WithNoClarification())
	if err != nil {
		return nil, fmt.Errorf("router: llm classify: %w", err)
	}

	resp, err := parseClassifyResponse(raw)
	if err != nil {
		return nil, fmt.Errorf("router: parse classify response: %w", err)
	}

	// NONE or blank → no match.
	if strings.EqualFold(resp.Pipeline, "NONE") || resp.Pipeline == "" {
		return &RouteResult{Method: "llm", Confidence: resp.Confidence}, nil
	}

	// Below ambiguous threshold → no match.
	if resp.Confidence < c.cfg.AmbiguousThreshold {
		return &RouteResult{Method: "llm", Confidence: resp.Confidence}, nil
	}

	// Case-insensitive match against known pipeline names.
	var matched *pipeline.PipelineRef
	for i := range pipelines {
		if strings.EqualFold(pipelines[i].Name, resp.Pipeline) {
			matched = &pipelines[i]
			break
		}
	}

	if matched == nil {
		// Hallucinated pipeline name.
		return &RouteResult{Method: "llm", Confidence: resp.Confidence}, nil
	}

	return &RouteResult{
		Pipeline:   matched,
		Confidence: resp.Confidence,
		Input:      sanitizeFocus(resp.Input),
		CronExpr:   validateCron(resp.Cron),
		Method:     "llm",
	}, nil
}

// ── prompt building ───────────────────────────────────────────────────────────

// buildPrompt creates the structured LLM prompt with few-shot examples.
func buildPrompt(userPrompt string, pipelines []pipeline.PipelineRef) string {
	var sb strings.Builder

	sb.WriteString(`You are an intent router. Given a user request and a list of available pipelines, output a single JSON object selecting the best pipeline.

Rules:
- If no pipeline matches well, set "pipeline" to "NONE".
- "confidence" must be between 0.0 and 1.0.
- "input" is the topic or focus the user wants, or "" if none.
- "cron" is a 5-field cron expression if the user wants a schedule, or "" if not.

Examples:
{"pipeline":"git-push","confidence":0.93,"input":"","cron":""}
{"pipeline":"docs-improve","confidence":0.87,"input":"executor package","cron":"0 */2 * * *"}
{"pipeline":"NONE","confidence":0.10,"input":"","cron":""}

Available pipelines:
`)

	for _, p := range pipelines {
		fmt.Fprintf(&sb, "- %s: %s\n", p.Name, p.Description)
	}

	sb.WriteString("\nUser request: ")
	sb.WriteString(userPrompt)
	sb.WriteString("\n\nRespond with ONLY a single JSON object:")
	return sb.String()
}

// ── response parsing ──────────────────────────────────────────────────────────

// parseClassifyResponse extracts a classifyResponse from the raw LLM output.
// It handles model verbosity by finding the first { and last } in the string.
func parseClassifyResponse(raw string) (classifyResponse, error) {
	start := strings.Index(raw, "{")
	end := strings.LastIndex(raw, "}")
	if start == -1 || end == -1 || end <= start {
		return classifyResponse{}, fmt.Errorf("no JSON object found in response")
	}
	jsonStr := raw[start : end+1]

	var resp classifyResponse
	if err := json.Unmarshal([]byte(jsonStr), &resp); err != nil {
		return classifyResponse{}, fmt.Errorf("unmarshal classify response: %w", err)
	}

	// Clamp confidence to [0, 1].
	if resp.Confidence > 1 {
		resp.Confidence = 1
	}
	if resp.Confidence < 0 {
		resp.Confidence = 0
	}

	return resp, nil
}

// ── cron / focus helpers ──────────────────────────────────────────────────────

// validateCron checks that s is exactly 5 whitespace-separated fields.
// Returns "" if the cron expression is invalid or "NONE".
func validateCron(s string) string {
	s = strings.TrimSpace(s)
	if strings.EqualFold(s, "NONE") || s == "" {
		return ""
	}
	fields := strings.Fields(s)
	if len(fields) != 5 {
		return ""
	}
	return strings.Join(fields, " ")
}

// sanitizeFocus strips surrounding quotes and periods from focus, and returns
// "" for "NONE" or blank values.
func sanitizeFocus(s string) string {
	s = strings.TrimSpace(s)
	if strings.EqualFold(s, "NONE") || s == "" {
		return ""
	}
	return strings.Trim(s, `"'.`)
}
