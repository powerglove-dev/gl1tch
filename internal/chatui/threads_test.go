package chatui

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestThreadStore_AppendAssignsIDAndOrder(t *testing.T) {
	s := NewThreadStore()
	a, err := s.Append(ChatMessage{Role: RoleUser, Type: MessageTypeText, Payload: TextPayload{Body: "first"}})
	if err != nil {
		t.Fatalf("Append a: %v", err)
	}
	b, err := s.Append(ChatMessage{Role: RoleAssistant, Type: MessageTypeText, Payload: TextPayload{Body: "second"}})
	if err != nil {
		t.Fatalf("Append b: %v", err)
	}
	if a.ID == "" || b.ID == "" || a.ID == b.ID {
		t.Errorf("IDs should be assigned and unique, got %q, %q", a.ID, b.ID)
	}
	if a.CreatedAt.IsZero() || b.CreatedAt.IsZero() {
		t.Errorf("CreatedAt should be stamped")
	}
	main := s.MainScrollback()
	if len(main) != 2 || main[0].ID != a.ID || main[1].ID != b.ID {
		t.Errorf("MainScrollback order wrong: %+v", main)
	}
}

func TestThreadStore_SpawnRejectsNesting(t *testing.T) {
	s := NewThreadStore()
	root, _ := s.Append(ChatMessage{Role: RoleUser, Type: MessageTypeText, Payload: TextPayload{Body: "root"}})
	thread, err := s.Spawn(root.ID, ExpandInline)
	if err != nil {
		t.Fatalf("Spawn root: %v", err)
	}

	// Add a child message inside the thread.
	child, err := s.Append(ChatMessage{
		ThreadID: thread.ID,
		Role:     RoleAssistant,
		Type:     MessageTypeText,
		Payload:  TextPayload{Body: "in-thread"},
	})
	if err != nil {
		t.Fatalf("Append child: %v", err)
	}

	// Spawning a thread under a message that's already in a thread must fail.
	if _, err := s.Spawn(child.ID, ExpandInline); !errors.Is(err, ErrNestingForbidden) {
		t.Errorf("nested spawn: got %v, want ErrNestingForbidden", err)
	}
}

func TestThreadStore_SpawnIsIdempotent(t *testing.T) {
	s := NewThreadStore()
	root, _ := s.Append(ChatMessage{Role: RoleUser, Type: MessageTypeText, Payload: TextPayload{Body: "root"}})
	t1, err := s.Spawn(root.ID, ExpandInline)
	if err != nil {
		t.Fatalf("first Spawn: %v", err)
	}
	t2, err := s.Spawn(root.ID, ExpandInline)
	if err != nil {
		t.Fatalf("second Spawn: %v", err)
	}
	if t1.ID != t2.ID {
		t.Errorf("Spawn should be idempotent, got %q vs %q", t1.ID, t2.ID)
	}
}

func TestThreadStore_LookupByParentAndByID(t *testing.T) {
	s := NewThreadStore()
	root, _ := s.Append(ChatMessage{Role: RoleUser, Type: MessageTypeText, Payload: TextPayload{Body: "root"}})
	thread, _ := s.Spawn(root.ID, ExpandSidePane)

	got, ok := s.LookupByParent(root.ID)
	if !ok || got.ID != thread.ID || got.ExpandPref != ExpandSidePane {
		t.Errorf("LookupByParent = %+v, ok=%v", got, ok)
	}
	got2, ok := s.LookupByID(thread.ID)
	if !ok || got2.ID != thread.ID {
		t.Errorf("LookupByID = %+v, ok=%v", got2, ok)
	}
	if _, ok := s.LookupByParent("nope"); ok {
		t.Errorf("LookupByParent of unknown parent should return ok=false")
	}
}

func TestThreadStore_CloseFreezesAndReopen(t *testing.T) {
	s := NewThreadStore()
	root, _ := s.Append(ChatMessage{Role: RoleUser, Type: MessageTypeText, Payload: TextPayload{Body: "root"}})
	thread, _ := s.Spawn(root.ID, ExpandInline)
	if _, err := s.Append(ChatMessage{ThreadID: thread.ID, Role: RoleAssistant, Type: MessageTypeText, Payload: TextPayload{Body: "x"}}); err != nil {
		t.Fatalf("Append in thread: %v", err)
	}

	if err := s.Close(thread.ID, "wrapped up"); err != nil {
		t.Fatalf("Close: %v", err)
	}
	// Append after Close must fail.
	_, err := s.Append(ChatMessage{ThreadID: thread.ID, Role: RoleUser, Type: MessageTypeText, Payload: TextPayload{Body: "after-close"}})
	if !errors.Is(err, ErrThreadClosed) {
		t.Errorf("Append after Close: got %v, want ErrThreadClosed", err)
	}
	// State + summary must be visible via lookup.
	got, _ := s.LookupByID(thread.ID)
	if got.State != ThreadClosed || got.Summary != "wrapped up" {
		t.Errorf("after Close: got state=%s summary=%q", got.State, got.Summary)
	}

	// Reopen restores Append capability.
	if err := s.Reopen(thread.ID); err != nil {
		t.Fatalf("Reopen: %v", err)
	}
	if _, err := s.Append(ChatMessage{ThreadID: thread.ID, Role: RoleUser, Type: MessageTypeText, Payload: TextPayload{Body: "after-reopen"}}); err != nil {
		t.Errorf("Append after Reopen: %v", err)
	}
}

func TestThreadStore_ThreadMessagesIsolated(t *testing.T) {
	s := NewThreadStore()
	root, _ := s.Append(ChatMessage{Role: RoleUser, Type: MessageTypeText, Payload: TextPayload{Body: "root"}})
	thread, _ := s.Spawn(root.ID, ExpandInline)
	for _, body := range []string{"a", "b", "c"} {
		_, _ = s.Append(ChatMessage{ThreadID: thread.ID, Role: RoleAssistant, Type: MessageTypeText, Payload: TextPayload{Body: body}})
	}
	// Add another main-chat message; it must NOT show up in the thread.
	_, _ = s.Append(ChatMessage{Role: RoleUser, Type: MessageTypeText, Payload: TextPayload{Body: "main2"}})

	in := s.ThreadMessages(thread.ID)
	if len(in) != 3 {
		t.Errorf("ThreadMessages = %d, want 3", len(in))
	}
	main := s.MainScrollback()
	if len(main) != 2 {
		t.Errorf("MainScrollback = %d, want 2 (root + main2)", len(main))
	}
}

// ─── slash dispatcher tests ──────────────────────────────────────────────────

func TestSlashRegistry_HelpListsCommands(t *testing.T) {
	reg := NewSlashRegistry()
	if err := reg.Register(HelpHandler(reg)); err != nil {
		t.Fatalf("Register help: %v", err)
	}
	if err := reg.Register(SlashHandlerFunc{
		NameField:     "status",
		DescribeField: "Show workspace status",
		Fn: func(_ context.Context, _ SlashInvocation) ([]ChatMessage, error) {
			return []ChatMessage{{Role: RoleAssistant, Type: MessageTypeText, Payload: TextPayload{Body: "ok"}}}, nil
		},
	}); err != nil {
		t.Fatalf("Register status: %v", err)
	}

	msgs, err := reg.Dispatch(context.Background(), "/help", SlashScopeMain)
	if err != nil {
		t.Fatalf("Dispatch /help: %v", err)
	}
	if len(msgs) != 1 || msgs[0].Type != MessageTypeWidgetCard {
		t.Fatalf("expected one widget_card, got %+v", msgs)
	}
	card, ok := msgs[0].Payload.(WidgetCardPayload)
	if !ok {
		t.Fatalf("expected WidgetCardPayload, got %T", msgs[0].Payload)
	}
	// Help must list every registered command, including itself.
	var keys []string
	for _, row := range card.Rows {
		keys = append(keys, row.Key)
	}
	if len(keys) != 2 || !contains(keys, "/help") || !contains(keys, "/status") {
		t.Errorf("help rows = %v, want both /help and /status", keys)
	}
}

func TestSlashRegistry_DispatchUnknownCommand(t *testing.T) {
	reg := NewSlashRegistry()
	_, err := reg.Dispatch(context.Background(), "/nope", SlashScopeMain)
	if !errors.Is(err, ErrUnknownCommand) {
		t.Errorf("got %v, want ErrUnknownCommand", err)
	}
}

func TestSlashRegistry_DispatchEmptyOrNonSlashIsEmptyInput(t *testing.T) {
	reg := NewSlashRegistry()
	for _, line := range []string{"", "  ", "hello world", "/   "} {
		_, err := reg.Dispatch(context.Background(), line, SlashScopeMain)
		if !errors.Is(err, ErrEmptyInput) {
			t.Errorf("Dispatch %q: got %v, want ErrEmptyInput", line, err)
		}
	}
}

func TestSlashRegistry_ScopeGating(t *testing.T) {
	reg := NewSlashRegistry()
	// /save is a thread-only command.
	_ = reg.Register(SlashHandlerFunc{
		NameField:     "save",
		DescribeField: "Close the current thread",
		Allowed:       []SlashScope{"thread:*"},
		Fn: func(_ context.Context, _ SlashInvocation) ([]ChatMessage, error) {
			return nil, nil
		},
	})

	// In main: rejected.
	_, err := reg.Dispatch(context.Background(), "/save", SlashScopeMain)
	if !errors.Is(err, ErrScopeNotAllowed) {
		t.Errorf("main /save: got %v, want ErrScopeNotAllowed", err)
	}
	// In a thread scope: accepted.
	if _, err := reg.Dispatch(context.Background(), "/save", ThreadScope("t-123")); err != nil {
		t.Errorf("thread /save: %v", err)
	}
}

func TestSlashRegistry_FlagAndArgParsing(t *testing.T) {
	reg := NewSlashRegistry()
	var captured SlashInvocation
	_ = reg.Register(SlashHandlerFunc{
		NameField:     "ask",
		DescribeField: "Ask the assistant",
		Fn: func(_ context.Context, in SlashInvocation) ([]ChatMessage, error) {
			captured = in
			return nil, nil
		},
	})
	if _, err := reg.Dispatch(context.Background(), "/ask why is sky blue model=qwen2.5:7b temperature=0.4", SlashScopeMain); err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	if captured.Name != "ask" {
		t.Errorf("Name = %q, want ask", captured.Name)
	}
	if captured.Flags["model"] != "qwen2.5:7b" || captured.Flags["temperature"] != "0.4" {
		t.Errorf("Flags = %v", captured.Flags)
	}
	wantArgs := []string{"why", "is", "sky", "blue"}
	if strings.Join(captured.Args, " ") != strings.Join(wantArgs, " ") {
		t.Errorf("Args = %v, want %v", captured.Args, wantArgs)
	}
	if captured.Raw != "why is sky blue model=qwen2.5:7b temperature=0.4" {
		t.Errorf("Raw = %q", captured.Raw)
	}
}

func TestSlashScope_HelpersRoundtrip(t *testing.T) {
	s := ThreadScope("t-abc")
	if !s.IsThreadScope() {
		t.Errorf("IsThreadScope should be true for %s", s)
	}
	if s.ThreadID() != "t-abc" {
		t.Errorf("ThreadID = %q, want t-abc", s.ThreadID())
	}
	if SlashScopeMain.IsThreadScope() {
		t.Errorf("main scope should not be a thread scope")
	}
}

func contains(ss []string, want string) bool {
	for _, s := range ss {
		if s == want {
			return true
		}
	}
	return false
}

// touch a time symbol so the import does not become unused if a test is removed
var _ = time.Now
