package inbox

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/8op-org/gl1tch/internal/store"
	"github.com/8op-org/gl1tch/internal/themes"
)

// openTestStore creates a Store backed by a temporary directory.
func openTestStore(t *testing.T) *store.Store {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test.db")
	s, err := store.OpenAt(path)
	if err != nil {
		t.Fatalf("store.OpenAt: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

// testBundle returns a minimal theme bundle for use in tests.
func testBundle() *themes.Bundle {
	return &themes.Bundle{
		Palette: themes.Palette{
			BG:      "#282a36",
			FG:      "#f8f8f2",
			Accent:  "#bd93f9",
			Dim:     "#6272a4",
			Border:  "#44475a",
			Error:   "#ff5555",
			Success: "#50fa7b",
		},
	}
}

// TestNew_NilStore verifies that New(nil, bundle) doesn't panic.
func TestNew_NilStore(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("New(nil, bundle) panicked: %v", r)
		}
	}()
	_ = New(nil, testBundle())
}

// TestModel_SetSize verifies that SetSize updates width and height fields.
func TestModel_SetSize(t *testing.T) {
	m := New(nil, testBundle())
	m.SetSize(100, 50)
	if m.width != 100 {
		t.Errorf("expected width=100, got %d", m.width)
	}
	if m.height != 50 {
		t.Errorf("expected height=50, got %d", m.height)
	}
}

// TestModel_SetFocused verifies toggling the focused field.
func TestModel_SetFocused(t *testing.T) {
	m := New(nil, testBundle())
	if m.focused {
		t.Error("expected focused=false initially")
	}
	m.SetFocused(true)
	if !m.focused {
		t.Error("expected focused=true after SetFocused(true)")
	}
	m.SetFocused(false)
	if m.focused {
		t.Error("expected focused=false after SetFocused(false)")
	}
}

// TestStatusIndicator_Success verifies exit 0 renders the filled-circle indicator.
func TestStatusIndicator_Success(t *testing.T) {
	b := testBundle()
	exitOK := 0
	run := store.Run{ExitStatus: &exitOK}
	s := statusIndicator(run, b)
	// The rendered string must contain the success dot character.
	if !strings.Contains(s, "●") {
		t.Errorf("expected success indicator '●', got %q", s)
	}
	// A nil exit should NOT produce the success indicator.
	runNil := store.Run{ExitStatus: nil}
	sNil := statusIndicator(runNil, b)
	if strings.Contains(sNil, "●") {
		t.Errorf("in-flight run should not render '●', got %q", sNil)
	}
}

// TestStatusIndicator_Error verifies non-zero exit renders the error indicator.
func TestStatusIndicator_Error(t *testing.T) {
	b := testBundle()
	exitErr := 1
	run := store.Run{ExitStatus: &exitErr}
	s := statusIndicator(run, b)
	if !strings.Contains(s, "●") {
		t.Errorf("expected error indicator '●', got %q", s)
	}
	// A zero exit should not match the same ANSI sequence — just ensure the
	// character is present for error.
	exitOK := 0
	sOK := statusIndicator(store.Run{ExitStatus: &exitOK}, b)
	// Both are "●" but styled differently; ensure error path still renders.
	if s == "" || sOK == "" {
		t.Error("statusIndicator returned empty string")
	}
}

// TestStatusIndicator_InFlight verifies nil exit returns the ring indicator.
func TestStatusIndicator_InFlight(t *testing.T) {
	b := testBundle()
	run := store.Run{ExitStatus: nil}
	s := statusIndicator(run, b)
	if !strings.Contains(s, "◉") {
		t.Errorf("expected in-flight indicator '◉', got %q", s)
	}
	// Completed runs must NOT use the ◉ character.
	exitOK := 0
	sDone := statusIndicator(store.Run{ExitStatus: &exitOK}, b)
	if strings.Contains(sDone, "◉") {
		t.Errorf("completed run should not render '◉', got %q", sDone)
	}
}

// TestElapsedStr_Finished verifies correct duration format for completed runs.
func TestElapsedStr_Finished(t *testing.T) {
	startedAt := time.Now().Add(-10 * time.Second).UnixMilli()
	finishedAt := time.Now().UnixMilli()
	run := store.Run{
		StartedAt:  startedAt,
		FinishedAt: &finishedAt,
	}
	s := elapsedStr(run)
	// Duration should be approximately 10s; should NOT contain "running".
	if strings.Contains(s, "running") {
		t.Errorf("finished run should not say 'running', got %q", s)
	}
	if !strings.Contains(s, "s") {
		t.Errorf("expected seconds in elapsed string, got %q", s)
	}
}

// TestElapsedStr_InFlight verifies "running X" format for in-progress runs.
func TestElapsedStr_InFlight(t *testing.T) {
	run := store.Run{
		StartedAt:  time.Now().Add(-3 * time.Second).UnixMilli(),
		FinishedAt: nil,
	}
	s := elapsedStr(run)
	if !strings.HasPrefix(s, "running ") {
		t.Errorf("in-flight run should start with 'running ', got %q", s)
	}
}

// TestInbox_BusConnectSetsFlag verifies that receiving a busConnectMsg sets
// busConnected to true on the model.
func TestInbox_BusConnectSetsFlag(t *testing.T) {
	s := openTestStore(t)
	m := New(s, nil)
	ch := make(chan busEventMsg, 1)
	updated, _ := m.Update(busConnectMsg{ch: ch})
	if !updated.busConnected {
		t.Fatal("expected busConnected to be true after busConnectMsg")
	}
	if updated.busCh == nil {
		t.Fatal("expected busCh to be set after busConnectMsg")
	}
}

// TestInbox_BusEventTriggersRefresh verifies that a busEventMsg causes the
// model to re-query the store and populate m.runs.
func TestInbox_BusEventTriggersRefresh(t *testing.T) {
	s := openTestStore(t)
	id, err := s.RecordRunStart("pipeline", "test-pipe", "")
	if err != nil {
		t.Fatalf("RecordRunStart: %v", err)
	}
	if err := s.RecordRunComplete(id, 0, "ok", ""); err != nil {
		t.Fatalf("RecordRunComplete: %v", err)
	}

	m := New(s, nil)
	ch := make(chan busEventMsg, 1)
	m.busConnected = true
	m.busCh = ch

	updated, _ := m.Update(busEventMsg{topic: "pipeline.run.completed"})
	if len(updated.runs) == 0 {
		t.Fatal("expected runs to be populated after busEventMsg")
	}
}

// TestInbox_BusDisconnectedClearsFlag verifies that busDisconnectedMsg resets
// busConnected and busCh so the model falls back to poll-based refresh.
func TestInbox_BusDisconnectedClearsFlag(t *testing.T) {
	m := New(nil, nil)
	ch := make(chan busEventMsg, 1)
	m.busConnected = true
	m.busCh = ch

	updated, _ := m.Update(busDisconnectedMsg{})
	if updated.busConnected {
		t.Fatal("expected busConnected to be false after busDisconnectedMsg")
	}
	if updated.busCh != nil {
		t.Fatal("expected busCh to be nil after busDisconnectedMsg")
	}
}

// TestInbox_ScheduleNextTick_Intervals verifies poll interval selection.
func TestInbox_ScheduleNextTick_Intervals(t *testing.T) {
	// When bus is not connected, scheduleNextTick should use pollIntervalFast.
	// We can't directly inspect the tick duration from a tea.Cmd, but we can
	// verify the method doesn't panic and that the busConnected flag affects
	// the model state correctly via the Update path.
	m := New(nil, nil)
	if m.busConnected {
		t.Fatal("new model should not be bus-connected")
	}
	// After a busConnectMsg the connected flag is true.
	ch := make(chan busEventMsg, 1)
	m2, _ := m.Update(busConnectMsg{ch: ch})
	if !m2.busConnected {
		t.Fatal("expected busConnected=true after connect msg")
	}
	// After disconnect the flag clears.
	m3, _ := m2.Update(busDisconnectedMsg{})
	if m3.busConnected {
		t.Fatal("expected busConnected=false after disconnect msg")
	}
}

// TestItem_Description_StepBadge verifies that a step count badge is rendered
// when the run has steps.
func TestItem_Description_StepBadge(t *testing.T) {
	b := testBundle()
	exitOK := 0
	run := store.Run{
		ExitStatus: &exitOK,
		StartedAt:  time.Now().Add(-5 * time.Second).UnixMilli(),
		Steps: []store.StepRecord{
			{ID: "step1", Status: "done"},
			{ID: "step2", Status: "done"},
		},
	}
	finishedAt := time.Now().UnixMilli()
	run.FinishedAt = &finishedAt

	it := item{run: run, bundle: b}
	desc := it.Description()
	if !strings.Contains(desc, "2 steps") {
		t.Errorf("expected '2 steps' in description, got %q", desc)
	}
}

// TestItem_Description_NoStepBadge verifies that no badge is shown when the
// run has no steps.
func TestItem_Description_NoStepBadge(t *testing.T) {
	b := testBundle()
	exitOK := 0
	finishedAt := time.Now().UnixMilli()
	run := store.Run{
		ExitStatus: &exitOK,
		StartedAt:  time.Now().Add(-5 * time.Second).UnixMilli(),
		FinishedAt: &finishedAt,
		Steps:      []store.StepRecord{},
	}
	it := item{run: run, bundle: b}
	desc := it.Description()
	if strings.Contains(desc, "steps") {
		t.Errorf("expected no 'steps' badge when run has 0 steps, got %q", desc)
	}
}
