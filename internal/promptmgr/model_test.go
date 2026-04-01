package promptmgr

import (
	"context"
	"io"
	"path/filepath"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/powerglove-dev/gl1tch/internal/executor"
	"github.com/powerglove-dev/gl1tch/internal/store"
)

// openTestStore creates a Store backed by a temporary directory.
func openTestStore(t *testing.T) *store.Store {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test.db")
	s, err := store.OpenAt(path)
	if err != nil {
		t.Fatalf("OpenAt: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

// insertPrompt is a helper that inserts a prompt and fatals on error.
func insertPrompt(t *testing.T, st *store.Store, title, body string) store.Prompt {
	t.Helper()
	id, err := st.InsertPrompt(context.Background(), store.Prompt{Title: title, Body: body})
	if err != nil {
		t.Fatalf("InsertPrompt(%q): %v", title, err)
	}
	return store.Prompt{ID: id, Title: title, Body: body}
}

// sendKey sends a rune key message through Update and returns the updated model.
func sendKey(t *testing.T, m *Model, key string) (*Model, tea.Cmd) {
	t.Helper()
	var msg tea.KeyMsg
	switch key {
	case "ctrl+r":
		msg = tea.KeyMsg{Type: tea.KeyCtrlR}
	case "ctrl+c":
		msg = tea.KeyMsg{Type: tea.KeyCtrlC}
	case "ctrl+s":
		msg = tea.KeyMsg{Type: tea.KeyCtrlS}
	case "enter":
		msg = tea.KeyMsg{Type: tea.KeyEnter}
	case "esc":
		msg = tea.KeyMsg{Type: tea.KeyEsc}
	case "tab":
		msg = tea.KeyMsg{Type: tea.KeyTab}
	default:
		msg = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)}
	}
	model, cmd := m.Update(msg)
	return model.(*Model), cmd
}

// loadPrompts delivers a promptsLoadedMsg to the model, simulating what Init would do.
func loadPrompts(t *testing.T, m *Model) *Model {
	t.Helper()
	prompts, err := m.store.ListPrompts(context.Background())
	if err != nil {
		t.Fatalf("ListPrompts: %v", err)
	}
	model, _ := m.Update(promptsLoadedMsg{prompts: prompts})
	return model.(*Model)
}

// drainCmd executes pending tea.Cmds until exhausted, returning the final model.
func drainCmd(m *Model, cmd tea.Cmd) *Model {
	if cmd == nil {
		return m
	}
	msg := cmd()
	if msg == nil {
		return m
	}
	// Unwrap batch messages by draining each sub-command independently.
	if batch, ok := msg.(tea.BatchMsg); ok {
		for _, c := range batch {
			m = drainCmd(m, c)
		}
		return m
	}
	// Skip spinner ticks — they loop indefinitely; not useful in tests.
	if _, ok := msg.(spinnerTickMsg); ok {
		return m
	}
	model, nextCmd := m.Update(msg)
	m = model.(*Model)
	return drainCmd(m, nextCmd)
}

// ---------------------------------------------------------------------------
// Task 9.1 — BubbleTea model tests
// ---------------------------------------------------------------------------

func TestModel_ListNavigation(t *testing.T) {
	st := openTestStore(t)
	insertPrompt(t, st, "Alpha prompt", "body 1")
	insertPrompt(t, st, "Beta prompt", "body 2")
	insertPrompt(t, st, "Gamma prompt", "body 3")

	m := New(st, nil, nil)
	m = loadPrompts(t, m)

	if len(m.filtered) != 3 {
		t.Fatalf("want 3 prompts loaded, got %d", len(m.filtered))
	}

	// Start at 0, press "j" → should go to 1.
	m.selectedIdx = 0
	var cmd tea.Cmd
	m, cmd = sendKey(t, m, "j")
	_ = cmd
	if m.selectedIdx != 1 {
		t.Errorf("after j: want selectedIdx=1, got %d", m.selectedIdx)
	}

	// Press "j" again → should go to 2.
	m, _ = sendKey(t, m, "j")
	if m.selectedIdx != 2 {
		t.Errorf("after j again: want selectedIdx=2, got %d", m.selectedIdx)
	}

	// Press "k" → should go back to 1.
	m, _ = sendKey(t, m, "k")
	if m.selectedIdx != 1 {
		t.Errorf("after k: want selectedIdx=1, got %d", m.selectedIdx)
	}

	// Clamp: at last index (2), press "j" → should stay at 2.
	m.selectedIdx = 2
	m, _ = sendKey(t, m, "j")
	if m.selectedIdx != 2 {
		t.Errorf("clamp at last index: want selectedIdx=2, got %d", m.selectedIdx)
	}

	// Clamp: at index 0, press "k" → should stay at 0.
	m.selectedIdx = 0
	m, _ = sendKey(t, m, "k")
	if m.selectedIdx != 0 {
		t.Errorf("clamp at 0: want selectedIdx=0, got %d", m.selectedIdx)
	}
}

func TestModel_SearchFilter(t *testing.T) {
	st := openTestStore(t)
	insertPrompt(t, st, "Alpha prompt", "first body")
	insertPrompt(t, st, "Beta prompt", "second body")
	insertPrompt(t, st, "Gamma stuff", "third body")

	m := New(st, nil, nil)
	m = loadPrompts(t, m)

	if len(m.filtered) != 3 {
		t.Fatalf("want 3 prompts initially, got %d", len(m.filtered))
	}

	// Filter for "Alpha" — should narrow to 1 result.
	m.filterInput.SetValue("Alpha")
	m.applyFilter()
	if len(m.filtered) != 1 {
		t.Errorf("after filter 'Alpha': want 1, got %d", len(m.filtered))
	}
	if m.filtered[0].Title != "Alpha prompt" {
		t.Errorf("want 'Alpha prompt', got %q", m.filtered[0].Title)
	}

	// Clear filter → should restore all 3.
	m.filterInput.SetValue("")
	m.applyFilter()
	if len(m.filtered) != 3 {
		t.Errorf("after clearing filter: want 3, got %d", len(m.filtered))
	}
}

func TestModel_DeleteConfirmation(t *testing.T) {
	t.Run("d then n cancels", func(t *testing.T) {
		st := openTestStore(t)
		p := insertPrompt(t, st, "To delete", "body")
		_ = p

		m := New(st, nil, nil)
		m = loadPrompts(t, m)

		if len(m.filtered) != 1 {
			t.Fatalf("want 1 prompt, got %d", len(m.filtered))
		}

		// Press "d" → confirmDelete should be true.
		m, _ = sendKey(t, m, "d")
		if !m.confirmDelete {
			t.Error("after d: want confirmDelete=true")
		}

		// Press "n" → confirmDelete should be false.
		m, _ = sendKey(t, m, "n")
		if m.confirmDelete {
			t.Error("after n: want confirmDelete=false")
		}

		// Prompt should still be in list.
		if len(m.filtered) != 1 {
			t.Errorf("after cancel delete: want 1 prompt, got %d", len(m.filtered))
		}
	})

	t.Run("d then y deletes prompt", func(t *testing.T) {
		st := openTestStore(t)
		insertPrompt(t, st, "Deletable", "body")

		m := New(st, nil, nil)
		m = loadPrompts(t, m)

		if len(m.filtered) != 1 {
			t.Fatalf("want 1 prompt, got %d", len(m.filtered))
		}

		// Press "d" → confirmDelete.
		m, _ = sendKey(t, m, "d")
		if !m.confirmDelete {
			t.Fatal("after d: want confirmDelete=true")
		}

		// Press "y" → triggers deletePromptCmd.
		m, cmd := sendKey(t, m, "y")
		if m.confirmDelete {
			t.Error("after y: want confirmDelete=false")
		}

		// Drain all cmds (delete → reload → prompts loaded).
		m = drainCmd(m, cmd)

		// After drain, filtered list should be empty.
		if len(m.filtered) != 0 {
			t.Errorf("after delete drain: want 0 prompts, got %d", len(m.filtered))
		}
	})
}

func TestModel_NewPrompt(t *testing.T) {
	st := openTestStore(t)
	m := New(st, nil, nil)

	// Start on list panel (0), press "n".
	m, _ = sendKey(t, m, "n")

	if m.focusPanel != 1 {
		t.Errorf("after n: want focusPanel=1, got %d", m.focusPanel)
	}
	if m.editingPrompt.ID != 0 {
		t.Errorf("after n: want editingPrompt.ID=0 (new), got %d", m.editingPrompt.ID)
	}
}

// ---------------------------------------------------------------------------
// Task 9.2 — Runner with mock executor
// ---------------------------------------------------------------------------

func TestModel_RunnerWithMockExecutor(t *testing.T) {
	st := openTestStore(t)

	stub := &executor.StubExecutor{
		ExecutorName: "test-model",
		ExecuteFn: func(_ context.Context, input string, _ map[string]string, w io.Writer) error {
			_, err := w.Write([]byte("hello world"))
			return err
		},
	}
	mgr := executor.NewManager()
	if err := mgr.Register(stub); err != nil {
		t.Fatalf("Register: %v", err)
	}

	m := New(st, mgr, nil)

	// Switch to editor panel, set body text, and set model slug directly.
	m.focusPanel = 1
	m.bodyInput.SetValue("my prompt body")
	m.editingPrompt.ModelSlug = "test-model"

	// Send ctrl+r to start a run.
	m, cmd := sendKey(t, m, "ctrl+r")
	if !m.runnerStreaming {
		t.Error("after ctrl+r: want runnerStreaming=true")
	}
	if m.runCancel == nil {
		t.Error("after ctrl+r: want runCancel != nil")
	}

	// Drain the run command — runExecutorCmd blocks until executor exits.
	m = drainCmd(m, cmd)

	// After run completes:
	if m.runnerStreaming {
		t.Error("after run: want runnerStreaming=false")
	}
	if m.runnerOutput != "hello world" {
		t.Errorf("after run: want runnerOutput=%q, got %q", "hello world", m.runnerOutput)
	}
	if m.runCancel != nil {
		t.Error("after run: want runCancel=nil")
	}
}

func TestModel_RunnerCancellation(t *testing.T) {
	st := openTestStore(t)

	stub := &executor.StubExecutor{
		ExecutorName: "test-model",
		ExecuteFn: func(ctx context.Context, _ string, _ map[string]string, _ io.Writer) error {
			// Block until context cancelled.
			<-ctx.Done()
			return ctx.Err()
		},
	}
	mgr := executor.NewManager()
	if err := mgr.Register(stub); err != nil {
		t.Fatalf("Register: %v", err)
	}

	m := New(st, mgr, nil)

	m.focusPanel = 1
	m.bodyInput.SetValue("some prompt")
	m.editingPrompt.ModelSlug = "test-model"

	// Start run.
	m, _ = sendKey(t, m, "ctrl+r")
	if !m.runnerStreaming {
		t.Error("after ctrl+r: want runnerStreaming=true")
	}
	if m.runCancel == nil {
		t.Fatal("after ctrl+r: want runCancel != nil")
	}

	// Cancel the run by calling runCancel directly.
	cancel := m.runCancel
	cancel()

	// Simulate the error message that the run goroutine would produce.
	model2, _ := m.Update(runErrMsg{err: context.Canceled})
	m = model2.(*Model)

	if m.runnerStreaming {
		t.Error("after cancel: want runnerStreaming=false")
	}
	if m.runCancel != nil {
		t.Error("after cancel: want runCancel=nil")
	}
}

// ---------------------------------------------------------------------------
// Task 9.3 — Theme inheritance
// ---------------------------------------------------------------------------

func TestModel_ThemeInheritance(t *testing.T) {
	st := openTestStore(t)

	// Construct model with nil bundle (defaults).
	m := New(st, nil, nil)

	// ThemeState should be initialized (not zero value struct).
	// NewThemeState always returns a valid ThemeState, so just confirm it
	// doesn't panic and View() returns something.
	view := m.View()
	if view == "" {
		t.Error("View() returned empty string, want non-empty")
	}
}
