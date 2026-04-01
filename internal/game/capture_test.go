package game

import (
	"io"
	"strings"
	"testing"
)

// Real output samples from the task specification.

const claudeSample = `{"type":"result","subtype":"success","is_error":false,"duration_ms":2961,"duration_api_ms":2907,"num_turns":1,"result":"Hey.","stop_reason":"end_turn","session_id":"758c06e2-3a73-45ea-a9e3-a0ea82cd7098","total_cost_usd":0.0713985,"usage":{"input_tokens":2,"cache_creation_input_tokens":19018,"cache_read_input_tokens":0,"output_tokens":5,"server_tool_use":{"web_search_requests":0,"web_fetch_requests":0},"service_tier":"standard","cache_creation":{"ephemeral_1h_input_tokens":19018,"ephemeral_5m_input_tokens":0},"inference_geo":"","iterations":[],"speed":"standard"},"modelUsage":{"claude-sonnet-4-6":{"inputTokens":2,"outputTokens":5,"cacheReadInputTokens":0,"cacheCreationInputTokens":19018,"webSearchRequests":0,"costUSD":0.0713985,"contextWindow":200000,"maxOutputTokens":32000}},"permission_denials":[],"fast_mode_state":"off","uuid":"5b0ebdf0-3556-4995-b955-75d0dc2f582f"}`

const codexSample = `{"type":"thread.started","thread_id":"019d4abb-922b-77e1-94cd-f4b25b819c93"}
{"type":"turn.started"}
{"type":"item.completed","item":{"id":"item_0","type":"agent_message","text":"Hi"}}
{"type":"turn.completed","usage":{"input_tokens":11695,"cached_input_tokens":3456,"output_tokens":17}}`

const copilotSample = `{"type":"assistant.message","data":{"messageId":"b74f0e6d","content":"Hi!","toolRequests":[],"interactionId":"96cf7d47","outputTokens":5},"id":"63d3b673","timestamp":"2026-04-01T20:28:38.030Z"}
{"type":"result","timestamp":"2026-04-01T20:28:38.031Z","sessionId":"0feadc6f","exitCode":0,"usage":{"premiumRequests":1,"totalApiDurationMs":1997,"sessionDurationMs":5117,"codeChanges":{"linesAdded":0,"linesRemoved":0,"filesModified":[]}}}`

const geminiSample = `{"prompt_token_count":100,"candidates_token_count":50,"cached_content_token_count":20}`

func TestParseClaudeTokens(t *testing.T) {
	got := parseClaudeTokens([]byte(claudeSample))
	if got.Provider != "providers.claude" {
		t.Errorf("Provider = %q, want providers.claude", got.Provider)
	}
	if got.Model != "claude-sonnet-4-6" {
		t.Errorf("Model = %q, want claude-sonnet-4-6", got.Model)
	}
	if got.InputTokens != 2 {
		t.Errorf("InputTokens = %d, want 2", got.InputTokens)
	}
	if got.OutputTokens != 5 {
		t.Errorf("OutputTokens = %d, want 5", got.OutputTokens)
	}
	if got.CacheCreationTokens != 19018 {
		t.Errorf("CacheCreationTokens = %d, want 19018", got.CacheCreationTokens)
	}
	if got.CacheReadTokens != 0 {
		t.Errorf("CacheReadTokens = %d, want 0", got.CacheReadTokens)
	}
	if got.TotalCostUSD == 0 {
		t.Error("TotalCostUSD should be non-zero")
	}
	if got.DurationMS != 2961 {
		t.Errorf("DurationMS = %d, want 2961", got.DurationMS)
	}
}

func TestParseCodexTokens(t *testing.T) {
	got := parseCodexTokens([]byte(codexSample))
	if got.Provider != "providers.codex" {
		t.Errorf("Provider = %q, want providers.codex", got.Provider)
	}
	if got.InputTokens != 11695 {
		t.Errorf("InputTokens = %d, want 11695", got.InputTokens)
	}
	if got.CacheReadTokens != 3456 {
		t.Errorf("CacheReadTokens = %d, want 3456", got.CacheReadTokens)
	}
	if got.OutputTokens != 17 {
		t.Errorf("OutputTokens = %d, want 17", got.OutputTokens)
	}
}

func TestParseCopilotTokens(t *testing.T) {
	got := parseCopilotTokens([]byte(copilotSample))
	if got.Provider != "providers.copilot" {
		t.Errorf("Provider = %q, want providers.copilot", got.Provider)
	}
	if got.OutputTokens != 5 {
		t.Errorf("OutputTokens = %d, want 5", got.OutputTokens)
	}
	if got.DurationMS != 1997 {
		t.Errorf("DurationMS = %d, want 1997", got.DurationMS)
	}
}

func TestParseGeminiTokens(t *testing.T) {
	got := parseGeminiTokens([]byte(geminiSample))
	if got.Provider != "providers.gemini" {
		t.Errorf("Provider = %q, want providers.gemini", got.Provider)
	}
	if got.InputTokens != 100 {
		t.Errorf("InputTokens = %d, want 100", got.InputTokens)
	}
	if got.OutputTokens != 50 {
		t.Errorf("OutputTokens = %d, want 50", got.OutputTokens)
	}
	if got.CacheReadTokens != 20 {
		t.Errorf("CacheReadTokens = %d, want 20", got.CacheReadTokens)
	}
}

func TestGameTeeWriter(t *testing.T) {
	var buf strings.Builder
	w := NewGameTeeWriter(&stringWriter{&buf}, "providers.claude")
	if _, err := io.WriteString(w, claudeSample); err != nil {
		t.Fatalf("Write: %v", err)
	}
	usage := w.Close()
	if usage.Provider != "providers.claude" {
		t.Errorf("TeeWriter provider = %q, want providers.claude", usage.Provider)
	}
	if usage.OutputTokens != 5 {
		t.Errorf("TeeWriter OutputTokens = %d, want 5", usage.OutputTokens)
	}
	// Verify the data was also forwarded to the underlying writer.
	if !strings.Contains(buf.String(), "result") {
		t.Error("data should be forwarded to underlying writer")
	}
}

func TestGameTeeWriter_UnknownCategory(t *testing.T) {
	var buf strings.Builder
	w := NewGameTeeWriter(&stringWriter{&buf}, "providers.unknown")
	io.WriteString(w, "some output") //nolint:errcheck
	usage := w.Close()
	// Unknown category returns zero-value TokenUsage.
	if usage.Provider != "" {
		t.Errorf("expected empty provider for unknown category, got %q", usage.Provider)
	}
}

type stringWriter struct{ b *strings.Builder }

func (sw *stringWriter) Write(p []byte) (int, error) {
	return sw.b.Write(p)
}
