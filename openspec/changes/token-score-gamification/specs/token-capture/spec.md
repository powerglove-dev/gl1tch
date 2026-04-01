## ADDED Requirements

### Requirement: TokenUsage struct captures per-run token data
The `internal/score` package SHALL define a `TokenUsage` struct:
```go
type TokenUsage struct {
    Provider            string
    Model               string
    InputTokens         int64
    OutputTokens        int64
    CacheReadTokens     int64
    CacheCreationTokens int64
    TotalCostUSD        float64
    DurationMS          int64
}
```
A zero-value `TokenUsage` SHALL be valid and represents a run with no token data captured.

#### Scenario: Zero value is valid
- **WHEN** a `TokenUsage{}` is passed to the XP engine
- **THEN** the engine returns zero XP without error

### Requirement: ScoreTeeWriter wraps executor output and extracts TokenUsage on close
`internal/score` SHALL provide a `ScoreTeeWriter` that wraps an `io.Writer`, accumulates all bytes written to it, and on `Close()` runs the appropriate provider parser to populate a `TokenUsage`.

The `ScoreTeeWriter` SHALL be constructed with a provider category string (e.g., `providers.claude`) and a destination `io.Writer`. All bytes written to the tee SHALL be forwarded to the destination writer unchanged.

#### Scenario: Bytes pass through unchanged
- **WHEN** data is written to a `ScoreTeeWriter`
- **THEN** the same bytes appear on the wrapped destination writer

#### Scenario: Close extracts token data
- **WHEN** Claude JSON output containing `"usage":{"input_tokens":2,"output_tokens":5}` is written and `Close()` is called
- **THEN** `TokenUsage()` returns a struct with `InputTokens == 2` and `OutputTokens == 5`

#### Scenario: Unknown provider returns zero usage
- **WHEN** the category string is not a known provider
- **THEN** `Close()` succeeds and `TokenUsage()` returns a zero-value struct

### Requirement: Claude parser extracts tokens from final result JSON
The Claude parser SHALL scan accumulated output for a JSON object where `"type"` is `"result"` and extract:
- `usage.input_tokens` → `InputTokens`
- `usage.output_tokens` → `OutputTokens`
- `usage.cache_read_input_tokens` → `CacheReadTokens`
- `usage.cache_creation_input_tokens` → `CacheCreationTokens`
- `total_cost_usd` → `TotalCostUSD`
- First key in `modelUsage` → `Model`

#### Scenario: Full Claude result parsed
- **WHEN** output contains `{"type":"result","usage":{"input_tokens":2,"cache_creation_input_tokens":19018,"cache_read_input_tokens":0,"output_tokens":5},"total_cost_usd":0.07,"modelUsage":{"claude-sonnet-4-6":{}}}`
- **THEN** `InputTokens == 2`, `OutputTokens == 5`, `CacheCreationTokens == 19018`, `CostUSD == 0.07`, `Model == "claude-sonnet-4-6"`

### Requirement: Codex parser extracts tokens from turn.completed JSONL event
The Codex parser SHALL scan accumulated JSONL output for a line where `"type"` is `"turn.completed"` and extract:
- `usage.input_tokens` → `InputTokens`
- `usage.cached_input_tokens` → `CacheReadTokens`
- `usage.output_tokens` → `OutputTokens`

#### Scenario: Codex turn.completed parsed
- **WHEN** output contains `{"type":"turn.completed","usage":{"input_tokens":11695,"cached_input_tokens":3456,"output_tokens":17}}`
- **THEN** `InputTokens == 11695`, `CacheReadTokens == 3456`, `OutputTokens == 17`

### Requirement: Copilot parser extracts tokens from result JSONL event
The Copilot parser SHALL scan accumulated JSONL output for the `"result"` event and accumulate `outputTokens` from all `"assistant.message"` events. It SHALL extract:
- Sum of `data.outputTokens` across all `assistant.message` events → `OutputTokens`
- `usage.premiumRequests` stored as metadata (not in TokenUsage directly)
- `usage.totalApiDurationMs` → `DurationMS`

#### Scenario: Copilot result parsed
- **WHEN** output contains an `assistant.message` event with `"outputTokens":5` and a `result` event with `"totalApiDurationMs":1997`
- **THEN** `OutputTokens == 5`, `DurationMS == 1997`

### Requirement: Gemini parser extracts tokens from JSON output
The Gemini parser SHALL parse the JSON output object and extract:
- `prompt_token_count` → `InputTokens`
- `candidates_token_count` → `OutputTokens`
- `cached_content_token_count` → `CacheReadTokens`

#### Scenario: Gemini output parsed
- **WHEN** output contains `{"prompt_token_count":100,"candidates_token_count":20,"cached_content_token_count":50}`
- **THEN** `InputTokens == 100`, `OutputTokens == 20`, `CacheReadTokens == 50`

### Requirement: Failed parse logs a warning and returns zero usage
If a parser encounters malformed output or cannot find the expected token fields, it SHALL log a warning at DEBUG level and return a zero-value `TokenUsage`. It SHALL NOT return an error or interrupt the run.

#### Scenario: Malformed output handled gracefully
- **WHEN** the output buffer contains only plain text with no JSON
- **THEN** `Close()` returns nil and `TokenUsage()` returns zero values
