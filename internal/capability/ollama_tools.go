package capability

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// OllamaToolProvider talks to a local Ollama server via /api/chat with the
// `tools` field populated, letting a tool-capable model (qwen2.5:7b and
// friends) decide between answering in text and emitting tool_calls.
//
// Only a single non-streaming request per Chat is issued. Streaming the
// model's text chunk-by-chunk is a nice-to-have for UX but complicates
// tool-call parsing (the tool_calls field only appears on the final
// message) and is left as a follow-up.
type OllamaToolProvider struct {
	// Model is the Ollama model tag, e.g. "qwen2.5:7b". Defaults to
	// DefaultRouterModel (qwen2.5:7b) when empty — same default as the
	// one-shot router so the two code paths agree on "the local model".
	Model string
	// BaseURL is the Ollama endpoint. Defaults to DefaultRouterBaseURL.
	BaseURL string
	// HTTPClient overrides the HTTP client. Tests inject a client
	// pointing at a httptest.Server.
	HTTPClient *http.Client
	// Timeout caps how long a single Chat call may block. Tool-using
	// turns sometimes take longer than a one-shot pick because the
	// model has to reason about arguments, so this default is higher
	// than the router's 30s.
	Timeout time.Duration
}

// NewOllamaToolProvider returns a provider pointed at the local Ollama
// server with the default tool-capable model. Matches the zero-config
// path the rest of gl1tch takes — you get something sensible by default
// and can override fields for testing or model experiments.
func NewOllamaToolProvider() *OllamaToolProvider {
	return &OllamaToolProvider{}
}

// ollamaChatReq is the wire shape for POST /api/chat with tools.
// Matches the schema Ollama exposes for tool-capable models.
type ollamaChatReq struct {
	Model    string           `json:"model"`
	Messages []ollamaChatMsg  `json:"messages"`
	Stream   bool             `json:"stream"`
	Tools    []ollamaToolDecl `json:"tools,omitempty"`
	Options  map[string]any   `json:"options,omitempty"`
}

type ollamaChatMsg struct {
	Role      string                 `json:"role"`
	Content   string                 `json:"content"`
	ToolCalls []ollamaToolCallOut    `json:"tool_calls,omitempty"`
	Name      string                 `json:"name,omitempty"`
	// ToolCallID is non-standard for Ollama today but harmless — Ollama
	// ignores unknown fields. We include it so tool-result turns keep
	// the linkage the agent cares about when other providers need it.
	ToolCallID string `json:"tool_call_id,omitempty"`
}

type ollamaToolDecl struct {
	Type     string              `json:"type"`
	Function ollamaToolDeclFunc  `json:"function"`
}

type ollamaToolDeclFunc struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  map[string]any `json:"parameters"`
}

type ollamaToolCallOut struct {
	Function ollamaToolCallOutFunc `json:"function"`
	// Ollama does not currently return an id field on tool calls,
	// so the agent synthesises one on parse. Kept here so a future
	// Ollama release that adds the field populates it naturally.
	ID string `json:"id,omitempty"`
}

type ollamaToolCallOutFunc struct {
	Name      string         `json:"name"`
	Arguments map[string]any `json:"arguments"`
}

type ollamaChatResp struct {
	Message ollamaChatMsg `json:"message"`
	Done    bool          `json:"done"`
}

// Chat sends one tool-enabled turn to Ollama and parses the reply into the
// Agent's neutral Reply shape.
func (p *OllamaToolProvider) Chat(ctx context.Context, msgs []Message, tools []ToolSpec) (Reply, error) {
	timeout := p.Timeout
	if timeout == 0 {
		timeout = 2 * time.Minute
	}
	askCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	req := ollamaChatReq{
		Model:    p.modelName(),
		Messages: toOllamaMessages(msgs),
		Stream:   false,
		Tools:    toOllamaTools(tools),
		Options: map[string]any{
			// Deterministic selection: we want the model to pick a
			// tool or answer, not improvise wildly.
			"temperature": 0.2,
		},
	}

	body, err := json.Marshal(req)
	if err != nil {
		return Reply{}, err
	}

	base := p.BaseURL
	if base == "" {
		base = DefaultRouterBaseURL
	}
	httpReq, err := http.NewRequestWithContext(askCtx, http.MethodPost, base+"/api/chat", bytes.NewReader(body))
	if err != nil {
		return Reply{}, err
	}
	httpReq.Header.Set("Content-Type", "application/json")

	client := p.HTTPClient
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(httpReq)
	if err != nil {
		return Reply{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		data, _ := io.ReadAll(resp.Body)
		return Reply{}, fmt.Errorf("ollama %d: %s", resp.StatusCode, bytes.TrimSpace(data))
	}

	var out ollamaChatResp
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return Reply{}, err
	}

	rep := Reply{Text: out.Message.Content}
	for i, tc := range out.Message.ToolCalls {
		id := tc.ID
		if id == "" {
			// Ollama does not currently emit ids; synthesise a stable
			// per-turn id so the agent can thread tool-result
			// messages back even when the model issues several calls
			// in one turn.
			id = fmt.Sprintf("call_%d", i)
		}
		rep.ToolCalls = append(rep.ToolCalls, ToolCall{
			ID:        id,
			Name:      tc.Function.Name,
			Arguments: tc.Function.Arguments,
		})
	}
	return rep, nil
}

func (p *OllamaToolProvider) modelName() string {
	if p.Model != "" {
		return p.Model
	}
	return DefaultRouterModel
}

// toOllamaMessages converts the agent's neutral message history into the
// Ollama chat wire format. Tool messages are mapped to role="tool" with
// the capability name in the name field — the shape Ollama documents for
// function-calling models.
func toOllamaMessages(msgs []Message) []ollamaChatMsg {
	out := make([]ollamaChatMsg, 0, len(msgs))
	for _, m := range msgs {
		om := ollamaChatMsg{
			Role:    string(m.Role),
			Content: m.Content,
		}
		if m.Name != "" {
			om.Name = m.Name
		}
		if m.ToolCallID != "" {
			om.ToolCallID = m.ToolCallID
		}
		for _, tc := range m.ToolCalls {
			om.ToolCalls = append(om.ToolCalls, ollamaToolCallOut{
				ID: tc.ID,
				Function: ollamaToolCallOutFunc{
					Name:      tc.Name,
					Arguments: tc.Arguments,
				},
			})
		}
		out = append(out, om)
	}
	return out
}

// toOllamaTools renders the agent's tool catalog as Ollama's function
// declarations. The parameter schema is deliberately minimal: every tool
// accepts a single string field named "input" that the runner will pipe
// into the capability's stdin. Capabilities that need richer structured
// inputs will get a per-tool schema once they exist — designing that
// surface before we have any consumer would be speculative.
func toOllamaTools(tools []ToolSpec) []ollamaToolDecl {
	if len(tools) == 0 {
		return nil
	}
	out := make([]ollamaToolDecl, 0, len(tools))
	for _, t := range tools {
		out = append(out, ollamaToolDecl{
			Type: "function",
			Function: ollamaToolDeclFunc{
				Name:        t.Name,
				Description: t.Description,
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"input": map[string]any{
							"type":        "string",
							"description": "Free-form input forwarded to the capability's stdin.",
						},
					},
					"required": []string{"input"},
				},
			},
		})
	}
	return out
}
