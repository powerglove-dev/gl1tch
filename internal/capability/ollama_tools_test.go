package capability

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

// fakeOllamaTools is a tiny httptest server that records the last /api/chat
// request and returns a canned response shape. Separate from fakeOllama in
// router_test.go because this one has to speak the tools-enabled schema.
type fakeOllamaTools struct {
	srv         *httptest.Server
	lastRequest ollamaChatReq
	reply       ollamaChatResp
}

func newFakeOllamaTools(t *testing.T) *fakeOllamaTools {
	f := &fakeOllamaTools{}
	f.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/chat" {
			http.NotFound(w, r)
			return
		}
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &f.lastRequest)
		_ = json.NewEncoder(w).Encode(f.reply)
	}))
	t.Cleanup(f.srv.Close)
	return f
}

func TestOllamaToolProvider_ForwardsToolsInRequest(t *testing.T) {
	fake := newFakeOllamaTools(t)
	fake.reply = ollamaChatResp{Message: ollamaChatMsg{Role: "assistant", Content: "ok"}}

	prov := &OllamaToolProvider{BaseURL: fake.srv.URL, HTTPClient: fake.srv.Client()}
	msgs := []Message{{Role: RoleUser, Content: "hi"}}
	tools := []ToolSpec{
		{Name: "echo", Description: "echo input"},
		{Name: "summarize", Description: "summarize text"},
	}

	reply, err := prov.Chat(context.Background(), msgs, tools)
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if reply.Text != "ok" {
		t.Errorf("text = %q", reply.Text)
	}
	if len(fake.lastRequest.Tools) != 2 {
		t.Fatalf("sent %d tools, want 2", len(fake.lastRequest.Tools))
	}
	// Every tool must be a "function" with an input-string schema.
	for _, tl := range fake.lastRequest.Tools {
		if tl.Type != "function" {
			t.Errorf("tool type = %q, want function", tl.Type)
		}
		if tl.Function.Name == "" {
			t.Errorf("tool missing name")
		}
		props, _ := tl.Function.Parameters["properties"].(map[string]any)
		if _, ok := props["input"]; !ok {
			t.Errorf("tool %q missing input property: %+v", tl.Function.Name, tl.Function.Parameters)
		}
	}
}

func TestOllamaToolProvider_ParsesToolCalls(t *testing.T) {
	fake := newFakeOllamaTools(t)
	fake.reply = ollamaChatResp{
		Message: ollamaChatMsg{
			Role: "assistant",
			ToolCalls: []ollamaToolCallOut{{
				Function: ollamaToolCallOutFunc{
					Name:      "echo",
					Arguments: map[string]any{"input": "hello"},
				},
			}},
		},
	}

	prov := &OllamaToolProvider{BaseURL: fake.srv.URL, HTTPClient: fake.srv.Client()}
	reply, err := prov.Chat(context.Background(), []Message{{Role: RoleUser, Content: "hi"}}, nil)
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if len(reply.ToolCalls) != 1 {
		t.Fatalf("tool calls = %d", len(reply.ToolCalls))
	}
	tc := reply.ToolCalls[0]
	if tc.Name != "echo" {
		t.Errorf("name = %q", tc.Name)
	}
	if tc.ID == "" {
		t.Errorf("id was not synthesised for Ollama call")
	}
	if got, _ := tc.Arguments["input"].(string); got != "hello" {
		t.Errorf("arg input = %q", got)
	}
}

func TestOllamaToolProvider_RoundtripsToolResultMessage(t *testing.T) {
	fake := newFakeOllamaTools(t)
	fake.reply = ollamaChatResp{Message: ollamaChatMsg{Role: "assistant", Content: "done"}}

	prov := &OllamaToolProvider{BaseURL: fake.srv.URL, HTTPClient: fake.srv.Client()}
	msgs := []Message{
		{Role: RoleUser, Content: "hi"},
		{Role: RoleAssistant, ToolCalls: []ToolCall{{ID: "call_0", Name: "echo", Arguments: map[string]any{"input": "x"}}}},
		{Role: RoleTool, Content: "x", ToolCallID: "call_0", Name: "echo"},
	}
	_, err := prov.Chat(context.Background(), msgs, nil)
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if n := len(fake.lastRequest.Messages); n != 3 {
		t.Fatalf("sent %d msgs, want 3", n)
	}
	toolMsg := fake.lastRequest.Messages[2]
	if toolMsg.Role != "tool" || toolMsg.Name != "echo" || toolMsg.Content != "x" {
		t.Errorf("tool msg = %+v", toolMsg)
	}
	assistantMsg := fake.lastRequest.Messages[1]
	if len(assistantMsg.ToolCalls) != 1 || assistantMsg.ToolCalls[0].Function.Name != "echo" {
		t.Errorf("assistant msg tool_calls = %+v", assistantMsg.ToolCalls)
	}
}
