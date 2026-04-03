package orchestrator

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

const (
	defaultDecisionTimeoutSecs = 30
	defaultOllamaURL           = "http://localhost:11434"
)

// DecisionNode evaluates a prompt against a local Ollama model and returns a
// branch name derived from the model's JSON response.
type DecisionNode struct {
	Model       string
	Prompt      string // raw template, expanded before call
	TimeoutSecs int    // default 30
	OllamaURL   string // default "http://localhost:11434"
}

// ollamaRequest is the body sent to Ollama /api/generate.
type ollamaRequest struct {
	Model     string `json:"model"`
	Prompt    string `json:"prompt"`
	Format    string `json:"format"`
	Stream    bool   `json:"stream"`
	KeepAlive int    `json:"keep_alive"`
}

// ollamaResponse is the response from Ollama /api/generate.
type ollamaResponse struct {
	Response string `json:"response"`
}

// Evaluate expands the prompt against wctx, calls Ollama /api/generate, and
// returns the branch name extracted from the model's JSON response.
//
// The model must return a JSON object with a string "branch" field inside the
// response field. For example: {"branch":"yes"}.
func (d *DecisionNode) Evaluate(ctx context.Context, wctx *WorkflowContext) (string, error) {
	timeoutSecs := d.TimeoutSecs
	if timeoutSecs <= 0 {
		timeoutSecs = defaultDecisionTimeoutSecs
	}
	ollamaURL := d.OllamaURL
	if ollamaURL == "" {
		ollamaURL = defaultOllamaURL
	}

	expanded := ExpandTemplate(d.Prompt, wctx)

	reqBody := ollamaRequest{
		Model:     d.Model,
		Prompt:    expanded,
		Format:    "json",
		Stream:    false,
		KeepAlive: -1,
	}
	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("orchestrator: decision marshal request: %w", err)
	}

	tctx, cancel := context.WithTimeout(ctx, time.Duration(timeoutSecs)*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(tctx, http.MethodPost, ollamaURL+"/api/generate", bytes.NewReader(bodyBytes))
	if err != nil {
		return "", fmt.Errorf("orchestrator: decision create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("orchestrator: decision ollama request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return "", fmt.Errorf("orchestrator: decision ollama HTTP %d: %s", resp.StatusCode, string(body))
	}

	var ollamaResp ollamaResponse
	if err := json.NewDecoder(resp.Body).Decode(&ollamaResp); err != nil {
		return "", fmt.Errorf("orchestrator: decision decode ollama response: %w", err)
	}

	// The response field is itself a JSON string containing the branch.
	var inner map[string]any
	if err := json.Unmarshal([]byte(ollamaResp.Response), &inner); err != nil {
		return "", fmt.Errorf("orchestrator: decision parse inner response JSON: %w", err)
	}

	branchVal, ok := inner["branch"]
	if !ok {
		return "", fmt.Errorf("orchestrator: decision response missing 'branch' field")
	}
	branch, ok := branchVal.(string)
	if !ok {
		return "", fmt.Errorf("orchestrator: decision 'branch' field must be a string, got %T", branchVal)
	}
	if branch == "" {
		return "", fmt.Errorf("orchestrator: decision returned empty branch")
	}
	return branch, nil
}
