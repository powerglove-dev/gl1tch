package console

import (
	"testing"
	"time"
)

// ── helpers ───────────────────────────────────────────────────────────────────

// blankModel returns a Model with no store, no bus, and a fresh glitchChatPanel.
// Sufficient for testing narration gate logic without any I/O.
func blankModel() Model {
	return Model{
		glitchChat: newGlitchPanel("", nil, nil, "", nil),
	}
}

// ── narrationAllowed ──────────────────────────────────────────────────────────

func TestNarrationAllowed_DefaultIsTrue(t *testing.T) {
	m := blankModel()
	if !m.narrationAllowed() {
		t.Error("narrationAllowed should return true on a fresh model")
	}
}

func TestNarrationAllowed_BlockedWhileStreaming(t *testing.T) {
	m := blankModel()
	m.glitchChat.streaming = true
	if m.narrationAllowed() {
		t.Error("narrationAllowed should return false while panel is streaming")
	}
}

func TestNarrationAllowed_BlockedWhileRouting(t *testing.T) {
	m := blankModel()
	m.glitchChat.routing = true
	if m.narrationAllowed() {
		t.Error("narrationAllowed should return false while panel is routing")
	}
}

func TestNarrationAllowed_BlockedWithinUserMsgWindow(t *testing.T) {
	m := blankModel()
	m.lastUserMsgAt = time.Now()
	if m.narrationAllowed() {
		t.Error("narrationAllowed should return false within 30s of last user message")
	}
}

func TestNarrationAllowed_AllowedAfterUserMsgWindow(t *testing.T) {
	m := blankModel()
	m.lastUserMsgAt = time.Now().Add(-31 * time.Second)
	if !m.narrationAllowed() {
		t.Error("narrationAllowed should return true after the 30s user-message window")
	}
}

func TestNarrationAllowed_BlockedWithinCooldown(t *testing.T) {
	m := blankModel()
	m.lastNarrationAt = time.Now()
	if m.narrationAllowed() {
		t.Error("narrationAllowed should return false within 5s of last narration")
	}
}

func TestNarrationAllowed_AllowedAfterCooldown(t *testing.T) {
	m := blankModel()
	m.lastNarrationAt = time.Now().Add(-6 * time.Second)
	if !m.narrationAllowed() {
		t.Error("narrationAllowed should return true after the 5s narration cooldown")
	}
}

func TestNarrationAllowed_BlockedInBusyMode(t *testing.T) {
	m := blankModel()
	m.recentRunCount = 2
	m.runWindowStart = time.Now()
	if m.narrationAllowed() {
		t.Error("narrationAllowed should return false in busy mode (≥2 runs/60s)")
	}
}

func TestNarrationAllowed_AllowedAfterBusyWindow(t *testing.T) {
	m := blankModel()
	m.recentRunCount = 2
	m.runWindowStart = time.Now().Add(-61 * time.Second)
	if !m.narrationAllowed() {
		t.Error("narrationAllowed should return true after the 60s busy window expires")
	}
}

// ── gameNarrationAllowed ──────────────────────────────────────────────────────

func TestGameNarrationAllowed_DefaultIsTrue(t *testing.T) {
	m := blankModel()
	if !m.gameNarrationAllowed() {
		t.Error("gameNarrationAllowed should return true on a fresh model")
	}
}

func TestGameNarrationAllowed_BlockedWhileStreaming(t *testing.T) {
	m := blankModel()
	m.glitchChat.streaming = true
	if m.gameNarrationAllowed() {
		t.Error("gameNarrationAllowed should return false while panel is streaming")
	}
}

func TestGameNarrationAllowed_BlockedWhileRouting(t *testing.T) {
	m := blankModel()
	m.glitchChat.routing = true
	if m.gameNarrationAllowed() {
		t.Error("gameNarrationAllowed should return false while panel is routing")
	}
}

// This is the key regression test: game narration must NOT be blocked by the
// 30s user-message window. Previously narrationAllowed was used for both
// run-analysis and game narration, causing fast Ollama responses to be
// silently dropped when the user had recently triggered a pipeline.
func TestGameNarrationAllowed_NotBlockedByRecentUserMessage(t *testing.T) {
	m := blankModel()
	m.lastUserMsgAt = time.Now() // user just sent a message
	if !m.gameNarrationAllowed() {
		t.Error("gameNarrationAllowed must NOT be blocked by lastUserMsgAt — game events are async and independent of conversation state")
	}
}

// Game narration should also not be blocked by the busy-mode or cooldown
// gates — those are for unsolicited run-analysis chatter, not scored events.
func TestGameNarrationAllowed_NotBlockedByCooldown(t *testing.T) {
	m := blankModel()
	m.lastNarrationAt = time.Now()
	if !m.gameNarrationAllowed() {
		t.Error("gameNarrationAllowed must not be blocked by narration cooldown")
	}
}

func TestGameNarrationAllowed_NotBlockedInBusyMode(t *testing.T) {
	m := blankModel()
	m.recentRunCount = 5
	m.runWindowStart = time.Now()
	if !m.gameNarrationAllowed() {
		t.Error("gameNarrationAllowed must not be blocked by busy mode")
	}
}

// ── Update routing ────────────────────────────────────────────────────────────

// TestNarrationMsg_AllowedWhenIdle verifies that a glitchNarrationMsg delivered
// to an idle deck is forwarded to the panel and appears as a chat message.
func TestNarrationMsg_AllowedWhenIdle(t *testing.T) {
	m := blankModel()
	_, _ = m.Update(glitchNarrationMsg{text: "xp gained"})
	// Forward to panel via Update — we check messages on the returned model.
	result, _ := m.Update(glitchNarrationMsg{text: "xp gained"})
	rm := result.(Model)
	msgs := rm.glitchChat.messages
	found := false
	for _, msg := range msgs {
		if msg.text == "xp gained" {
			found = true
			break
		}
	}
	if !found {
		t.Error("narration message should appear in chat when panel is idle")
	}
}

// TestNarrationMsg_DroppedWhileStreaming verifies that a glitchNarrationMsg is
// silently dropped when the panel is mid-stream (active LLM response in progress).
func TestNarrationMsg_DroppedWhileStreaming(t *testing.T) {
	m := blankModel()
	m.glitchChat.streaming = true
	result, _ := m.Update(glitchNarrationMsg{text: "xp gained"})
	rm := result.(Model)
	for _, msg := range rm.glitchChat.messages {
		if msg.text == "xp gained" {
			t.Error("narration message should be dropped while streaming")
		}
	}
}

// TestNarrationMsg_AllowedDespiteRecentUserMsg is the regression test for the
// keep_alive bug: game narration must reach the chat even when the user
// submitted a message less than 30 seconds ago.
func TestNarrationMsg_AllowedDespiteRecentUserMsg(t *testing.T) {
	m := blankModel()
	m.lastUserMsgAt = time.Now() // simulate user just sent a command
	result, _ := m.Update(glitchNarrationMsg{text: "level up!"})
	rm := result.(Model)
	found := false
	for _, msg := range rm.glitchChat.messages {
		if msg.text == "level up!" {
			found = true
			break
		}
	}
	if !found {
		t.Error("game narration should appear even when user sent a message recently")
	}
}
