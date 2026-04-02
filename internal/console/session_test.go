package console

import "testing"

// ── SessionRegistry ───────────────────────────────────────────────────────────

func TestNewSessionRegistry_DefaultsToMain(t *testing.T) {
	r := newSessionRegistry()
	if r.active != "main" {
		t.Fatalf("active = %q, want %q", r.active, "main")
	}
	if len(r.sessions) != 1 {
		t.Fatalf("len(sessions) = %d, want 1", len(r.sessions))
	}
	if r.sessions[0].name != "main" {
		t.Fatalf("sessions[0].name = %q, want %q", r.sessions[0].name, "main")
	}
	if r.sessions[0].status != SessionActive {
		t.Fatalf("sessions[0].status = %v, want SessionActive", r.sessions[0].status)
	}
}

func TestNeedsFooter(t *testing.T) {
	r := newSessionRegistry()
	if r.NeedsFooter() {
		t.Fatal("NeedsFooter() = true with 1 session, want false")
	}
	r.create("work")
	if !r.NeedsFooter() {
		t.Fatal("NeedsFooter() = false with 2 sessions, want true")
	}
}

func TestActive_ReturnsCurrentSession(t *testing.T) {
	r := newSessionRegistry()
	a := r.Active()
	if a == nil {
		t.Fatal("Active() = nil")
	}
	if a.name != "main" {
		t.Fatalf("Active().name = %q, want %q", a.name, "main")
	}
}

func TestCreate_AddsIdleSession(t *testing.T) {
	r := newSessionRegistry()
	s := r.create("work")
	if s.name != "work" {
		t.Fatalf("created name = %q, want %q", s.name, "work")
	}
	if s.status != SessionIdle {
		t.Fatalf("created status = %v, want SessionIdle", s.status)
	}
	if len(r.sessions) != 2 {
		t.Fatalf("len(sessions) = %d, want 2", len(r.sessions))
	}
}

func TestSwitchTo_ActivatesSession(t *testing.T) {
	r := newSessionRegistry()
	r.create("work")

	ok := r.switchTo("work")
	if !ok {
		t.Fatal("switchTo returned false for existing session")
	}
	if r.active != "work" {
		t.Fatalf("active = %q, want %q", r.active, "work")
	}
	if r.Active().status != SessionActive {
		t.Fatalf("new active status = %v, want SessionActive", r.Active().status)
	}
	// Previous session should be idle.
	prev := r.get("main")
	if prev.status != SessionIdle {
		t.Fatalf("previous session status = %v, want SessionIdle", prev.status)
	}
}

func TestSwitchTo_ReturnsFalseForMissing(t *testing.T) {
	r := newSessionRegistry()
	if r.switchTo("ghost") {
		t.Fatal("switchTo returned true for nonexistent session")
	}
}

func TestMarkUnread_BumpsBackgroundSession(t *testing.T) {
	r := newSessionRegistry()
	r.create("work")

	r.markUnread("work")
	s := r.get("work")
	if s.status != SessionUnread {
		t.Fatalf("status = %v, want SessionUnread", s.status)
	}
}

func TestMarkUnread_NoOpOnActiveSession(t *testing.T) {
	r := newSessionRegistry()
	r.markUnread("main") // active session — should be ignored
	if r.Active().status != SessionActive {
		t.Fatalf("active session status changed to %v", r.Active().status)
	}
}

func TestMarkAttention_BumpsToAttention(t *testing.T) {
	r := newSessionRegistry()
	r.create("work")
	r.markAttention("work")
	if r.get("work").status != SessionAttention {
		t.Fatalf("status = %v, want SessionAttention", r.get("work").status)
	}
}

func TestMarkUnread_DoesNotDowngradeAttention(t *testing.T) {
	r := newSessionRegistry()
	r.create("work")
	r.markAttention("work")
	r.markUnread("work") // lower priority — status must not regress
	if r.get("work").status != SessionAttention {
		t.Fatalf("status = %v, want SessionAttention after markUnread on Attention session", r.get("work").status)
	}
}

func TestAttentionSession_ReturnsFirstNeedingAction(t *testing.T) {
	r := newSessionRegistry()
	r.create("a")
	r.create("b")
	r.markUnread("b")

	got := r.AttentionSession()
	if got != "b" {
		t.Fatalf("AttentionSession() = %q, want %q", got, "b")
	}
}

func TestAttentionSession_EmptyWhenNone(t *testing.T) {
	r := newSessionRegistry()
	r.create("a") // idle
	if attn := r.AttentionSession(); attn != "" {
		t.Fatalf("AttentionSession() = %q, want empty", attn)
	}
}

func TestAttentionSession_SkipsActiveSession(t *testing.T) {
	r := newSessionRegistry()
	// Only active session exists — should always return "".
	if attn := r.AttentionSession(); attn != "" {
		t.Fatalf("AttentionSession() = %q for single active session", attn)
	}
}
