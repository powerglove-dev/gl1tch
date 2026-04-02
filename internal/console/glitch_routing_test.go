package console

import (
	"context"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/8op-org/gl1tch/internal/pipeline"
	"github.com/8op-org/gl1tch/internal/router"
)

// ── mock backend ─────────────────────────────────────────────────────────────

type mockGlitchBackend struct {
	tokens []string
}

func (m *mockGlitchBackend) name() string { return "mock" }

func (m *mockGlitchBackend) streamIntro(_ context.Context, _ string) (<-chan string, error) {
	ch := make(chan string, len(m.tokens))
	for _, t := range m.tokens {
		ch <- t
	}
	close(ch)
	return ch, nil
}

func (m *mockGlitchBackend) stream(_ context.Context, _ []glitchTurn, _, _, _ string) (<-chan string, error) {
	ch := make(chan string, len(m.tokens))
	for _, t := range m.tokens {
		ch <- t
	}
	close(ch)
	return ch, nil
}

// ── intent routing tests ──────────────────────────────────────────────────────

// TestGlitchPanel_IntentMsg_ResetsRoutingFlag is a regression test for the bug
// where glitchIntentMsg was not forwarded to the panel by the deck,
// leaving p.routing permanently true and blocking all subsequent Enter presses.
func TestGlitchPanel_IntentMsg_ResetsRoutingFlag(t *testing.T) {
	p := newTestPanel()
	p.routing = true

	p2, _ := p.update(glitchIntentMsg{result: nil, prompt: "hello", turns: nil})
	if p2.routing {
		t.Error("routing should be false after glitchIntentMsg — panel stuck if this fails")
	}
}

// TestGlitchPanel_IntentMsg_NoBackend_ShowsError verifies that when no
// provider is configured, glitchIntentMsg shows a helpful error message.
func TestGlitchPanel_IntentMsg_NoBackend_ShowsError(t *testing.T) {
	p := newTestPanel()
	p.backend = nil // force no-backend path regardless of local Ollama availability
	p2, _ := p.update(glitchIntentMsg{result: nil, prompt: "what can you do", turns: nil})

	if len(p2.messages) == 0 {
		t.Fatal("expected at least one message after glitchIntentMsg with no backend")
	}
	last := p2.messages[len(p2.messages)-1]
	if !strings.Contains(last.text, "no provider") {
		t.Errorf("expected 'no provider' error message, got: %q", last.text)
	}
}

// TestGlitchPanel_IntentMsg_WithBackend_SetsStreaming verifies that when a
// backend is available, glitchIntentMsg kicks off the LLM stream.
func TestGlitchPanel_IntentMsg_WithBackend_SetsStreaming(t *testing.T) {
	p := newTestPanel()
	p.backend = &mockGlitchBackend{tokens: []string{"hi", " there"}}

	p2, cmd := p.update(glitchIntentMsg{result: nil, prompt: "hello", turns: nil})
	if !p2.streaming {
		t.Error("expected streaming=true after glitchIntentMsg with available backend")
	}
	if cmd == nil {
		t.Error("expected non-nil cmd to kick off token stream")
	}
}

// TestGlitchPanel_IntentMsg_PipelineMatch_ReturnsRerunMsg verifies that when
// intent routing finds a matching pipeline, the panel returns a cmd whose
// message is a glitchRerunMsg directed at the deck.
func TestGlitchPanel_IntentMsg_PipelineMatch_ReturnsRerunMsg(t *testing.T) {
	p := newTestPanel()
	routeResult := &router.RouteResult{
		Pipeline: &pipeline.PipelineRef{Name: "my-pipe"},
		Input:    "test input",
	}

	_, cmd := p.update(glitchIntentMsg{result: routeResult, prompt: "run my-pipe", turns: nil})
	if cmd == nil {
		t.Fatal("expected non-nil cmd when pipeline matched")
	}
	msg := cmd()
	rerun, ok := msg.(glitchRerunMsg)
	if !ok {
		t.Fatalf("expected glitchRerunMsg from cmd, got %T", msg)
	}
	if rerun.name != "my-pipe" {
		t.Errorf("expected rerun.name %q, got %q", "my-pipe", rerun.name)
	}
}

// TestGlitchPanel_StreamTokens_AccumulateAndFinalize verifies the full
// stream lifecycle: tokens accumulate in streamBuf, glitchDoneMsg finalizes
// the message and clears streaming state.
func TestGlitchPanel_StreamTokens_AccumulateAndFinalize(t *testing.T) {
	p := newTestPanel()
	ch := make(chan string, 2)
	ch <- " world"
	close(ch)

	// Deliver first token.
	p2, _ := p.update(glitchStreamMsg{token: "hello", ch: ch})
	if !strings.Contains(p2.streamBuf, "hello") {
		t.Errorf("expected streamBuf to contain 'hello', got: %q", p2.streamBuf)
	}

	// Finalize stream.
	p3, _ := p2.update(glitchDoneMsg{})
	if p3.streaming {
		t.Error("expected streaming=false after glitchDoneMsg")
	}
	if len(p3.messages) == 0 {
		t.Fatal("expected at least one message after stream completes")
	}
	last := p3.messages[len(p3.messages)-1]
	if !strings.Contains(last.text, "hello") {
		t.Errorf("expected final message to contain 'hello', got: %q", last.text)
	}
}

// TestGlitchPanel_InitCmd_AlwaysReturnsCmd verifies that initCmd never
// returns nil — the user should always see a welcome or ready message.
func TestGlitchPanel_InitCmd_AlwaysReturnsCmd(t *testing.T) {
	dir := t.TempDir()

	// Case 1: first run, no backend.
	p := newTestPanel()
	p.cfgDir = dir
	cmd := p.initCmd()
	if cmd == nil {
		t.Error("initCmd should return a cmd even with no backend on first run")
	}

	// Case 2: not first run (sentinel already written), no backend.
	p2 := newTestPanel()
	p2.cfgDir = dir
	cmd2 := p2.initCmd()
	if cmd2 == nil {
		t.Error("initCmd should return a ready-message cmd on non-first run")
	}
	// Verify it dispatches a narration message (or a batch that contains one).
	msg := cmd2()
	switch m := msg.(type) {
	case glitchNarrationMsg:
		if m.text == "" {
			t.Error("expected non-empty narration text")
		}
	case tea.BatchMsg:
		// initCmd now returns a tea.Batch([narration, watcher]) — execute each
		// sub-command and look for a narration among the results.
		found := false
		for _, sub := range m {
			if sub == nil {
				continue
			}
			if nm, ok := sub().(glitchNarrationMsg); ok && nm.text != "" {
				found = true
				break
			}
		}
		if !found {
			t.Error("expected glitchNarrationMsg within BatchMsg from non-first-run initCmd")
		}
	default:
		t.Fatalf("expected glitchNarrationMsg or BatchMsg from non-first-run initCmd, got %T", msg)
	}
}
