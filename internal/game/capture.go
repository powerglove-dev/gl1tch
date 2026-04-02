// Package game implements the token-score gamification system for gl1tch.
// It captures provider token usage, computes XP, and narrates results via Ollama.
package game

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
)

// TokenUsage holds the token metrics extracted from a provider's output stream,
// plus game-context fields set by the scoring path before calling ComputeXP.
type TokenUsage struct {
	Provider            string
	Model               string
	InputTokens         int64
	OutputTokens        int64
	CacheReadTokens     int64
	CacheCreationTokens int64
	TotalCostUSD        float64
	DurationMS          int64
	// StreakDays is set by the scoring path to allow the streak multiplier to
	// be applied inside ComputeXP. It is never populated by token parsers.
	StreakDays int
}

// TokenParser extracts TokenUsage from accumulated provider output bytes.
type TokenParser func(data []byte) TokenUsage

// GameTeeWriter wraps an io.Writer, accumulates all written bytes, and
// parses provider token usage on Close.
type GameTeeWriter struct {
	dst    io.Writer
	buf    bytes.Buffer
	parser TokenParser
}

// NewGameTeeWriter returns a GameTeeWriter that tees output to dst and
// selects the parser by executor category key (e.g. "providers.claude").
func NewGameTeeWriter(dst io.Writer, category string) *GameTeeWriter {
	parser := tokenParsers[category]
	if parser == nil {
		parser = func([]byte) TokenUsage { return TokenUsage{} }
	}
	return &GameTeeWriter{dst: dst, parser: parser}
}

// Write writes p to the underlying writer and also buffers it locally.
func (w *GameTeeWriter) Write(p []byte) (int, error) {
	n, err := w.dst.Write(p)
	// Buffer only what was actually written.
	if n > 0 {
		w.buf.Write(p[:n])
	}
	return n, err
}

// Close runs the parser on the accumulated buffer and returns the TokenUsage.
// The underlying writer is NOT closed — the caller owns its lifetime.
func (w *GameTeeWriter) Close() TokenUsage {
	return w.parser(w.buf.Bytes())
}

// tokenParsers maps executor category strings to their parsers.
var tokenParsers = map[string]TokenParser{
	"providers.claude":  parseClaudeTokens,
	"providers.codex":   parseCodexTokens,
	"providers.copilot": parseCopilotTokens,
	"providers.gemini":  parseGeminiTokens,
	"ollama":            parseOllamaTokens,
	"ollama-complete":   parseOllamaTokens,
}

// ── Claude ───────────────────────────────────────────────────────────────────

// parseClaudeTokens scans JSONL output for a line with "type":"result" and
// extracts token counts from the usage and modelUsage fields.
func parseClaudeTokens(data []byte) TokenUsage {
	var usage TokenUsage
	scanner := bufio.NewScanner(bytes.NewReader(data))
	for scanner.Scan() {
		line := scanner.Bytes()
		var record struct {
			Type       string  `json:"type"`
			TotalCost  float64 `json:"total_cost_usd"`
			DurationMS int64   `json:"duration_ms"`
			Usage      struct {
				InputTokens              int64 `json:"input_tokens"`
				OutputTokens             int64 `json:"output_tokens"`
				CacheReadInputTokens     int64 `json:"cache_read_input_tokens"`
				CacheCreationInputTokens int64 `json:"cache_creation_input_tokens"`
			} `json:"usage"`
			ModelUsage map[string]struct {
				InputTokens              int64  `json:"inputTokens"`
				OutputTokens             int64  `json:"outputTokens"`
				CacheReadInputTokens     int64  `json:"cacheReadInputTokens"`
				CacheCreationInputTokens int64  `json:"cacheCreationInputTokens"`
				CostUSD                  float64 `json:"costUSD"`
			} `json:"modelUsage"`
		}
		if err := json.Unmarshal(line, &record); err != nil {
			continue
		}
		if record.Type != "result" {
			continue
		}
		usage.Provider = "providers.claude"
		usage.InputTokens = record.Usage.InputTokens
		usage.OutputTokens = record.Usage.OutputTokens
		usage.CacheReadTokens = record.Usage.CacheReadInputTokens
		usage.CacheCreationTokens = record.Usage.CacheCreationInputTokens
		usage.TotalCostUSD = record.TotalCost
		usage.DurationMS = record.DurationMS
		// Extract model name from modelUsage map (first key).
		for model := range record.ModelUsage {
			usage.Model = model
			break
		}
		return usage
	}
	return usage
}

// ── Codex ────────────────────────────────────────────────────────────────────

// parseCodexTokens scans JSONL for "type":"turn.completed" and extracts usage.
func parseCodexTokens(data []byte) TokenUsage {
	var usage TokenUsage
	scanner := bufio.NewScanner(bytes.NewReader(data))
	for scanner.Scan() {
		var record struct {
			Type  string `json:"type"`
			Usage struct {
				InputTokens        int64 `json:"input_tokens"`
				CachedInputTokens  int64 `json:"cached_input_tokens"`
				OutputTokens       int64 `json:"output_tokens"`
			} `json:"usage"`
		}
		if err := json.Unmarshal(scanner.Bytes(), &record); err != nil {
			continue
		}
		if record.Type != "turn.completed" {
			continue
		}
		usage.Provider = "providers.codex"
		usage.InputTokens = record.Usage.InputTokens
		usage.CacheReadTokens = record.Usage.CachedInputTokens
		usage.OutputTokens = record.Usage.OutputTokens
		return usage
	}
	return usage
}

// ── Copilot ──────────────────────────────────────────────────────────────────

// parseCopilotTokens accumulates outputTokens from assistant.message events and
// extracts totalApiDurationMs from the result event.
func parseCopilotTokens(data []byte) TokenUsage {
	var usage TokenUsage
	usage.Provider = "providers.copilot"
	scanner := bufio.NewScanner(bytes.NewReader(data))
	for scanner.Scan() {
		var record struct {
			Type string `json:"type"`
			Data struct {
				OutputTokens int64 `json:"outputTokens"`
			} `json:"data"`
			Usage struct {
				TotalAPIDurationMS int64 `json:"totalApiDurationMs"`
			} `json:"usage"`
		}
		if err := json.Unmarshal(scanner.Bytes(), &record); err != nil {
			continue
		}
		switch record.Type {
		case "assistant.message":
			usage.OutputTokens += record.Data.OutputTokens
		case "result":
			usage.DurationMS = record.Usage.TotalAPIDurationMS
		}
	}
	return usage
}

// ── Gemini ───────────────────────────────────────────────────────────────────

// parseGeminiTokens finds a JSON object with prompt_token_count and
// candidates_token_count fields and extracts them.
func parseGeminiTokens(data []byte) TokenUsage {
	var usage TokenUsage
	usage.Provider = "providers.gemini"
	scanner := bufio.NewScanner(bytes.NewReader(data))
	for scanner.Scan() {
		var record struct {
			PromptTokenCount     int64 `json:"prompt_token_count"`
			CandidatesTokenCount int64 `json:"candidates_token_count"`
			CachedContentTokenCount int64 `json:"cached_content_token_count"`
		}
		if err := json.Unmarshal(scanner.Bytes(), &record); err != nil {
			continue
		}
		if record.PromptTokenCount > 0 || record.CandidatesTokenCount > 0 {
			usage.InputTokens = record.PromptTokenCount
			usage.OutputTokens = record.CandidatesTokenCount
			usage.CacheReadTokens = record.CachedContentTokenCount
			return usage
		}
	}
	return usage
}

// ── Ollama ───────────────────────────────────────────────────────────────────

// parseOllamaTokens scans output for the "gl1tch-stats" JSON line emitted by
// the gl1tch-ollama plugin after the response text.  Fields map directly from
// the Ollama /api/generate response: prompt_eval_count → InputTokens,
// eval_count → OutputTokens, total_duration (ns) → DurationMS.
func parseOllamaTokens(data []byte) TokenUsage {
	var usage TokenUsage
	scanner := bufio.NewScanner(bytes.NewReader(data))
	for scanner.Scan() {
		var record struct {
			Type         string `json:"type"`
			Model        string `json:"model"`
			InputTokens  int64  `json:"input_tokens"`
			OutputTokens int64  `json:"output_tokens"`
			DurationMS   int64  `json:"duration_ms"`
		}
		if err := json.Unmarshal(scanner.Bytes(), &record); err != nil {
			continue
		}
		if record.Type != "gl1tch-stats" {
			continue
		}
		usage.Provider = "ollama"
		usage.Model = record.Model
		usage.InputTokens = record.InputTokens
		usage.OutputTokens = record.OutputTokens
		usage.DurationMS = record.DurationMS
		return usage
	}
	return usage
}
