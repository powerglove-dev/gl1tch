package capability

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// smoke_test.go drives end-to-end "desktop chat" scenarios through the new
// capability.Agent so we have confidence that the AI-first stack works
// across multiple workspaces without any hardcoded pre-context. These
// tests simulate the user opening glitch-desktop, switching between
// workspaces, and chatting with the assistant — except the provider is
// a scripted stub instead of a real Ollama server so the suite runs
// anywhere.
//
// The key invariants being guarded:
//
//  1. The agent discovers workspace state through tool calls, not
//     through any system-prompt preamble that gl1tch prepended.
//  2. Conversation history carries across multiple Send calls on the
//     same Agent, so a desktop chat session feels stateful.
//  3. Switching workspaces means switching which capability set the
//     agent has access to — nothing from workspace A leaks into a chat
//     rooted in workspace B.

// workspaceFixture is a temp directory pretending to be a desktop
// workspace, with a seeded set of files the capabilities below serve to
// the agent. Everything in here is self-contained per-test so we do not
// collide on /tmp.
type workspaceFixture struct {
	root  string
	files map[string]string
}

func newWorkspace(t *testing.T, name string, files map[string]string) *workspaceFixture {
	t.Helper()
	dir := t.TempDir()
	root := filepath.Join(dir, name)
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatalf("mkdir workspace %q: %v", name, err)
	}
	for rel, body := range files {
		full := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatalf("mkdir %q: %v", filepath.Dir(full), err)
		}
		if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
			t.Fatalf("write %q: %v", rel, err)
		}
	}
	return &workspaceFixture{root: root, files: files}
}

// listFilesCap is an on-demand capability that lists the files in a
// fixed workspace directory. It is the smoke-test analogue of a real
// "workspace.ls" capability: a Go-native implementation that serves its
// output as a Stream event. The agent calls it via tool-use, the runner
// buffers the stream, and the result lands back in the model's history
// as a tool-result message.
type listFilesCap struct {
	workspace string
}

func (c *listFilesCap) Manifest() Manifest {
	return Manifest{
		Name:        "list_files",
		Description: "List the files in the current workspace directory, one per line.",
		Trigger:     Trigger{Mode: TriggerOnDemand},
		Sink:        Sink{Stream: true},
	}
}

func (c *listFilesCap) Invoke(_ context.Context, _ Input) (<-chan Event, error) {
	ch := make(chan Event, 4)
	go func() {
		defer close(ch)
		entries, err := os.ReadDir(c.workspace)
		if err != nil {
			ch <- Event{Kind: EventError, Err: err}
			return
		}
		names := make([]string, 0, len(entries))
		for _, e := range entries {
			names = append(names, e.Name())
		}
		sort.Strings(names)
		ch <- Event{Kind: EventStream, Text: strings.Join(names, "\n")}
	}()
	return ch, nil
}

// readFileCap serves the contents of a single file relative to the
// workspace root. The path comes from the tool-call arguments via
// Input.Stdin — matching how OllamaToolProvider packs the "input"
// argument of every tool call. This is the minimal shape a real
// workspace.read capability would take.
type readFileCap struct {
	workspace string
}

func (c *readFileCap) Manifest() Manifest {
	return Manifest{
		Name:        "read_file",
		Description: "Read a file from the current workspace. Input is the relative path.",
		Trigger:     Trigger{Mode: TriggerOnDemand},
		Sink:        Sink{Stream: true},
	}
}

func (c *readFileCap) Invoke(_ context.Context, in Input) (<-chan Event, error) {
	ch := make(chan Event, 4)
	go func() {
		defer close(ch)
		rel := strings.TrimSpace(in.Stdin)
		if rel == "" {
			ch <- Event{Kind: EventError, Err: fmt.Errorf("read_file: empty path")}
			return
		}
		data, err := os.ReadFile(filepath.Join(c.workspace, rel))
		if err != nil {
			ch <- Event{Kind: EventError, Err: err}
			return
		}
		ch <- Event{Kind: EventStream, Text: string(data)}
	}()
	return ch, nil
}

// buildWorkspaceAgent wires a registry + runner + scripted ToolProvider
// into an Agent rooted at the given workspace. This is the smoke-test
// equivalent of what glitch-desktop does on a "switch workspace" event:
// build the agent from the current workspace's capability set, keep
// nothing from the previous one.
func buildWorkspaceAgent(t *testing.T, ws *workspaceFixture, replies []Reply) (*Agent, *scriptedProvider) {
	t.Helper()
	reg := NewRegistry()
	if err := reg.Register(&listFilesCap{workspace: ws.root}); err != nil {
		t.Fatalf("register list_files: %v", err)
	}
	if err := reg.Register(&readFileCap{workspace: ws.root}); err != nil {
		t.Fatalf("register read_file: %v", err)
	}
	runner := NewRunner(reg, nil)
	prov := &scriptedProvider{replies: replies}
	agent := NewAgent(reg, runner, prov)
	return agent, prov
}

// TestSmoke_ChatDiscoversWorkspaceViaTools is the single-workspace
// happy-path: the user asks "what's in my workspace?"; the model uses
// the list_files tool to answer; the agent forwards the final text to
// the caller's writer. Mirrors the first thing a desktop user would
// type after opening a new workspace.
func TestSmoke_ChatDiscoversWorkspaceViaTools(t *testing.T) {
	ws := newWorkspace(t, "proj-a", map[string]string{
		"README.md": "# proj-a\nthis is the readme",
		"main.go":   "package main",
	})

	agent, prov := buildWorkspaceAgent(t, ws, []Reply{
		{ToolCalls: []ToolCall{{ID: "c0", Name: "list_files", Arguments: map[string]any{"input": ""}}}},
		{Text: "your workspace has README.md and main.go"},
	})

	var buf strings.Builder
	out, err := agent.Send(context.Background(), "what files are in here?", &buf)
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if !strings.Contains(out, "README.md") || !strings.Contains(out, "main.go") {
		t.Errorf("answer missing workspace files: %q", out)
	}

	// The model's second turn must have seen the tool result from the
	// first turn. That is the whole chain of "agent picks up workspace
	// state via tools" the user cares about.
	if len(prov.calls) != 2 {
		t.Fatalf("chat calls = %d", len(prov.calls))
	}
	var toolResult string
	for _, m := range prov.calls[1].msgs {
		if m.Role == RoleTool && m.Name == "list_files" {
			toolResult = m.Content
		}
	}
	if !strings.Contains(toolResult, "README.md") || !strings.Contains(toolResult, "main.go") {
		t.Errorf("list_files tool result lost workspace state: %q", toolResult)
	}
}

// TestSmoke_ChatReadsFileContentAcrossTurns exercises a two-step tool
// chain the way a desktop user would: "list the files", then "what's
// in README.md?". The second model turn calls read_file with the path
// the first turn produced.
func TestSmoke_ChatReadsFileContentAcrossTurns(t *testing.T) {
	ws := newWorkspace(t, "proj-b", map[string]string{
		"notes.md": "secret marker: hedgehog",
	})

	agent, _ := buildWorkspaceAgent(t, ws, []Reply{
		// Turn 1: user asks "what's here" — model lists.
		{ToolCalls: []ToolCall{{ID: "c0", Name: "list_files", Arguments: map[string]any{"input": ""}}}},
		{Text: "only notes.md"},
		// Turn 2: user asks "what's in notes.md" — model reads.
		{ToolCalls: []ToolCall{{ID: "c1", Name: "read_file", Arguments: map[string]any{"input": "notes.md"}}}},
		{Text: "the marker is hedgehog"},
	})

	if _, err := agent.Send(context.Background(), "what's here", nil); err != nil {
		t.Fatalf("Send1: %v", err)
	}
	out, err := agent.Send(context.Background(), "read notes.md", nil)
	if err != nil {
		t.Fatalf("Send2: %v", err)
	}
	if !strings.Contains(out, "hedgehog") {
		t.Errorf("second turn missed file content: %q", out)
	}

	// History must include both user turns — that's how a desktop chat
	// feels stateful across sends.
	userTurns := 0
	for _, m := range agent.History() {
		if m.Role == RoleUser {
			userTurns++
		}
	}
	if userTurns != 2 {
		t.Errorf("user turns in history = %d, want 2", userTurns)
	}
}

// TestSmoke_ChatSwitchingWorkspaces simulates the user switching
// between two desktop workspaces. Each workspace gets its own fresh
// Agent (matches how glitch-desktop's RunChain/StreamPrompt paths
// already operate: context is per-invocation, per-workspace). The
// invariant under test is that a chat rooted in workspace A cannot
// see workspace B's files, because the tools it was handed only know
// about A. This is the safety property behind the "don't bake a
// system prompt that mentions every workspace" redesign.
func TestSmoke_ChatSwitchingWorkspaces(t *testing.T) {
	wsA := newWorkspace(t, "proj-alpha", map[string]string{
		"alpha-only.txt": "from alpha",
	})
	wsB := newWorkspace(t, "proj-beta", map[string]string{
		"beta-only.txt": "from beta",
	})

	agentA, _ := buildWorkspaceAgent(t, wsA, []Reply{
		{ToolCalls: []ToolCall{{ID: "c0", Name: "list_files", Arguments: map[string]any{"input": ""}}}},
		{Text: "workspace A files listed"},
	})
	agentB, _ := buildWorkspaceAgent(t, wsB, []Reply{
		{ToolCalls: []ToolCall{{ID: "c0", Name: "list_files", Arguments: map[string]any{"input": ""}}}},
		{Text: "workspace B files listed"},
	})

	if _, err := agentA.Send(context.Background(), "what do I have", nil); err != nil {
		t.Fatalf("agentA.Send: %v", err)
	}
	if _, err := agentB.Send(context.Background(), "what do I have", nil); err != nil {
		t.Fatalf("agentB.Send: %v", err)
	}

	// The tool-result messages in each agent's history must only
	// mention their own workspace's files. No cross-contamination.
	aSawAlpha, aSawBeta := hasToolResult(agentA.History(), "alpha-only.txt"), hasToolResult(agentA.History(), "beta-only.txt")
	bSawAlpha, bSawBeta := hasToolResult(agentB.History(), "alpha-only.txt"), hasToolResult(agentB.History(), "beta-only.txt")

	if !aSawAlpha {
		t.Errorf("agent A did not see its own workspace files")
	}
	if aSawBeta {
		t.Errorf("agent A leaked workspace B content")
	}
	if !bSawBeta {
		t.Errorf("agent B did not see its own workspace files")
	}
	if bSawAlpha {
		t.Errorf("agent B leaked workspace A content")
	}
}

// TestSmoke_ChatNoHardcodedSystemContext is the guardrail for the
// "we shouldn't be injecting hardcoded pre-context messages everywhere"
// rule. It runs a full desktop-style chat conversation and asserts the
// provider was never handed a system message containing any of the
// old preamble markers: workflow YAML boilerplate, executor catalogs,
// or the GL1TCH persona prose. If this test fails, something has
// re-introduced a hardcoded pre-context somewhere in the path.
func TestSmoke_ChatNoHardcodedSystemContext(t *testing.T) {
	ws := newWorkspace(t, "proj-guard", map[string]string{
		"x.txt": "y",
	})
	agent, prov := buildWorkspaceAgent(t, ws, []Reply{
		{Text: "hi"},
	})

	if _, err := agent.Send(context.Background(), "hello", nil); err != nil {
		t.Fatalf("Send: %v", err)
	}
	if len(prov.calls) == 0 {
		t.Fatal("no calls recorded")
	}

	// Forbidden substrings — anything that was in BuildSystemContext,
	// glitchSystemPrompt, or glitchctx.BuildShellContext. If any of
	// these appear in a message the agent sent to the provider, a
	// preamble has crept back in.
	forbidden := []string{
		"You are gl1tch",
		"Available Executors",
		"Workflow YAML Format",
		"Response Rules",
		"GL1TCH — a veteran underground hacker",
		"BBS scene",
		"Shell Environment",
		"Directory Contents",
	}
	for _, call := range prov.calls {
		for _, m := range call.msgs {
			for _, bad := range forbidden {
				if strings.Contains(m.Content, bad) {
					t.Errorf("forbidden preamble %q appeared in %s message: %q", bad, m.Role, m.Content)
				}
			}
		}
	}
}

// TestSmoke_ChatPersonaIsOptAndExplicit proves the only legitimate
// way to get a system turn into the chat is to set Agent.System
// explicitly (loaded from a user-owned file, say ~/.config/glitch/persona.md).
// The default must be no system turn at all.
func TestSmoke_ChatPersonaIsOptAndExplicit(t *testing.T) {
	ws := newWorkspace(t, "proj-persona", map[string]string{"x.txt": "y"})

	// Default agent — no system turn.
	agent, prov := buildWorkspaceAgent(t, ws, []Reply{{Text: "hi"}})
	_, _ = agent.Send(context.Background(), "ping", nil)
	for _, m := range prov.calls[0].msgs {
		if m.Role == RoleSystem {
			t.Errorf("default agent should have no system turn, got: %q", m.Content)
		}
	}

	// Explicitly opted-in persona — exactly one system turn, matching
	// what the caller provided.
	agent2, prov2 := buildWorkspaceAgent(t, ws, []Reply{{Text: "hi"}})
	agent2.System = "custom persona body"
	_, _ = agent2.Send(context.Background(), "ping", nil)
	sysCount := 0
	var sysContent string
	for _, m := range prov2.calls[0].msgs {
		if m.Role == RoleSystem {
			sysCount++
			sysContent = m.Content
		}
	}
	if sysCount != 1 {
		t.Errorf("opt-in persona: system turns = %d, want 1", sysCount)
	}
	if sysContent != "custom persona body" {
		t.Errorf("persona content = %q, want exact caller-provided body", sysContent)
	}
}

// hasToolResult reports whether any tool-result message in the history
// contains s. Used by the workspace-switching test to assert isolation.
func hasToolResult(hist []Message, s string) bool {
	for _, m := range hist {
		if m.Role == RoleTool && strings.Contains(m.Content, s) {
			return true
		}
	}
	return false
}
