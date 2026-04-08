package capability

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
)

// scriptedProvider is a ToolProvider test double that returns a pre-canned
// sequence of Replies, one per Chat call. It also records every (msgs,
// tools) pair it was handed so tests can assert on what the Agent actually
// sent the model.
type scriptedProvider struct {
	replies []Reply
	calls   []scriptedCall
	err     error
}

type scriptedCall struct {
	msgs  []Message
	tools []ToolSpec
}

func (s *scriptedProvider) Chat(_ context.Context, msgs []Message, tools []ToolSpec) (Reply, error) {
	if s.err != nil {
		return Reply{}, s.err
	}
	s.calls = append(s.calls, scriptedCall{
		msgs:  append([]Message(nil), msgs...),
		tools: append([]ToolSpec(nil), tools...),
	})
	if len(s.replies) == 0 {
		return Reply{Text: ""}, nil
	}
	r := s.replies[0]
	s.replies = s.replies[1:]
	return r, nil
}

func TestAgent_PlainTextReplyShortCircuits(t *testing.T) {
	reg := NewRegistry()
	runner := NewRunner(reg, nil)
	prov := &scriptedProvider{replies: []Reply{{Text: "hello world"}}}

	a := NewAgent(reg, runner, prov)
	var buf bytes.Buffer
	out, err := a.Send(context.Background(), "hi", &buf)
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if out != "hello world" {
		t.Errorf("out = %q, want %q", out, "hello world")
	}
	if buf.String() != "hello world" {
		t.Errorf("writer = %q, want %q", buf.String(), "hello world")
	}
	if len(prov.calls) != 1 {
		t.Errorf("chat calls = %d, want 1", len(prov.calls))
	}
}

func TestAgent_ToolCallLoopsUntilText(t *testing.T) {
	// Two-turn conversation: model first calls the "echo" capability,
	// then produces a text reply that cites the tool output.
	reg := NewRegistry()
	if err := reg.Register(&onDemandCap{name: "echo", desc: "echo the input"}); err != nil {
		t.Fatalf("register: %v", err)
	}
	runner := NewRunner(reg, nil)
	prov := &scriptedProvider{
		replies: []Reply{
			{
				ToolCalls: []ToolCall{{
					ID:        "call_0",
					Name:      "echo",
					Arguments: map[string]any{"input": "ping"},
				}},
			},
			{Text: "tool said ping"},
		},
	}

	a := NewAgent(reg, runner, prov)
	var buf bytes.Buffer
	out, err := a.Send(context.Background(), "test", &buf)
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if out != "tool said ping" {
		t.Errorf("out = %q", out)
	}
	if len(prov.calls) != 2 {
		t.Fatalf("chat calls = %d, want 2", len(prov.calls))
	}
	// Second call should include the tool-result message.
	secondMsgs := prov.calls[1].msgs
	var sawToolResult bool
	for _, m := range secondMsgs {
		if m.Role == RoleTool && m.Name == "echo" {
			sawToolResult = true
			if !strings.Contains(m.Content, "ping") {
				t.Errorf("tool result missing payload: %q", m.Content)
			}
		}
	}
	if !sawToolResult {
		t.Errorf("second turn missing tool-result message: %+v", secondMsgs)
	}
}

func TestAgent_ToolCatalogFiltersOnDemandOnly(t *testing.T) {
	reg := NewRegistry()
	_ = reg.Register(&onDemandCap{name: "ask", desc: "answer"})
	_ = reg.Register(&fakeDaemon{name: "claude"})
	_ = reg.Register(&fakeInterval{name: "workspace"})
	runner := NewRunner(reg, nil)
	prov := &scriptedProvider{replies: []Reply{{Text: "ok"}}}

	a := NewAgent(reg, runner, prov)
	_, _ = a.Send(context.Background(), "hi", nil)
	if len(prov.calls) != 1 {
		t.Fatalf("calls = %d", len(prov.calls))
	}
	tools := prov.calls[0].tools
	if len(tools) != 1 || tools[0].Name != "ask" {
		t.Errorf("tools = %+v, want just ask", tools)
	}
}

func TestAgent_SystemPromptOnlyOnFirstTurn(t *testing.T) {
	reg := NewRegistry()
	runner := NewRunner(reg, nil)
	prov := &scriptedProvider{replies: []Reply{{Text: "a"}, {Text: "b"}}}

	a := NewAgent(reg, runner, prov)
	a.System = "be terse"

	_, _ = a.Send(context.Background(), "hi", nil)
	_, _ = a.Send(context.Background(), "again", nil)

	// Count system messages in the final history — should be exactly one.
	sysCount := 0
	for _, m := range a.History() {
		if m.Role == RoleSystem {
			sysCount++
		}
	}
	if sysCount != 1 {
		t.Errorf("system messages in history = %d, want 1", sysCount)
	}
}

func TestAgent_NoHardcodedSystemPrompt(t *testing.T) {
	// Guardrail test for the AI-first redesign: Agent must not inject
	// any default system turn. A Send with System="" should produce a
	// history that starts with the user message.
	reg := NewRegistry()
	runner := NewRunner(reg, nil)
	prov := &scriptedProvider{replies: []Reply{{Text: "ok"}}}

	a := NewAgent(reg, runner, prov)
	_, _ = a.Send(context.Background(), "hi", nil)
	if len(prov.calls) == 0 {
		t.Fatal("no calls recorded")
	}
	msgs := prov.calls[0].msgs
	if len(msgs) == 0 {
		t.Fatal("empty msgs")
	}
	if msgs[0].Role != RoleUser {
		t.Errorf("first message role = %q, want user (no hardcoded system turn)", msgs[0].Role)
	}
}

func TestAgent_ToolHopLimit(t *testing.T) {
	// Model keeps calling a tool forever — the loop must terminate
	// once MaxToolHops is exceeded instead of spinning.
	reg := NewRegistry()
	_ = reg.Register(&onDemandCap{name: "echo", desc: "echo"})
	runner := NewRunner(reg, nil)
	prov := &scriptedProvider{}
	// Return tool calls indefinitely.
	for i := 0; i < 20; i++ {
		prov.replies = append(prov.replies, Reply{
			ToolCalls: []ToolCall{{ID: "x", Name: "echo", Arguments: map[string]any{"input": "loop"}}},
		})
	}
	a := NewAgent(reg, runner, prov)
	a.MaxToolHops = 3
	_, err := a.Send(context.Background(), "start", nil)
	if !errors.Is(err, ErrToolHopLimit) {
		t.Fatalf("err = %v, want ErrToolHopLimit", err)
	}
}

func TestAgent_ToolErrorIsFedBackNotFatal(t *testing.T) {
	// Unknown capability → runner returns NotFoundError → the agent
	// surfaces it as a tool-result message rather than failing Send.
	reg := NewRegistry()
	runner := NewRunner(reg, nil)
	prov := &scriptedProvider{
		replies: []Reply{
			{ToolCalls: []ToolCall{{ID: "1", Name: "missing", Arguments: map[string]any{"input": "x"}}}},
			{Text: "recovered"},
		},
	}
	a := NewAgent(reg, runner, prov)
	out, err := a.Send(context.Background(), "go", nil)
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if out != "recovered" {
		t.Errorf("out = %q", out)
	}
	if len(prov.calls) != 2 {
		t.Fatalf("calls = %d", len(prov.calls))
	}
	// The second turn should see the error as a tool-result message.
	secondMsgs := prov.calls[1].msgs
	var sawErr bool
	for _, m := range secondMsgs {
		if m.Role == RoleTool && strings.Contains(m.Content, "error:") {
			sawErr = true
		}
	}
	if !sawErr {
		t.Errorf("expected tool-result error message: %+v", secondMsgs)
	}
}
