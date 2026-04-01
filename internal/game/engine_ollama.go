package game

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// GameEngine calls Ollama to evaluate run data and narrate the result.
type GameEngine struct {
	baseURL string
	model   string
	client  *http.Client
}

// NewGameEngine returns a GameEngine pointed at the local Ollama instance.
func NewGameEngine() *GameEngine {
	return &GameEngine{
		baseURL: "http://localhost:11434",
		model:   "llama3.2",
		client:  &http.Client{},
	}
}

// EvaluateResult holds the structured output from the game_rules evaluation.
type EvaluateResult struct {
	Achievements []string `json:"achievements"`
	ICEClass     string   `json:"ice_class"`
	QuestEvents  []string `json:"quest_events"`
}

// Evaluate calls Ollama with the game_rules system prompt and run data as user
// message, returning a structured evaluation. On JSON parse failure it retries
// once with a stricter prompt. On second failure it returns an empty result
// and nil error — the game system is optional.
func (e *GameEngine) Evaluate(ctx context.Context, runDataJSON string, pack GameWorldPack) (EvaluateResult, error) {
	userMsg := "Run data: " + runDataJSON
	content, err := e.ollamaChat(ctx, pack.GameRules, userMsg)
	if err != nil {
		return EvaluateResult{}, nil //nolint:nilerr — game is optional
	}

	var result EvaluateResult
	if jsonErr := parseEvaluateResult(content, &result); jsonErr == nil {
		return result, nil
	}

	// Retry with a stricter prompt.
	strictMsg := userMsg + "\n\nReturn ONLY valid JSON, nothing else."
	content2, err2 := e.ollamaChat(ctx, pack.GameRules, strictMsg)
	if err2 != nil {
		return EvaluateResult{}, nil //nolint:nilerr
	}
	if jsonErr2 := parseEvaluateResult(content2, &result); jsonErr2 != nil {
		// Second failure — return empty result, no error.
		return EvaluateResult{}, nil
	}
	return result, nil
}

// Narrate calls Ollama with the narrator_style system prompt and returns a
// free-form narration string. Returns "" on any error.
func (e *GameEngine) Narrate(ctx context.Context, runDataJSON string, eval EvaluateResult, pack GameWorldPack) string {
	evalJSON, _ := json.Marshal(eval)
	userMsg := fmt.Sprintf("Run data: %s\nGame evaluation: %s", runDataJSON, string(evalJSON))
	content, err := e.ollamaChat(ctx, pack.NarratorStyle, userMsg)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(content)
}

// ollamaChat sends a non-streaming chat request to Ollama and returns the
// assistant message content.
func (e *GameEngine) ollamaChat(ctx context.Context, systemPrompt, userMsg string) (string, error) {
	body := map[string]any{
		"model": e.model,
		"messages": []map[string]string{
			{"role": "system", "content": systemPrompt},
			{"role": "user", "content": userMsg},
		},
		"stream": false,
	}
	bodyBytes, err := json.Marshal(body)
	if err != nil {
		return "", fmt.Errorf("game: marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		e.baseURL+"/api/chat", bytes.NewReader(bodyBytes))
	if err != nil {
		return "", fmt.Errorf("game: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := e.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("game: ollama request: %w", err)
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("game: read response: %w", err)
	}

	var ollamaResp struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	}
	if err := json.Unmarshal(respBytes, &ollamaResp); err != nil {
		return "", fmt.Errorf("game: parse response: %w", err)
	}
	return ollamaResp.Message.Content, nil
}

// parseEvaluateResult tries to JSON-unmarshal content into result. It handles
// the case where Ollama wraps the JSON in markdown code fences.
func parseEvaluateResult(content string, result *EvaluateResult) error {
	content = strings.TrimSpace(content)
	// Strip markdown fences if present.
	if strings.HasPrefix(content, "```") {
		lines := strings.Split(content, "\n")
		var inner []string
		for i, line := range lines {
			if i == 0 && strings.HasPrefix(line, "```") {
				continue
			}
			if line == "```" {
				break
			}
			inner = append(inner, line)
		}
		content = strings.Join(inner, "\n")
	}
	// Find the first '{' and last '}'.
	start := strings.Index(content, "{")
	end := strings.LastIndex(content, "}")
	if start < 0 || end < start {
		return fmt.Errorf("game: no JSON object found in response")
	}
	return json.Unmarshal([]byte(content[start:end+1]), result)
}
