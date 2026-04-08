package capability

import (
	"context"
	"strings"
	"testing"
	"time"
)

// TestSecurityAlertsCapability_EmptyWindow covers the "nothing to
// report" path. The assistant should get a clear message instead of
// silence, otherwise a clean system looks identical to a broken
// capability.
func TestSecurityAlertsCapability_EmptyWindow(t *testing.T) {
	cap := &SecurityAlertsCapability{
		Searcher: func(_ context.Context, _ map[string]any) ([]map[string]any, error) {
			return nil, nil
		},
	}
	out := drainStream(t, cap, Input{})
	if !strings.Contains(out, "no security alerts") {
		t.Errorf("out = %q, want 'no security alerts'", out)
	}
}

// TestSecurityAlertsCapability_FakeSSHBreach is the core scenario:
// the searcher returns a fake SSH breach doc and the capability
// formats it into a line the assistant can read and relay.
func TestSecurityAlertsCapability_FakeSSHBreach(t *testing.T) {
	cap := &SecurityAlertsCapability{
		Searcher: func(_ context.Context, _ map[string]any) ([]map[string]any, error) {
			return []map[string]any{
				{
					"timestamp":  "2026-04-08T12:34:56Z",
					"severity":   "critical",
					"event_type": "ssh.auth_failure",
					"user":       "root",
					"source_ip":  "203.0.113.42",
					"message":    "Failed password for root from 203.0.113.42 port 55123 ssh2",
				},
			}, nil
		},
	}
	out := drainStream(t, cap, Input{})
	if !strings.Contains(out, "ssh.auth_failure") {
		t.Errorf("missing event_type: %q", out)
	}
	if !strings.Contains(out, "203.0.113.42") {
		t.Errorf("missing source_ip: %q", out)
	}
	if !strings.Contains(out, "critical") {
		t.Errorf("missing severity: %q", out)
	}
}

// TestSecurityAlertsCapability_AgentSurface proves the end-to-end
// desktop-chat scenario: the Agent calls the security_alerts tool
// and receives the formatted fake-breach summary as a tool result
// on its next turn. This is what the user will see when they ask
// glitch "any security alerts?" after the breach fixture has been
// injected.
func TestSecurityAlertsCapability_AgentSurface(t *testing.T) {
	secCap := &SecurityAlertsCapability{
		Searcher: func(_ context.Context, _ map[string]any) ([]map[string]any, error) {
			return []map[string]any{
				{
					"timestamp":  "2026-04-08T12:34:56Z",
					"severity":   "critical",
					"event_type": "ssh.auth_failure",
					"user":       "root",
					"source_ip":  "203.0.113.42",
					"message":    "Failed password for root",
				},
			}, nil
		},
	}

	reg := NewRegistry()
	if err := reg.Register(secCap); err != nil {
		t.Fatalf("register: %v", err)
	}
	runner := NewRunner(reg, nil)

	prov := &scriptedProvider{replies: []Reply{
		{ToolCalls: []ToolCall{{
			ID:        "c0",
			Name:      "security_alerts",
			Arguments: map[string]any{"input": ""},
		}}},
		{Text: "one critical ssh.auth_failure from 203.0.113.42 — looks like a root brute force"},
	}}
	agent := NewAgent(reg, runner, prov)

	out, err := agent.Send(context.Background(), "any security alerts?", nil)
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if !strings.Contains(out, "203.0.113.42") {
		t.Errorf("final reply missing breach source: %q", out)
	}

	// Tool-result message on the second turn must carry the raw
	// capability output verbatim, so the model can reason about it.
	if len(prov.calls) != 2 {
		t.Fatalf("calls = %d", len(prov.calls))
	}
	var toolResult string
	for _, m := range prov.calls[1].msgs {
		if m.Role == RoleTool && m.Name == "security_alerts" {
			toolResult = m.Content
		}
	}
	if !strings.Contains(toolResult, "ssh.auth_failure") {
		t.Errorf("tool result missing event_type: %q", toolResult)
	}
	if !strings.Contains(toolResult, "critical") {
		t.Errorf("tool result missing severity: %q", toolResult)
	}
}

// TestSecurityAlertsCapability_CustomWindow verifies that passing a
// duration via Input.Stdin is honoured — matches how the Agent packs
// the "input" argument from the model's tool call.
func TestSecurityAlertsCapability_CustomWindow(t *testing.T) {
	var seen map[string]any
	cap := &SecurityAlertsCapability{
		Searcher: func(_ context.Context, q map[string]any) ([]map[string]any, error) {
			seen = q
			return nil, nil
		},
	}
	_ = drainStream(t, cap, Input{Stdin: "1h"})
	if seen == nil {
		t.Fatal("searcher not called")
	}
	// Peel open the query to find the timestamp gte — it should be
	// within ~1 hour of now, not 24.
	boolq, _ := seen["query"].(map[string]any)["bool"].(map[string]any)
	filter, _ := boolq["filter"].([]any)
	var gte string
	for _, f := range filter {
		fm, ok := f.(map[string]any)
		if !ok {
			continue
		}
		rng, ok := fm["range"].(map[string]any)
		if !ok {
			continue
		}
		ts, ok := rng["timestamp"].(map[string]any)
		if !ok {
			continue
		}
		if v, ok := ts["gte"].(string); ok {
			gte = v
		}
	}
	if gte == "" {
		t.Fatal("no gte filter in query")
	}
	parsed, err := time.Parse(time.RFC3339, gte)
	if err != nil {
		t.Fatalf("parse gte %q: %v", gte, err)
	}
	delta := time.Since(parsed)
	if delta < 30*time.Minute || delta > 90*time.Minute {
		t.Errorf("gte window ~%s, expected ~1h", delta)
	}
}

// drainStream invokes a capability once and returns its combined
// stream output as a single string. Matches how runOnce collects
// Stream events in production without importing the Runner.
func drainStream(t *testing.T, c Capability, in Input) string {
	t.Helper()
	ch, err := c.Invoke(context.Background(), in)
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	var sb strings.Builder
	for ev := range ch {
		if ev.Kind == EventStream {
			sb.WriteString(ev.Text)
		}
	}
	return sb.String()
}
