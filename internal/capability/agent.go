package capability

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
)

// ToolProvider is the abstraction over a chat model that natively supports
// function / tool calling. Implementations wrap a concrete backend (Ollama
// /api/chat with the tools field, opencode CLI in tool mode, the Claude
// Messages API, etc.).
//
// The contract is deliberately narrow: one call, one Reply. No streaming at
// this layer — the Agent loop drives the conversation turn by turn, and a
// reply either lands as a text chunk (forwarded to the caller's writer) or
// as tool calls (executed and fed back). Streaming at the token level is a
// separate concern that can be added later without changing this interface.
type ToolProvider interface {
	// Chat sends the accumulated message history plus a tool catalog to the
	// model and returns what the model decided to do next.
	Chat(ctx context.Context, msgs []Message, tools []ToolSpec) (Reply, error)
}

// Role is a chat message role. We intentionally do not include "system"
// here because Agent does not prepend a hardcoded system prompt — that is
// the whole point of the redesign. If a caller wants a persona, they pass
// one explicitly via Agent.System.
type Role string

const (
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	RoleTool      Role = "tool"
	RoleSystem    Role = "system"
)

// Message is one turn in the conversation history. For tool results, Name
// holds the capability name and ToolCallID ties the result back to the
// tool call the model emitted.
type Message struct {
	Role       Role
	Content    string
	ToolCalls  []ToolCall // set on assistant messages that requested tools
	ToolCallID string     // set on tool-result messages
	Name       string     // set on tool-result messages (capability name)
}

// ToolCall is one tool invocation the model requested. ID is provider-
// assigned and must be echoed back on the tool-result message so the model
// can match calls to results when it issued several in parallel.
type ToolCall struct {
	ID        string
	Name      string
	Arguments map[string]any
}

// ToolSpec is the function-calling schema advertised to the model. The
// Agent builds one per on-demand capability in the registry, using the
// manifest name and description verbatim. Arguments schema is minimal: a
// single "input" string that the runner passes as Input.Stdin. Richer
// typed arguments are a follow-up once we have capabilities that actually
// need them.
type ToolSpec struct {
	Name        string
	Description string
}

// Reply is what the model decided to do on one turn. Either Text is
// non-empty (a normal answer — the loop terminates) or ToolCalls is
// non-empty (the loop executes them and asks again).
type Reply struct {
	Text      string
	ToolCalls []ToolCall
}

// Agent is the multi-turn tool-calling loop. It holds conversation history,
// presents the registry's on-demand capabilities to the model as tools, and
// iterates until the model produces a text reply.
//
// There is no hardcoded system prompt. If you want one, set Agent.System
// to a string the caller explicitly supplies (e.g. the contents of a
// persona.md file the user authored). The default is empty and the
// agent will happily run with no system turn at all.
//
// One Agent per conversation. Concurrent Send calls on the same Agent are
// not supported — the history slice is mutated in place.
type Agent struct {
	reg      *Registry
	runner   *Runner
	provider ToolProvider

	// System is an optional system message prepended to history on the
	// first Chat call. Empty means no system turn. Callers that want a
	// persona should load it from disk and set this field.
	System string

	// MaxToolHops caps how many tool-call rounds a single Send can run
	// before giving up and returning whatever text (or an error) the
	// model last produced. Prevents runaway loops on buggy prompts or
	// flaky tool responses. Defaults to 8.
	MaxToolHops int

	history []Message
}

// NewAgent constructs an Agent bound to the given registry, runner, and
// tool-capable provider. The runner is what actually invokes capabilities
// when the model emits tool calls.
func NewAgent(reg *Registry, runner *Runner, provider ToolProvider) *Agent {
	return &Agent{
		reg:      reg,
		runner:   runner,
		provider: provider,
	}
}

// History returns a copy of the current conversation turns. Useful for
// persistence between sessions and for tests.
func (a *Agent) History() []Message {
	out := make([]Message, len(a.history))
	copy(out, a.history)
	return out
}

// Reset clears the conversation history. The optional System turn is
// re-prepended on the next Send.
func (a *Agent) Reset() {
	a.history = nil
}

// ErrToolHopLimit is returned when Send exceeds MaxToolHops without the
// model producing a terminal text reply. Callers that want to recover can
// inspect Agent.History() to see the last tool result the loop recorded.
var ErrToolHopLimit = errors.New("agent: tool hop limit exceeded")

// Send runs one user turn: appends the message to history, then loops
// (chat → tool call → invoke → append result → chat again …) until the
// model returns a text reply. The final text is written to w as a single
// write (no streaming at this layer) and returned.
//
// Tool calls are executed through the runner with Input.Stdin set to the
// "input" argument the model provided. Stream events from the capability
// are buffered and become the tool-result content the model sees on the
// next turn. This matches how every tool-using API treats tool output:
// a text blob keyed to the call id.
func (a *Agent) Send(ctx context.Context, userMsg string, w io.Writer) (string, error) {
	if a.provider == nil {
		return "", errors.New("agent: no tool provider configured")
	}
	if w == nil {
		w = io.Discard
	}
	if len(a.history) == 0 && a.System != "" {
		a.history = append(a.history, Message{
			Role:    RoleSystem,
			Content: a.System,
		})
	}
	a.history = append(a.history, Message{
		Role:    RoleUser,
		Content: userMsg,
	})

	tools := a.buildToolCatalog()
	maxHops := a.MaxToolHops
	if maxHops == 0 {
		maxHops = 8
	}

	for hop := 0; hop < maxHops; hop++ {
		reply, err := a.provider.Chat(ctx, a.history, tools)
		if err != nil {
			return "", fmt.Errorf("agent: chat: %w", err)
		}

		// Record the assistant turn exactly as the model produced it.
		// Tool-call messages may have empty Text (the model chose to
		// call tools instead of answering) — that is fine and normal.
		a.history = append(a.history, Message{
			Role:      RoleAssistant,
			Content:   reply.Text,
			ToolCalls: reply.ToolCalls,
		})

		if len(reply.ToolCalls) == 0 {
			// Terminal text reply. Forward to the writer and return.
			if reply.Text != "" {
				if _, werr := io.WriteString(w, reply.Text); werr != nil {
					return reply.Text, werr
				}
			}
			return reply.Text, nil
		}

		// Execute each tool call and append its result as a tool
		// message so the next Chat call sees what happened. We run
		// them sequentially to keep history ordering deterministic;
		// parallel fan-out is a premature optimisation for a 7B-model
		// chat loop.
		for _, call := range reply.ToolCalls {
			result := a.runToolCall(ctx, call)
			a.history = append(a.history, Message{
				Role:       RoleTool,
				Content:    result,
				ToolCallID: call.ID,
				Name:       call.Name,
			})
		}
	}

	return "", ErrToolHopLimit
}

// buildToolCatalog produces the tool list the agent advertises to the
// model on every Chat call. Mirrors Router.candidates — only on-demand
// capabilities are presented, because background workers (indexers,
// daemons) are not things a user asks for by name and should never be
// callable mid-chat.
func (a *Agent) buildToolCatalog() []ToolSpec {
	if a.reg == nil {
		return nil
	}
	all := a.reg.List()
	out := make([]ToolSpec, 0, len(all))
	for _, c := range all {
		m := c.Manifest()
		if m.Trigger.Mode != TriggerOnDemand {
			continue
		}
		out = append(out, ToolSpec{
			Name:        m.Name,
			Description: firstLine(m.Description),
		})
	}
	return out
}

// runToolCall invokes one capability via the runner and returns its
// buffered Stream output as the tool-result content. Errors become part
// of the content (prefixed with "error: ") rather than terminating the
// Send loop — the model can react to a failed tool, retry with different
// arguments, or apologise to the user. Turning every tool failure into a
// hard Send error would make the agent brittle.
func (a *Agent) runToolCall(ctx context.Context, call ToolCall) string {
	if a.runner == nil {
		return "error: no runner configured"
	}
	input, _ := call.Arguments["input"].(string)
	var buf bytes.Buffer
	err := a.runner.Invoke(ctx, call.Name, Input{Stdin: input}, &buf)
	if err != nil {
		// Unknown capability, start failure, etc. The model sees the
		// error and can adapt.
		if buf.Len() > 0 {
			return fmt.Sprintf("error: %s\n%s", err, buf.String())
		}
		return fmt.Sprintf("error: %s", err)
	}
	if buf.Len() == 0 {
		return "(no output)"
	}
	return buf.String()
}
