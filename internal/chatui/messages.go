package chatui

import "time"

type role int

const (
	roleUser      role = iota
	roleAssistant role = iota
)

// message represents a single turn in a conversation history.
type message struct {
	role    role
	content string
	events  []string // tool/status events from the provider (e.g. "Bash: ls -la")
}

// sessionEntry represents a resumable session from any provider.
type sessionEntry struct {
	id        string
	name      string    // slug, custom name, or truncated first prompt
	provider  string    // "claude", "gemini", "copilot"
	updatedAt time.Time // for sorting most-recent-first
}

// StreamChunk carries a partial text chunk from a provider.
type StreamChunk struct {
	Text string
}

// StreamStatus carries a tool/action event from the provider.
type StreamStatus struct {
	Tool  string
	Input string
}

// StreamDone signals the stream ended; carries final stats.
type StreamDone struct {
	SessionID     string
	Model         string
	InputTokens   int
	CacheTokens   int
	OutputTokens  int
	ContextWindow int
}

// StreamErr carries an error that occurred during streaming.
type StreamErr struct {
	Err string
}

// StreamWaiting signals that the adapter subprocess needs user input.
type StreamWaiting struct {
	Hint    string
	InputCh chan<- string
}

// TelemetryPayload is published to the "orcai.telemetry" bus topic on stream
// start ("streaming") and stream end ("done").
type TelemetryPayload struct {
	SessionID    string  `json:"session_id"`
	WindowName   string  `json:"window_name"`
	Provider     string  `json:"provider"`
	Status       string  `json:"status"` // "streaming" | "done"
	InputTokens  int     `json:"input_tokens"`
	OutputTokens int     `json:"output_tokens"`
	CostUSD      float64 `json:"cost_usd"`
}

// StreamEvent is the union type for all streaming events sent on a provider channel.
type StreamEvent struct {
	Chunk   *StreamChunk
	Status  *StreamStatus
	Done    *StreamDone
	Err     *StreamErr
	Waiting *StreamWaiting
}
