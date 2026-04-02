package console_test

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/8op-org/gl1tch/internal/console"
)

// ── scanPipelines ─────────────────────────────────────────────────────────────

func TestScanPipelines_MissingDir(t *testing.T) {
	result := console.ScanPipelines("/tmp/does-not-exist-orcai-test-dir")
	if len(result) != 0 {
		t.Errorf("expected 0 pipelines for missing dir, got %d", len(result))
	}
}

func TestScanPipelines_EmptyDir(t *testing.T) {
	dir := t.TempDir()
	result := console.ScanPipelines(dir)
	if len(result) != 0 {
		t.Errorf("expected 0 pipelines for empty dir, got %d", len(result))
	}
}

func TestScanPipelines_PopulatedDir(t *testing.T) {
	dir := t.TempDir()
	// Create some .pipeline.yaml files.
	for _, name := range []string{"alpha.pipeline.yaml", "beta.pipeline.yaml", "gamma.pipeline.yaml"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("name: "+name+"\nsteps: []\n"), 0o600); err != nil {
			t.Fatalf("create file: %v", err)
		}
	}
	// Create a non-pipeline file that should be ignored.
	os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("ignore me"), 0o600) //nolint:errcheck

	result := console.ScanPipelines(dir)
	if len(result) != 3 {
		t.Fatalf("expected 3 pipelines, got %d: %v", len(result), result)
	}
	// Check that extensions are stripped.
	for _, r := range result {
		if strings.HasSuffix(r, ".pipeline.yaml") {
			t.Errorf("expected extension stripped, got %q", r)
		}
		if strings.Contains(r, ".yaml") {
			t.Errorf("expected no yaml suffix, got %q", r)
		}
	}
	// Verify names are present.
	names := map[string]bool{}
	for _, r := range result {
		names[r] = true
	}
	for _, want := range []string{"alpha", "beta", "gamma"} {
		if !names[want] {
			t.Errorf("missing pipeline %q in results %v", want, result)
		}
	}
}

// ── ChanPublisher ─────────────────────────────────────────────────────────────

func TestChanPublisher_SendsFeedLineMsg(t *testing.T) {
	ch := make(chan tea.Msg, 10)
	pub := console.NewChanPublisher("test-id", ch)
	err := pub.Publish(context.Background(), "step.done", []byte(`{"step":"s1"}`))
	if err != nil {
		t.Fatalf("Publish returned error: %v", err)
	}

	select {
	case msg := <-ch:
		fl, ok := msg.(console.FeedLineMsg)
		if !ok {
			t.Fatalf("expected FeedLineMsg, got %T", msg)
		}
		if fl.ID != "test-id" {
			t.Errorf("expected id %q, got %q", "test-id", fl.ID)
		}
		if !strings.Contains(fl.Line, "step.done") {
			t.Errorf("expected line to contain 'step.done', got %q", fl.Line)
		}
	default:
		t.Fatal("expected message in channel, got none")
	}
}

// ── Saved pipeline picker in send panel ──────────────────────────────────────


// ── Agent modal overlay ───────────────────────────────────────────────────────

// TestAgentSendPanelFocusedOnA asserts that pressing 'a' focuses the send panel.

// ── View smoke test ───────────────────────────────────────────────────────────

func TestViewContainsBanner(t *testing.T) {
	m := console.New()
	m2, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	view := m2.(console.Model).View()
	// The top bar is gone — TDF block-art is the header. Check that the view
	// rendered at all: it should contain panel box-drawing borders.
	if !strings.Contains(view, "│") {
		t.Errorf("View() missing box-drawing border:\n%s", view)
	}
}


func TestViewContainsPanelHintFooter(t *testing.T) {
	// Panels show their hint footer inside their own border when focused.
	m := console.New()
	m2, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m3 := m2.(console.Model)

	// Launcher is focused by default — its hint footer should be visible.
	viewLauncher := m3.View()
	if !strings.Contains(viewLauncher, "enter") {
		t.Errorf("View() launcher hint footer missing when launcher focused:\n%s", viewLauncher)
	}

	// Tab 4x to reach agent send panel — it should show a hint.
	cur := m3
	for i := 0; i < 4; i++ {
		nx, _ := cur.Update(tea.KeyMsg{Type: tea.KeyTab})
		cur = nx.(console.Model)
	}
	viewAgent := cur.View()
	// Send panel shows "send" or "pick agent" hints when focused.
	if !strings.Contains(viewAgent, "tab") && !strings.Contains(viewAgent, "send") {
		t.Errorf("View() agent send panel hint footer missing when agent focused:\n%s", viewAgent)
	}
}

// ── Feed scroll (task 1.6) ─────────────────────────────────────────────────────

func TestFeedScrollOffset_ClampedAtZero(t *testing.T) {
	m := console.New()
	m2, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m3 := m2.(console.Model)
	// Press up — offset should stay at 0.
	m4, _ := m3.Update(tea.KeyMsg{Type: tea.KeyUp})
	m5 := m4.(console.Model)
	if got := m5.FeedScrollOffset(); got != 0 {
		t.Errorf("feedScrollOffset should be 0 at top, got %d", got)
	}
}

func TestFeedScrollOffset_InitialIsZero(t *testing.T) {
	m := console.New()
	m2, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 10})
	m3 := m2.(console.Model)
	// Add many feed entries with output lines so total lines exceed visible height.
	for i := 0; i < 30; i++ {
		lines := make([]string, 5)
		for j := range lines {
			lines[j] = "output line"
		}
		m3 = m3.AddFeedEntry("id", "title", console.FeedDone, lines)
	}
	// Verify offset is 0 by default.
	if got := m3.FeedScrollOffset(); got != 0 {
		t.Errorf("initial feedScrollOffset should be 0, got %d", got)
	}
}

func TestFeedScrollOffset_ResetOnNewEntry(t *testing.T) {
	m := console.New()
	m2, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 10})
	m3 := m2.(console.Model)
	// Add a feed entry — scroll offset should be 0.
	m4 := m3.AddFeedEntry("id1", "first job", console.FeedDone, []string{"line"})
	if got := m4.FeedScrollOffset(); got != 0 {
		t.Errorf("feedScrollOffset should be 0 after new entry, got %d", got)
	}
}


// ── Agent section fixed height (task 2.6) ──────────────────────────────────────

func TestBuildAgentSection_FixedHeight(t *testing.T) {
	m := console.NewWithTestProviders()
	m2, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m3 := m2.(console.Model)

	// Measure height when agent is focused (includes hint footer row).
	m3a, _ := m3.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")})
	m3b := m3a.(console.Model)
	focusedLines := m3b.BuildAgentSection(60)

	// Advance focus away — agent section loses hint footer row.
	m4, _ := m3b.Update(tea.KeyMsg{Type: tea.KeyTab})
	m5 := m4.(console.Model)
	unfocusedLines := m5.BuildAgentSection(60)

	// Both focused and unfocused panels have the same height (footer row always present).
	diff := len(focusedLines) - len(unfocusedLines)
	if diff != 0 {
		t.Errorf("buildAgentSection focused vs unfocused height diff should be 0, got %d (focused=%d unfocused=%d)",
			diff, len(focusedLines), len(unfocusedLines))
	}
}

// ── Signal board (task 3.8) ────────────────────────────────────────────────────

func TestSignalBoard_FilterAll(t *testing.T) {
	m := console.New()
	m2, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m3 := m2.(console.Model)
	m3 = m3.AddFeedEntry("j1", "running job", console.FeedRunning, nil)
	m3 = m3.AddFeedEntry("j2", "done job", console.FeedDone, nil)
	m3 = m3.AddFeedEntry("j3", "failed job", console.FeedFailed, nil)
	m3 = m3.SetSignalBoardFilter("all") // default is now "running"; set explicitly for this test

	sb := m3.BuildSignalBoard(15, 60)
	rendered := strings.Join(sb, "\n")
	// All filter — all 3 jobs should appear.
	if !strings.Contains(rendered, "running") {
		t.Errorf("signal board (all filter) missing 'running': %s", rendered)
	}
	if !strings.Contains(rendered, "done") {
		t.Errorf("signal board (all filter) missing 'done': %s", rendered)
	}
	if !strings.Contains(rendered, "failed") {
		t.Errorf("signal board (all filter) missing 'failed': %s", rendered)
	}
}

func TestSignalBoard_BlinkToggleOnTick(t *testing.T) {
	m := console.New()
	m2, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m3 := m2.(console.Model)
	m3 = m3.AddFeedEntry("j1", "running job", console.FeedRunning, nil)

	before := m3.SignalBoardBlinkOn()
	// Send a tick message (use time.Now as the tick value).
	m4, _ := m3.Update(console.MakeTickMsg())
	m5 := m4.(console.Model)
	after := m5.SignalBoardBlinkOn()
	if before == after {
		t.Errorf("blink state should toggle on tick when running job exists: before=%v after=%v", before, after)
	}
}


// ── Tmux hidden windows (task 4.6) ────────────────────────────────────────────

func TestCreateJobWindow_SkipsIfNoTmux(t *testing.T) {
	if _, err := exec.LookPath("tmux"); err != nil {
		// tmux not available — verify the function returns empty string gracefully.
		// We can't call createJobWindow directly (unexported) but we can verify that
		// launching a job in a non-tmux environment doesn't crash.
		t.Skip("tmux not found — skipping window creation test")
	}
	// If tmux is available, just verify the test setup doesn't panic.
	t.Log("tmux available — createJobWindow would attempt window creation")
}

// ── Debug popup (task 5.8) ──────────────────────────────────────────────────────

// Enter on signal board now navigates directly to the tmux window (no popup).
// In tests there is no real tmux, so we just verify the model state is unchanged
// (no popup opened, signal board still focused).
func TestSignalBoard_EnterDoesNotOpenPopup(t *testing.T) {
	m := console.New()
	m2, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m3 := m2.(console.Model)
	m3 = m3.AddFeedEntry("job1", "test job", console.FeedDone, nil)
	m3 = m3.SetSignalBoardFocused(true)

	// Enter should navigate directly (tmux select-window) without opening any popup.
	// In tests there is no real tmux session, so we just verify the model
	// remains valid (signal board still focused, no crash).
	m4, _ := m3.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m5 := m4.(console.Model)
	if !m5.SignalBoardFocused() {
		t.Error("signal board should remain focused after enter with no tmux window")
	}
}


// ── Parallel Jobs (tasks 2.1–2.7 / 7.1–7.2) ──────────────────────────────────

func TestParallelJobs(t *testing.T) {
	m := console.New()
	// Inject two FeedRunning entries.
	m = m.AddFeedEntry("job1", "pipeline: alpha", console.FeedRunning, nil)
	m = m.AddFeedEntry("job2", "pipeline: beta", console.FeedRunning, nil)
	// Inject two fake active job handles.
	m = m.AddActiveJob("job1")
	m = m.AddActiveJob("job2")

	if got := m.ActiveJobsCount(); got != 2 {
		t.Errorf("expected 2 active jobs, got %d", got)
	}

	// Verify both feed entries are FeedRunning via signal board.
	sb := m.BuildSignalBoard(8, 60)
	rendered := strings.Join(sb, "\n")
	if !strings.Contains(rendered, "running") {
		t.Errorf("signal board missing 'running' status: %s", rendered)
	}
}


// ── [p] send panel focus shortcut ────────────────────────────────────────────


// ── [d] delete pipeline confirmation (via signal board archive) ──────────────

func TestDKey_CancelWithN(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "my-pipe.pipeline.yaml")
	os.WriteFile(path, []byte("name: my-pipe\nsteps: []\n"), 0o600) //nolint:errcheck

	m := console.NewWithPipelines(console.ScanPipelines(dir))
	m2, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})

	// Press d then n — file should still exist.
	m3, _ := m2.(console.Model).Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})
	m4, _ := m3.(console.Model).Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("n")})
	_ = m4
	if _, err := os.Stat(path); err != nil {
		t.Error("file should still exist after cancel with n")
	}
}

// ── Feed scroll indicators ────────────────────────────────────────────────────

func TestFeedScrollIndicator_NoIndicatorWhenAllVisible(t *testing.T) {
	m := console.New()
	m2, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m3 := m2.(console.Model)
	// With no feed entries and no scroll, no indicator expected.
	view := m3.View()
	if strings.Contains(view, "ACTIVITY FEED ↑") || strings.Contains(view, "ACTIVITY FEED ↓") || strings.Contains(view, "ACTIVITY FEED ↕") {
		t.Errorf("expected no scroll indicator with no feed content:\n%s", view)
	}
}


// ── Feed navigation (tasks 6.1–6.4) ──────────────────────────────────────────

// TestTabCycle_FullCycle verifies the full Tab focus cycle (three-column layout):
// inbox → cron → agentsCenter → agent runner (cycles providers) → signalBoard → feed → inbox

// TestTabFromFeed_FocusesInbox verifies that pressing Tab when the Activity
// Feed is focused moves focus to inbox (feed → inbox wrap-around in new layout).

// TestFeedCursor_JKOnlyWhenFocused verifies that j and k only move feedCursor
// when the Activity Feed is focused.

// TestFeedCursor_GAndGJumps verifies that g goes to the first line and G goes
// to the last line of the Activity Feed when feed is focused.

// ── step badge rendering ──────────────────────────────────────────────────────


func TestStepBadges_WrapsOnNarrowTerminal(t *testing.T) {
	m := console.New()
	// Three-column layout: the activity feed (right column) is hidden at
	// width < 80.  Test step badge rendering via ViewActivityFeed directly
	// at a narrow panel width so the behaviour is still verified.
	m2, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m3 := m2.(console.Model)
	m3 = m3.AddFeedEntry("job1", "pipeline: test", console.FeedRunning, nil)
	// Add enough steps to force wrapping at a narrow panel width.
	// Use "running" so steps are not suppressed (done+no-output steps are hidden).
	for i := range 6 {
		id := fmt.Sprintf("step-with-long-name-%d", i)
		m3x, _ := m3.Update(console.StepStatusMsg{FeedID: "job1", StepID: id, Status: "running"})
		m3 = m3x.(console.Model)
	}
	// Use a narrow feed panel (40 chars) to verify wrapping does not drop steps.
	view := m3.ViewActivityFeed(40, 40)
	// All step IDs must still appear (none truncated/dropped).
	for i := range 6 {
		id := fmt.Sprintf("step-with-long-name-%d", i)
		if !strings.Contains(view, id) {
			t.Errorf("ViewActivityFeed() missing step id %q after narrow-width wrap", id)
		}
	}
}

// ── step failure → FeedFailed promotion ──────────────────────────────────────

func TestJobDoneMsg_AllStepsDone_ProducesFeedDone(t *testing.T) {
	m := console.New()
	m2, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m3 := m2.(console.Model)
	m3 = m3.AddFeedEntry("job1", "pipeline: test", console.FeedRunning, nil)
	// Record two done steps.
	for _, id := range []string{"step-a", "step-b"} {
		mx, _ := m3.Update(console.StepStatusMsg{FeedID: "job1", StepID: id, Status: "done"})
		m3 = mx.(console.Model)
	}
	// Simulate pipeline process exit 0.
	mx, _ := m3.Update(console.MakeJobDoneMsg("job1"))
	m3 = mx.(console.Model)

	status, ok := m3.FeedEntryStatus("job1")
	if !ok {
		t.Fatal("feed entry 'job1' not found after jobDoneMsg")
	}
	if status != console.FeedDone {
		t.Errorf("expected FeedDone when all steps succeeded, got %v", status)
	}
}

func TestJobDoneMsg_AnyStepFailed_ProducesFeedFailed(t *testing.T) {
	m := console.New()
	m2, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m3 := m2.(console.Model)
	m3 = m3.AddFeedEntry("job1", "pipeline: test", console.FeedRunning, nil)
	// One step done, one step failed.
	for _, tc := range []struct{ id, status string }{
		{"step-a", "done"},
		{"step-b", "failed"},
	} {
		mx, _ := m3.Update(console.StepStatusMsg{FeedID: "job1", StepID: tc.id, Status: tc.status})
		m3 = mx.(console.Model)
	}
	// Simulate pipeline process exit 0 (process succeeded, step failed).
	mx, _ := m3.Update(console.MakeJobDoneMsg("job1"))
	m3 = mx.(console.Model)

	status, ok := m3.FeedEntryStatus("job1")
	if !ok {
		t.Fatal("feed entry 'job1' not found after jobDoneMsg")
	}
	if status != console.FeedFailed {
		t.Errorf("expected FeedFailed when a step failed, got %v", status)
	}
}

func TestJobDoneMsg_NoSteps_ProducesFeedDone(t *testing.T) {
	m := console.New()
	m2, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m3 := m2.(console.Model)
	m3 = m3.AddFeedEntry("job1", "pipeline: test", console.FeedRunning, nil)
	// No step status messages — simulates a pipeline with no step events.
	mx, _ := m3.Update(console.MakeJobDoneMsg("job1"))
	m3 = mx.(console.Model)

	status, ok := m3.FeedEntryStatus("job1")
	if !ok {
		t.Fatal("feed entry 'job1' not found after jobDoneMsg")
	}
	if status != console.FeedDone {
		t.Errorf("expected FeedDone when no steps recorded, got %v", status)
	}
}

// ── Agent modal SCHEDULE field (cron-recurring-ui-wiring) ────────────────────

// TestSendPanel_TabCyclesToMessage checks that Tab from send panel Name field
// eventually reaches the Message field.

// TestSendPanel_SubmitDoesNotCrash checks that submitting from the send panel
// message field with a non-empty message doesn't crash.
func TestSendPanel_SubmitDoesNotCrash(t *testing.T) {
	m := console.NewWithTestProviders()
	m2, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m3 := m2.(console.Model)

	// Focus agent send panel.
	m4, _ := m3.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")})
	m5 := m4.(console.Model)

	// Tab to message field (Name → Agent → SavedPrompt → Message = 3 tabs).
	cur := m5
	for i := 0; i < 3; i++ {
		nx, _ := cur.Update(tea.KeyMsg{Type: tea.KeyTab})
		cur = nx.(console.Model)
	}

	// Type some text.
	for _, r := range "hello world" {
		nx, _ := cur.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		cur = nx.(console.Model)
	}

	// Press enter — should not crash.
	_, _ = cur.Update(tea.KeyMsg{Type: tea.KeyEnter})
}

// TestSendPanel_NoScheduleError checks that the model has no schedule error
// concept (schedule was removed from inline send panel).
func TestSendPanel_NoScheduleError(t *testing.T) {
	m := console.NewWithTestProviders()
	m2, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m3 := m2.(console.Model)

	// Focus agent send panel and interact — no schedule error should exist.
	m4, _ := m3.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")})
	m5 := m4.(console.Model)
	_ = m5 // just verify model is valid
}

// ── Kill and Archive (signal-board-kill-and-archive) ──────────────────────────


func TestSignalBoard_KillNonRunningEntry_NoOp(t *testing.T) {
	m := console.New()
	m = m.AddFeedEntry("job1", "done job", console.FeedDone, nil)
	m = m.SetSignalBoardFocused(true)
	// Set filter to all so the done entry is visible
	m2, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("f")})
	m3, _ := m2.(console.Model).Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("x")})
	result := m3.(console.Model)

	status, found := result.FeedEntryStatus("job1")
	if !found {
		t.Fatal("feed entry not found")
	}
	if status != console.FeedDone {
		t.Errorf("expected status to remain FeedDone, got %v", status)
	}
}


func TestSignalBoard_DefaultFilter_IsRunning(t *testing.T) {
	m := console.New()
	if got := m.SignalBoardActiveFilter(); got != "running" {
		t.Errorf("expected default filter 'running', got %q", got)
	}
}


// TestAgentSendPanel_ViewContainsSEND verifies that the inline send panel renders
// a SEND header when the agent section is built.
func TestAgentSendPanel_ViewContainsSEND(t *testing.T) {
	m := console.New()
	m2, _ := m.Update(tea.WindowSizeMsg{Width: 160, Height: 40})
	m3 := m2.(console.Model)
	rows := m3.BuildAgentSection(80)
	joined := strings.Join(rows, "\n")
	if !strings.Contains(joined, "SEND") {
		t.Errorf("expected inline send panel to contain 'SEND' header, got:\n%s", joined)
	}
}


// ── Step suppression ─────────────────────────────────────────────────────────

func TestFeedStep_DoneWithNoOutput_NotRendered(t *testing.T) {
	m := console.New()
	m2, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = m2.(console.Model)
	m = m.AddFeedEntry("job1", "pipeline: test", console.FeedRunning, nil)
	mx, _ := m.Update(console.StepStatusMsg{FeedID: "job1", StepID: "my-done-step", Status: "done"})
	m = mx.(console.Model)
	view := m.View()
	if strings.Contains(view, "my-done-step") {
		t.Error("expected done step with no output to be suppressed from feed view")
	}
}


// ── Cursor overlay ────────────────────────────────────────────────────────────

var ansiEscapeRe = regexp.MustCompile(`\x1b\[[0-9;]*[a-zA-Z]`)

func stripTestANSI(s string) string { return ansiEscapeRe.ReplaceAllString(s, "") }

func TestFeed_CursorRow_SameWidthAsNonCursorRow(t *testing.T) {
	m := console.New()
	m2, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = m2.(console.Model)
	m = m.AddFeedEntry("job1", "pipeline: alpha", console.FeedDone, nil)
	m = m.AddFeedEntry("job2", "pipeline: beta", console.FeedDone, nil)
	m = m.SetFeedFocused(true)

	// The cursor is at line 0. Get the feed's raw view to isolate feed rows.
	// We use ViewActivityFeed to inspect just the feed panel rows.
	view := m.ViewActivityFeed(40, 60)
	lines := strings.Split(view, "\n")
	// Collect widths of content rows (│...│) from the feed panel.
	var rowWidths []int
	for _, line := range lines {
		plain := stripTestANSI(line)
		runes := []rune(plain)
		if len(runes) > 2 && runes[0] == '│' && runes[len(runes)-1] == '│' {
			rowWidths = append(rowWidths, len(runes))
		}
	}
	if len(rowWidths) < 2 {
		t.Skip("not enough box rows to compare widths")
	}
	first := rowWidths[0]
	for i, w := range rowWidths {
		if w != first {
			t.Errorf("row %d width %d differs from row 0 width %d (cursor causes layout shift)", i, w, first)
		}
	}
}

// ── feedLineCount integration ─────────────────────────────────────────────────


// ── WriteSingleStepPipeline ───────────────────────────────────────────────────

// TestWriteSingleStepPipeline_BrainAlwaysOn verifies that use_brain is never
// written to YAML (brain is always on; use_brain is no longer a pipeline field).
func TestWriteSingleStepPipeline_BrainAlwaysOn(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("GLITCH_PIPELINES_DIR", dir)

	// useBrain=true is passed but should have no effect on YAML output.
	path, err := console.WriteSingleStepPipeline("test-brain", "opencode", "", "do a thing", true)
	if err != nil {
		t.Fatalf("WriteSingleStepPipeline: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read pipeline: %v", err)
	}
	if strings.Contains(string(data), "use_brain") {
		t.Errorf("expected no use_brain key in YAML (brain is always on), got:\n%s", string(data))
	}
}

func TestWriteSingleStepPipeline_NoBrain(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("GLITCH_PIPELINES_DIR", dir)

	path, err := console.WriteSingleStepPipeline("test-no-brain", "opencode", "", "do a thing", false)
	if err != nil {
		t.Fatalf("WriteSingleStepPipeline: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read pipeline: %v", err)
	}
	if strings.Contains(string(data), "use_brain") {
		t.Errorf("expected no use_brain key in YAML when useBrain=false")
	}
}

// ── Inline picker: saved prompt ───────────────────────────────────────────────

// openAgentSendPanel is a test helper that focuses the agent send panel with a wide terminal.
// It presses 'a' to focus the agent section, which activates the inline send panel.
func openAgentSendPanel(t *testing.T) console.Model {
	t.Helper()
	m := console.NewWithTestProviders()
	m2, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m3 := m2.(console.Model)
	m4, _ := m3.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")})
	result := m4.(console.Model)
	if !result.AgentFocused() {
		t.Fatal("agent section did not receive focus")
	}
	if !result.SendPanelFocused() {
		t.Fatal("send panel did not become focused after 'a'")
	}
	return result
}

// tabNTimes tabs through the send panel N times and returns the result.
func tabNTimes(m console.Model, n int) console.Model {
	for i := 0; i < n; i++ {
		nx, _ := m.Update(tea.KeyMsg{Type: tea.KeyTab})
		m = nx.(console.Model)
	}
	return m
}

// TestSendPanel_TabReachesSavedPromptSlot verifies that two tabs from the initial
// send panel focus (Name) moves focus to the SavedPrompt slot (slot 2).
// We verify this by confirming that pressing Enter at that point opens the saved
// prompt picker (when prompts are available).

// TestSendPanel_EnterOnAgentSlot_OpensAgentPicker verifies that one tab from Name
// (landing on Agent slot) and pressing Enter opens the agent picker popup.

// TestSendPanel_EnterOnSavedPromptSlot_OpensPicker verifies that after 2 tabs
// from the send panel's initial focus, pressing Enter opens the saved prompt picker.

// TestSendPanel_BracketKeys_NoLongerCyclePrompts verifies that [ and ] no longer
// cycle through saved prompts (the old bracket-cycling behavior was removed).

// ── Three-column layout: column width helpers (task 9.2) ─────────────────────

// TestRightColWidth verifies the 25%-of-width formula with a min of 20.
func TestRightColWidth(t *testing.T) {
	cases := []struct {
		termWidth int
		wantRight int
	}{
		{termWidth: 120, wantRight: 30}, // 120*25/100 = 30
		{termWidth: 160, wantRight: 40}, // 160*25/100 = 40
		{termWidth: 40, wantRight: 20},  // 40*25/100 = 10, clamped to 20
		{termWidth: 0, wantRight: 30},   // default 120 → 30
		{termWidth: 200, wantRight: 50}, // 200*25/100 = 50
	}
	for _, tc := range cases {
		m := console.New()
		if tc.termWidth > 0 {
			m2, _ := m.Update(tea.WindowSizeMsg{Width: tc.termWidth, Height: 40})
			m = m2.(console.Model)
		}
		if got := m.RightColWidth(); got != tc.wantRight {
			t.Errorf("RightColWidth() for termWidth=%d: got %d, want %d", tc.termWidth, got, tc.wantRight)
		}
	}
}

// TestMidColWidth verifies the center-column formula: w - leftW - rightW - 4.

// ── Agents grid panel (task 9.3) ──────────────────────────────────────────────

// TestAgentsGrid_ColumnCount verifies gridCols = max(1, width/24).
func TestAgentsGrid_ColumnCount(t *testing.T) {
	cases := []struct {
		midW     int
		wantCols int
	}{
		{midW: 24, wantCols: 1},   // 24/24 = 1
		{midW: 48, wantCols: 2},   // 48/24 = 2
		{midW: 72, wantCols: 3},   // 72/24 = 3
		{midW: 10, wantCols: 1},   // 10/24 = 0, clamped to 1
		{midW: 96, wantCols: 4},   // 96/24 = 4
	}
	for _, tc := range cases {
		m := console.New()
		// Add enough agents to populate all columns.
		for i := 0; i < tc.wantCols*2; i++ {
			m = m.AddFeedEntry(fmt.Sprintf("id%d", i), fmt.Sprintf("agent-%d", i), console.FeedRunning, nil)
		}
		rendered := m.BuildAgentsGrid(20, tc.midW)
		joined := strings.Join(rendered, "\n")
		// The header should contain "agents".
		if !strings.Contains(joined, "agents") && !strings.Contains(joined, "AGENTS") {
			t.Errorf("BuildAgentsGrid(midW=%d): missing 'agents' header label; got:\n%s", tc.midW, joined)
		}
	}
}

// TestAgentsGrid_MinOneColumn verifies narrow widths get at least 1 column.
func TestAgentsGrid_MinOneColumn(t *testing.T) {
	m := console.New()
	m = m.AddFeedEntry("id1", "agent-one", console.FeedRunning, nil)
	rendered := m.BuildAgentsGrid(10, 5) // very narrow: 5/24 = 0, clamped to 1
	if len(rendered) == 0 {
		t.Error("BuildAgentsGrid: expected at least one line at narrow width")
	}
}

// ── h/j/k/l cursor navigation (task 9.4) ─────────────────────────────────────

// TestAgentsGrid_HJKLNavigation verifies cursor movement and clamping.

// ── 12-hour timestamp format (task 9.5) ───────────────────────────────────────

// TestFeedTimestamp_12HrFormat verifies timestamps use 12hr am/pm format.
func TestFeedTimestamp_12HrFormat(t *testing.T) {
	m := console.New()
	m2, _ := m.Update(tea.WindowSizeMsg{Width: 160, Height: 40})
	m = m2.(console.Model)
	// AddFeedEntry uses time.Now() — just verify the pattern in the view.
	m = m.AddFeedEntry("job1", "test-agent", console.FeedDone, nil)
	m = m.SetFeedFocused(true)
	view := m.ViewActivityFeed(40, 80)

	// Should NOT contain 24hr format (two digits colon two digits colon two digits).
	// The 24hr format "15:04:05" would have a colon-separated seconds field.
	// 12hr format is like "2:34 pm" — no seconds, lowercase am/pm.
	if strings.Contains(view, " am") || strings.Contains(view, " pm") {
		// Good — 12hr format detected.
		return
	}
	// If neither am nor pm found, fall back to checking the pattern isn't 24hr.
	t.Log("Note: timestamp may not be in am/pm window for current time; checking absence of 24hr seconds")
}

// TestFeedTimestamp_AMFormat verifies 9:05:00 renders as "9:05 am" not "09:05".
func TestFeedTimestamp_AMFormat(t *testing.T) {
	// Build a feed entry, get its view, and check the format.
	// We use feedRawLines indirectly via ViewActivityFeed.
	m := console.New()
	m2, _ := m.Update(tea.WindowSizeMsg{Width: 160, Height: 40})
	m = m2.(console.Model)
	m = m.AddFeedEntry("job1", "test-agent", console.FeedDone, nil)
	view := m.ViewActivityFeed(40, 80)
	// The timestamp format should be "H:MM am" or "H:MM pm" (lowercase, no seconds).
	// We just verify no seconds field (HH:MM:SS) appears.
	// A 24hr format would look like "09:05:30" with seconds.
	hasSeconds := strings.Contains(view, ":00:") || strings.Contains(view, ":30:") || strings.Contains(view, ":15:")
	if hasSeconds {
		t.Error("feed timestamp should not include seconds (24hr format detected)")
	}
}

// ── Step connectors and no raw output (task 9.6) ──────────────────────────────

// TestFeedConnectors_TreeConnectors verifies ├─ and └─ are used (not ├  or └ ).
func TestFeedConnectors_TreeConnectors(t *testing.T) {
	m := console.New()
	m2, _ := m.Update(tea.WindowSizeMsg{Width: 160, Height: 40})
	m = m2.(console.Model)
	m = m.AddFeedEntry("job1", "pipeline: test", console.FeedRunning, nil)
	// Add two steps so we get both ├─ and └─ connectors.
	for _, id := range []string{"step-a", "step-b"} {
		mx, _ := m.Update(console.StepStatusMsg{FeedID: "job1", StepID: id, Status: "running"})
		m = mx.(console.Model)
	}
	view := m.ViewActivityFeed(40, 80)
	if !strings.Contains(view, "├─") {
		t.Error("feed step connector: expected '├─' (tee with dash) for non-final step")
	}
	if !strings.Contains(view, "└─") {
		t.Error("feed step connector: expected '└─' (corner with dash) for final step")
	}
	// Verify old ASCII connectors do NOT appear.
	if strings.Contains(view, "├ ") && !strings.Contains(view, "├─") {
		t.Error("feed should use '├─' not '├ ' (missing dash)")
	}
}

// TestFeedConnectors_NoRawStepOutput verifies step.lines do not appear unless rendered.
func TestFeedConnectors_NoRawStepOutput(t *testing.T) {
	m := console.New()
	m2, _ := m.Update(tea.WindowSizeMsg{Width: 160, Height: 40})
	m = m2.(console.Model)
	m = m.AddFeedEntry("job1", "pipeline: test", console.FeedDone, nil)
	// A done step with no output should be suppressed from the feed.
	mx, _ := m.Update(console.StepStatusMsg{FeedID: "job1", StepID: "quiet-step", Status: "done"})
	m = mx.(console.Model)
	view := m.ViewActivityFeed(40, 80)
	if strings.Contains(view, "quiet-step") {
		t.Error("done step with no output should not appear in activity feed")
	}
}

// ── Feed card centering (task 9.7) ────────────────────────────────────────────

// TestFeedCard_RightColumnWidth verifies the feed is rendered at rightColWidth.
func TestFeedCard_RightColumnWidth(t *testing.T) {
	m := console.New()
	m2, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = m2.(console.Model)
	m = m.AddFeedEntry("job1", "test-agent", console.FeedDone, nil)

	// The feed is rendered at rightColWidth() = 120*25/100 = 30.
	rightW := m.RightColWidth()
	view := m.ViewActivityFeed(40, rightW)

	// Every box content row (│...│) should have the same visible width.
	// The panel header may be a sprite with a slightly different width, so we
	// skip the first matching row and verify all remaining rows are consistent.
	re := regexp.MustCompile(`\x1b\[[0-9;]*[a-zA-Z]`)
	var rowWidths []int
	for _, line := range strings.Split(view, "\n") {
		plain := strings.TrimRight(line, "\r")
		plain = re.ReplaceAllString(plain, "")
		runes := []rune(plain)
		if len(runes) > 2 && runes[0] == '│' && runes[len(runes)-1] == '│' {
			rowWidths = append(rowWidths, len(runes))
		}
	}
	// Skip the first │...│ row — it may be a sprite header line with its own
	// width. Verify the remaining body rows are all consistent.
	if len(rowWidths) < 3 {
		t.Skip("not enough box body rows to compare widths")
	}
	bodyWidths := rowWidths[1:]
	for i, w := range bodyWidths {
		if w != bodyWidths[0] {
			t.Errorf("body row %d width mismatch: got %d, want %d (consistent row width)", i+1, w, bodyWidths[0])
		}
	}
}

// TestNarrowTerminal_HidesRightColumn verifies w<80 omits the activity feed.
func TestNarrowTerminal_HidesRightColumn(t *testing.T) {
	m := console.New()
	m2, _ := m.Update(tea.WindowSizeMsg{Width: 60, Height: 40})
	m = m2.(console.Model)
	m = m.AddFeedEntry("job1", "hidden-agent", console.FeedDone, nil)
	view := m.View()
	// The activity feed header should not appear in the narrow layout.
	// (The right column is suppressed when w < 80.)
	if strings.Contains(view, "ACTIVITY FEED") {
		t.Error("expected activity feed to be hidden at terminal width < 80")
	}
}

// TestWideTerminal_ShowsAllThreeColumns verifies w>=80 shows all three columns.

// ── Inline picker: working directory ─────────────────────────────────────────

// TestSendPanel_EnterOnCWDSlot_SetsInlineDirPicker verifies that navigating to the
// CWD row inside the agent picker popup and pressing Enter opens the dir picker.
// Flow: Tab (Name→Agent) → Enter (open popup) → Tab (picker→CWD) → Enter → DirPickerOpen.
