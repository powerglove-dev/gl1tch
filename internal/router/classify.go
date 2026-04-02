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

// buildPrompt creates the structured LLM prompt with a two-step intent gate.
// Step 1 gates on whether the input is an actionable command. Questions,
// observations, and conversational statements must return NONE immediately
// without proceeding to pipeline selection.
func buildPrompt(userPrompt string, pipelines []pipeline.PipelineRef) string {
	var sb strings.Builder

	sb.WriteString(`You are deciding whether a user message is an actionable command to run an automated workflow, or is instead a question, observation, or conversational statement.

Step 1 — Intent check: Is this an explicit command? The user must be directing you to perform an action (run, review, generate, check, scan, fix, build, deploy, start, launch, etc.).
If the message is phrased as a question, observation, speculation, or statement of uncertainty, output {"pipeline":"NONE","confidence":0.05,"input":"","cron":""} immediately without reading Step 2.

Step 2 — Only if it IS a command: select the best matching pipeline from the list below.
- "pipeline" must be a name from the list, or "NONE" if no pipeline fits the command.
- "confidence" is between 0.0 and 1.0.
- "input" is the topic, URL, or focus the user wants acted on, or "".
- "cron" is a 5-field cron expression if the user wants a schedule, or "".

Non-command examples — always output NONE regardless of topic:
{"pipeline":"NONE","confidence":0.05,"input":"","cron":""}  // "looks like there are merge conflicts?"
{"pipeline":"NONE","confidence":0.05,"input":"","cron":""}  // "why is the build failing?"
{"pipeline":"NONE","confidence":0.05,"input":"","cron":""}  // "seems like the deploy is slow"
{"pipeline":"NONE","confidence":0.05,"input":"","cron":""}  // "I wonder if this worked"
{"pipeline":"NONE","confidence":0.05,"input":"","cron":""}  // "any open PRs?"
{"pipeline":"NONE","confidence":0.05,"input":"","cron":""}  // "is the pipeline running?"

Command examples — match a pipeline or NONE if no pipeline fits:
{"pipeline":"pr-review","confidence":0.93,"input":"https://github.com/org/repo/pull/42","cron":""}
{"pipeline":"docs-improve","confidence":0.87,"input":"executor package","cron":"0 */2 * * *"}
{"pipeline":"NONE","confidence":0.10,"input":"","cron":""}  // command with no matching pipeline

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
// It finds the first balanced { ... } object so any trailing metadata lines
// (e.g. gl1tch-stats emitted by the ollama plugin) do not confuse the parse.
func parseClassifyResponse(raw string) (classifyResponse, error) {
	jsonStr := extractFirstJSONObject(raw)
	if jsonStr == "" {
		return classifyResponse{}, fmt.Errorf("no JSON object found in response")
	}

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

// extractFirstJSONObject returns the first balanced { ... } substring in s,
// allowing the caller to ignore any text or metadata that follows the JSON.
func extractFirstJSONObject(s string) string {
	depth := 0
	start := -1
	for i, c := range s {
		switch c {
		case '{':
			if depth == 0 {
				start = i
			}
			depth++
		case '}':
			depth--
			if depth == 0 && start != -1 {
				return s[start : i+1]
			}
		}
	}
	return ""
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
