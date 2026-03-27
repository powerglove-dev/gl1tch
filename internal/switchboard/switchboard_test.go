package switchboard_test

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/adam-stokes/orcai/internal/switchboard"
)

// ── scanPipelines ─────────────────────────────────────────────────────────────

func TestScanPipelines_MissingDir(t *testing.T) {
	result := switchboard.ScanPipelines("/tmp/does-not-exist-orcai-test-dir")
	if len(result) != 0 {
		t.Errorf("expected 0 pipelines for missing dir, got %d", len(result))
	}
}

func TestScanPipelines_EmptyDir(t *testing.T) {
	dir := t.TempDir()
	result := switchboard.ScanPipelines(dir)
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

	result := switchboard.ScanPipelines(dir)
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
	pub := switchboard.NewChanPublisher("test-id", ch)
	err := pub.Publish(context.Background(), "step.done", []byte(`{"step":"s1"}`))
	if err != nil {
		t.Fatalf("Publish returned error: %v", err)
	}

	select {
	case msg := <-ch:
		fl, ok := msg.(switchboard.FeedLineMsg)
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

// ── Launcher navigation ───────────────────────────────────────────────────────

func TestLauncherNavDown(t *testing.T) {
	m := switchboard.NewWithPipelines([]string{"alpha", "beta", "gamma"})
	// Initially focused on launcher; cursor at 0.
	m2, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	if got := m2.(switchboard.Model).Cursor(); got != 1 {
		t.Errorf("cursor after j: got %d, want 1", got)
	}
}

func TestLauncherNavUp(t *testing.T) {
	m := switchboard.NewWithPipelines([]string{"alpha", "beta", "gamma"})
	m2, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	m3, _ := m2.(switchboard.Model).Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("k")})
	if got := m3.(switchboard.Model).Cursor(); got != 0 {
		t.Errorf("cursor after j then k: got %d, want 0", got)
	}
}

func TestLauncherNavClampedAtBottom(t *testing.T) {
	m := switchboard.NewWithPipelines([]string{"alpha"})
	m2, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	if got := m2.(switchboard.Model).Cursor(); got != 0 {
		t.Errorf("cursor should stay at 0 with one item: got %d", got)
	}
}

func TestLauncherNavClampedAtTop(t *testing.T) {
	m := switchboard.NewWithPipelines([]string{"alpha"})
	m2, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("k")})
	if got := m2.(switchboard.Model).Cursor(); got != 0 {
		t.Errorf("cursor should not go negative: got %d", got)
	}
}

// ── Agent modal overlay ───────────────────────────────────────────────────────

// TestAgentModalOpenOnEnter asserts that pressing enter when the agent runner
// is focused (and terminal is wide enough) opens the modal overlay.
func TestAgentModalOpenOnEnter(t *testing.T) {
	m := switchboard.NewWithTestProviders()

	// Size the terminal wide enough for the modal.
	m2, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m3 := m2.(switchboard.Model)

	// Focus agent section.
	m4, _ := m3.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")})
	m5 := m4.(switchboard.Model)
	if m5.AgentModalOpen() {
		t.Fatal("modal should not be open before enter")
	}

	// Press enter — modal should open.
	m6, _ := m5.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m7 := m6.(switchboard.Model)
	if !m7.AgentModalOpen() {
		t.Error("expected agent modal to be open after enter")
	}

	// Press ESC — modal should close.
	m8, _ := m7.Update(tea.KeyMsg{Type: tea.KeyEscape})
	m9 := m8.(switchboard.Model)
	if m9.AgentModalOpen() {
		t.Error("expected agent modal to be closed after ESC")
	}
}

// ── View smoke test ───────────────────────────────────────────────────────────

func TestViewContainsBanner(t *testing.T) {
	m := switchboard.New()
	m2, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	view := m2.(switchboard.Model).View()
	if !strings.Contains(view, "ORCAI") {
		t.Errorf("View() missing ORCAI banner:\n%s", view)
	}
	if !strings.Contains(view, "│") {
		t.Errorf("View() missing box-drawing border:\n%s", view)
	}
}

func TestViewContainsPipelinesSection(t *testing.T) {
	m := switchboard.NewWithPipelines([]string{"my-pipeline"})
	m2, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	view := m2.(switchboard.Model).View()
	if !strings.Contains(view, "PIPELINES") {
		t.Errorf("View() missing PIPELINES section:\n%s", view)
	}
	if !strings.Contains(view, "my-pipeline") {
		t.Errorf("View() missing pipeline name:\n%s", view)
	}
}

func TestViewContainsActivityFeed(t *testing.T) {
	m := switchboard.New()
	m2, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	view := m2.(switchboard.Model).View()
	if !strings.Contains(view, "ACTIVITY FEED") {
		t.Errorf("View() missing ACTIVITY FEED section:\n%s", view)
	}
}

func TestViewContainsBottomBar(t *testing.T) {
	// When launcher is focused (default), bottom bar is hidden to avoid
	// double-bar awkwardness with the tmux status bar.
	m := switchboard.New()
	m2, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m3 := m2.(switchboard.Model)
	// Shift focus off launcher via Tab to reach the agent panel.
	m4, _ := m3.Update(tea.KeyMsg{Type: tea.KeyTab})
	view := m4.(switchboard.Model).View()
	if !strings.Contains(view, "ctrl+s") {
		t.Errorf("View() bottom bar missing hint when agent focused:\n%s", view)
	}
	// Note: "quit" was moved to the tmux status bar (^spc q quit) and is no
	// longer rendered in the BubbleTea bottom bar.
}

// ── Feed scroll (task 1.6) ─────────────────────────────────────────────────────

func TestFeedScrollOffset_ClampedAtZero(t *testing.T) {
	m := switchboard.New()
	m2, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m3 := m2.(switchboard.Model)
	// Press up — offset should stay at 0.
	m4, _ := m3.Update(tea.KeyMsg{Type: tea.KeyUp})
	m5 := m4.(switchboard.Model)
	if got := m5.FeedScrollOffset(); got != 0 {
		t.Errorf("feedScrollOffset should be 0 at top, got %d", got)
	}
}

func TestFeedScrollOffset_InitialIsZero(t *testing.T) {
	m := switchboard.New()
	m2, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 10})
	m3 := m2.(switchboard.Model)
	// Add many feed entries with output lines so total lines exceed visible height.
	for i := 0; i < 30; i++ {
		lines := make([]string, 5)
		for j := range lines {
			lines[j] = "output line"
		}
		m3 = m3.AddFeedEntry("id", "title", switchboard.FeedDone, lines)
	}
	// Verify offset is 0 by default.
	if got := m3.FeedScrollOffset(); got != 0 {
		t.Errorf("initial feedScrollOffset should be 0, got %d", got)
	}
}

func TestFeedScrollOffset_ResetOnNewEntry(t *testing.T) {
	m := switchboard.New()
	m2, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 10})
	m3 := m2.(switchboard.Model)
	// Add a feed entry — scroll offset should be 0.
	m4 := m3.AddFeedEntry("id1", "first job", switchboard.FeedDone, []string{"line"})
	if got := m4.FeedScrollOffset(); got != 0 {
		t.Errorf("feedScrollOffset should be 0 after new entry, got %d", got)
	}
}

func TestFeedScrollOffset_ClampedAtMax(t *testing.T) {
	m := switchboard.New()
	m2, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m3 := m2.(switchboard.Model)
	// Add feed entries with lines.
	for i := 0; i < 5; i++ {
		m3 = m3.AddFeedEntry("id", "title", switchboard.FeedDone, []string{"a", "b"})
	}
	// View should still render without crashing.
	view := m3.View()
	if !strings.Contains(view, "ACTIVITY FEED") {
		t.Errorf("View() should still contain ACTIVITY FEED after clamping, got: %s", view)
	}
}

// ── Agent section fixed height (task 2.6) ──────────────────────────────────────

func TestBuildAgentSection_FixedHeight(t *testing.T) {
	m := switchboard.NewWithTestProviders()
	m2, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m3 := m2.(switchboard.Model)

	// Measure height at step 0.
	m3a, _ := m3.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")})
	m3b := m3a.(switchboard.Model)
	step0Lines := m3b.BuildAgentSection(60)

	// Advance to step 1.
	m4, _ := m3b.Update(tea.KeyMsg{Type: tea.KeyTab})
	m5 := m4.(switchboard.Model)
	step1Lines := m5.BuildAgentSection(60)

	// Advance to step 2.
	m6, _ := m5.Update(tea.KeyMsg{Type: tea.KeyTab})
	m7 := m6.(switchboard.Model)
	step2Lines := m7.BuildAgentSection(60)

	if len(step0Lines) != len(step1Lines) {
		t.Errorf("buildAgentSection step 0 vs step 1 line count mismatch: %d vs %d", len(step0Lines), len(step1Lines))
	}
	if len(step0Lines) != len(step2Lines) {
		t.Errorf("buildAgentSection step 0 vs step 2 line count mismatch: %d vs %d", len(step0Lines), len(step2Lines))
	}
}

// ── Signal board (task 3.8) ────────────────────────────────────────────────────

func TestSignalBoard_FilterAll(t *testing.T) {
	m := switchboard.New()
	m2, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m3 := m2.(switchboard.Model)
	m3 = m3.AddFeedEntry("j1", "running job", switchboard.FeedRunning, nil)
	m3 = m3.AddFeedEntry("j2", "done job", switchboard.FeedDone, nil)
	m3 = m3.AddFeedEntry("j3", "failed job", switchboard.FeedFailed, nil)

	sb := m3.BuildSignalBoard(8, 60)
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
	m := switchboard.New()
	m2, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m3 := m2.(switchboard.Model)
	m3 = m3.AddFeedEntry("j1", "running job", switchboard.FeedRunning, nil)

	before := m3.SignalBoardBlinkOn()
	// Send a tick message (use time.Now as the tick value).
	m4, _ := m3.Update(switchboard.MakeTickMsg())
	m5 := m4.(switchboard.Model)
	after := m5.SignalBoardBlinkOn()
	if before == after {
		t.Errorf("blink state should toggle on tick when running job exists: before=%v after=%v", before, after)
	}
}

func TestSignalBoard_HeaderContainsFilter(t *testing.T) {
	m := switchboard.New()
	m2, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m3 := m2.(switchboard.Model)
	sb := m3.BuildSignalBoard(8, 60)
	rendered := strings.Join(sb, "\n")
	if !strings.Contains(rendered, "SIGNAL BOARD") {
		t.Errorf("signal board missing 'SIGNAL BOARD' header: %s", rendered)
	}
	if !strings.Contains(rendered, "all") {
		t.Errorf("signal board missing filter 'all' in header: %s", rendered)
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
	m := switchboard.New()
	m2, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m3 := m2.(switchboard.Model)
	m3 = m3.AddFeedEntry("job1", "test job", switchboard.FeedDone, nil)
	m3 = m3.SetSignalBoardFocused(true)

	// Enter should navigate directly (tmux select-window) without opening any popup.
	// In tests there is no real tmux session, so we just verify the model
	// remains valid (signal board still focused, no crash).
	m4, _ := m3.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m5 := m4.(switchboard.Model)
	if !m5.SignalBoardFocused() {
		t.Error("signal board should remain focused after enter with no tmux window")
	}
}

func TestSignalBoard_ViewContainsSignalBoard(t *testing.T) {
	m := switchboard.New()
	m2, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	view := m2.(switchboard.Model).View()
	if !strings.Contains(view, "SIGNAL BOARD") {
		t.Errorf("View() missing SIGNAL BOARD section:\n%s", view)
	}
}

// ── Parallel Jobs (tasks 2.1–2.7 / 7.1–7.2) ──────────────────────────────────

func TestParallelJobs(t *testing.T) {
	m := switchboard.New()
	// Inject two FeedRunning entries.
	m = m.AddFeedEntry("job1", "pipeline: alpha", switchboard.FeedRunning, nil)
	m = m.AddFeedEntry("job2", "pipeline: beta", switchboard.FeedRunning, nil)
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

	// View should show [2 running] badge.
	m2, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	view := m2.(switchboard.Model).View()
	if !strings.Contains(view, "2 running") {
		t.Errorf("expected view to show '2 running', got:\n%s", view)
	}
}

func TestParallelJobCap(t *testing.T) {
	m := switchboard.New()
	cap := switchboard.MaxParallelJobs()

	// Fill activeJobs to the cap.
	for i := 0; i < cap; i++ {
		m = m.AddActiveJob(fmt.Sprintf("job%d", i))
	}
	if got := m.ActiveJobsCount(); got != cap {
		t.Fatalf("expected %d active jobs before cap check, got %d", cap, got)
	}

	// Give the model some pipelines so we can try to launch.
	m = switchboard.NewWithPipelines([]string{"test-pipeline"})
	// Re-inject active jobs after creating new model.
	for i := 0; i < cap; i++ {
		m = m.AddActiveJob(fmt.Sprintf("job%d", i))
	}

	// Try to launch another job via Enter key.
	m2, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m3 := m2.(switchboard.Model)

	// activeJobs count should still be cap (no new job added).
	if got := m3.ActiveJobsCount(); got != cap {
		t.Errorf("expected activeJobs count to stay at cap %d, got %d", cap, got)
	}

	// A warning feed entry should have been added.
	view := m3.View()
	if !strings.Contains(view, "max parallel") {
		t.Errorf("expected warning 'max parallel' in view after cap exceeded:\n%s", view)
	}
}

// ── [p] pipelines focus shortcut ─────────────────────────────────────────────

func TestPKeyFocusesPipelines_FromAgent(t *testing.T) {
	m := switchboard.New()
	m2, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	// Focus agent section.
	m3, _ := m2.(switchboard.Model).Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")})
	m4 := m3.(switchboard.Model)

	// Press p — should focus pipelines (launcher).
	m5, _ := m4.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("p")})
	m6 := m5.(switchboard.Model)
	if m6.Cursor() == -1 {
		// Cursor() reads launcher.selected; if it panics we have a problem.
		t.Fatal("launcher should be accessible after p key")
	}
	// Signal board and feed should not be focused — verified by view rendering.
	view := m6.View()
	if !strings.Contains(view, "PIPELINES") {
		t.Errorf("expected PIPELINES panel in view after p key:\n%s", view)
	}
}

func TestPKeyFocusesPipelines_FromFeed(t *testing.T) {
	m := switchboard.New()
	m2, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	// Focus feed.
	m3, _ := m2.(switchboard.Model).Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("f")})
	// Press p — should focus pipelines.
	m4, _ := m3.(switchboard.Model).Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("p")})
	view := m4.(switchboard.Model).View()
	if !strings.Contains(view, "PIPELINES") {
		t.Errorf("expected PIPELINES panel in view after p key from feed:\n%s", view)
	}
}

// ── [d] delete pipeline confirmation ─────────────────────────────────────────

func TestDKey_ShowsConfirmation(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "my-pipe.pipeline.yaml")
	if err := os.WriteFile(path, []byte("name: my-pipe\nsteps: []\n"), 0o600); err != nil {
		t.Fatalf("write file: %v", err)
	}
	m := switchboard.NewWithPipelines(switchboard.ScanPipelines(dir))
	m2, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})

	// Press d — confirmation modal should appear in view.
	m3, _ := m2.(switchboard.Model).Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})
	view := m3.(switchboard.Model).View()
	if !strings.Contains(view, "Delete") {
		t.Errorf("expected Delete confirmation in view after d key:\n%s", view)
	}
}

func TestDKey_CancelWithN(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "my-pipe.pipeline.yaml")
	os.WriteFile(path, []byte("name: my-pipe\nsteps: []\n"), 0o600) //nolint:errcheck

	m := switchboard.NewWithPipelines(switchboard.ScanPipelines(dir))
	m2, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})

	// Press d then n — file should still exist.
	m3, _ := m2.(switchboard.Model).Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})
	m4, _ := m3.(switchboard.Model).Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("n")})
	_ = m4
	if _, err := os.Stat(path); err != nil {
		t.Error("file should still exist after cancel with n")
	}
}

// ── Feed scroll indicators ────────────────────────────────────────────────────

func TestFeedScrollIndicator_NoIndicatorWhenAllVisible(t *testing.T) {
	m := switchboard.New()
	m2, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m3 := m2.(switchboard.Model)
	// With no feed entries and no scroll, no indicator expected.
	view := m3.View()
	if strings.Contains(view, "ACTIVITY FEED ↑") || strings.Contains(view, "ACTIVITY FEED ↓") || strings.Contains(view, "ACTIVITY FEED ↕") {
		t.Errorf("expected no scroll indicator with no feed content:\n%s", view)
	}
}

func TestFeedScrollIndicator_DownWhenContentBelow(t *testing.T) {
	m := switchboard.New()
	m2, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 20})
	m3 := m2.(switchboard.Model)
	// Add enough entries to overflow the visible height.
	for i := range 30 {
		m3 = m3.AddFeedEntry(fmt.Sprintf("job%d", i), fmt.Sprintf("pipeline: job%d", i), switchboard.FeedDone, []string{"output line"})
	}
	view := m3.View()
	if !strings.Contains(view, "↓") {
		t.Errorf("expected ↓ indicator when content extends below viewport:\n%s", view)
	}
}

// ── Feed navigation (tasks 6.1–6.4) ──────────────────────────────────────────

// TestTabCycle_FullCycle verifies the full Tab focus cycle:
// launcher → agent → signalBoard → inbox → feed → launcher
func TestTabCycle_FullCycle(t *testing.T) {
	m := switchboard.NewWithPipelines([]string{"alpha", "beta"})
	// Start: launcher focused (default).

	// Tab 1: launcher → agent
	m2, _ := m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m2m := m2.(switchboard.Model)
	if m2m.SignalBoardFocused() || m2m.FeedFocused() {
		t.Error("after 1 Tab: expected agent focused, got signalBoard or feed")
	}

	// Tab 2: agent → signalBoard
	m3, _ := m2m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m3m := m3.(switchboard.Model)
	if !m3m.SignalBoardFocused() {
		t.Error("after 2 Tabs: expected signalBoard focused")
	}

	// Tab 3: signalBoard → inbox (new step in cycle)
	m4, _ := m3m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m4m := m4.(switchboard.Model)
	if m4m.FeedFocused() || m4m.SignalBoardFocused() {
		t.Error("after 3 Tabs: expected inbox focused, got feed or signalBoard")
	}

	// Tab 4: inbox → feed
	m5, _ := m4m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m5m := m5.(switchboard.Model)
	if !m5m.FeedFocused() {
		t.Error("after 4 Tabs: expected feed focused")
	}

	// Tab 5: feed → launcher — add entries first so feedCursor could move if broken
	m5m = m5m.AddFeedEntry("id1", "job one", switchboard.FeedDone, []string{"line a", "line b"})
	m6, _ := m5m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m6m := m6.(switchboard.Model)
	if m6m.FeedFocused() || m6m.SignalBoardFocused() {
		t.Error("after 5 Tabs: expected launcher focused")
	}
	// j should move launcher cursor now, not feedCursor
	cursorBefore := m6m.FeedCursor()
	m7, _ := m6m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	m7m := m7.(switchboard.Model)
	if m7m.FeedCursor() != cursorBefore {
		t.Errorf("feedCursor should not change when launcher is focused, got %d → %d", cursorBefore, m7m.FeedCursor())
	}
}

// TestTabFromFeed_FocusesCron verifies that pressing Tab when the Activity
// Feed is focused moves focus to the Cron panel (feed → cron → launcher cycle).
func TestTabFromFeed_FocusesCron(t *testing.T) {
	m := switchboard.NewWithPipelines([]string{"alpha", "beta"})
	m = m.SetFeedFocused(true)

	// First Tab: feed → cron.
	m2, _ := m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m3 := m2.(switchboard.Model)
	if !m3.CronPanelFocused() {
		t.Errorf("expected cron panel focused after Tab-from-feed")
	}

	// Second Tab: cron → launcher. Feed cursor should not move.
	m3 = m3.AddFeedEntry("id1", "job one", switchboard.FeedDone, []string{"line a", "line b"})
	cursorBefore := m3.FeedCursor()
	m4, _ := m3.Update(tea.KeyMsg{Type: tea.KeyTab})
	m5 := m4.(switchboard.Model)
	if m5.CronPanelFocused() {
		t.Errorf("expected cron panel unfocused after second Tab")
	}
	m6, _ := m5.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	m7 := m6.(switchboard.Model)
	if m7.FeedCursor() != cursorBefore {
		t.Errorf("feedCursor should not change when launcher is focused, got %d → %d",
			cursorBefore, m7.FeedCursor())
	}
}

// TestFeedCursor_JKOnlyWhenFocused verifies that j and k only move feedCursor
// when the Activity Feed is focused.
func TestFeedCursor_JKOnlyWhenFocused(t *testing.T) {
	// Scenario A: feed NOT focused — j and k should not change feedCursor.
	m := switchboard.New()
	m = m.AddFeedEntry("id1", "job one", switchboard.FeedDone, []string{"line a", "line b", "line c"})
	m = m.AddFeedEntry("id2", "job two", switchboard.FeedDone, []string{"line d", "line e"})
	// Default state: launcher focused, feed not focused.
	m2, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	if got := m2.(switchboard.Model).FeedCursor(); got != 0 {
		t.Errorf("feedCursor should be 0 when feed not focused after j, got %d", got)
	}
	m3, _ := m2.(switchboard.Model).Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("k")})
	if got := m3.(switchboard.Model).FeedCursor(); got != 0 {
		t.Errorf("feedCursor should be 0 when feed not focused after k, got %d", got)
	}

	// Scenario B: feed focused — j should advance feedCursor.
	m4 := switchboard.New()
	m4 = m4.AddFeedEntry("id1", "job one", switchboard.FeedDone, []string{"line a", "line b", "line c"})
	m4 = m4.AddFeedEntry("id2", "job two", switchboard.FeedDone, []string{"line d", "line e"})
	m4 = m4.SetFeedFocused(true)
	m5, _ := m4.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	if got := m5.(switchboard.Model).FeedCursor(); got == 0 {
		t.Error("feedCursor should advance after j when feed is focused")
	}

	// Scenario C: feed focused — k after j should decrement.
	m6, _ := m5.(switchboard.Model).Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("k")})
	if got := m6.(switchboard.Model).FeedCursor(); got != 0 {
		t.Errorf("feedCursor should decrement back to 0 after j then k, got %d", got)
	}
}

// TestFeedCursor_GAndGJumps verifies that g goes to the first line and G goes
// to the last line of the Activity Feed when feed is focused.
func TestFeedCursor_GAndGJumps(t *testing.T) {
	m := switchboard.New()
	// Add entries with enough lines so last-line index is meaningfully > 0.
	for i := range 5 {
		m = m.AddFeedEntry(fmt.Sprintf("job%d", i), fmt.Sprintf("pipeline %d", i), switchboard.FeedDone,
			[]string{"output line 1", "output line 2", "output line 3"})
	}
	m = m.SetFeedFocused(true)

	// Press G — should jump to last line.
	m2, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("G")})
	m3 := m2.(switchboard.Model)
	if m3.FeedCursor() == 0 {
		t.Error("feedCursor should be > 0 after G (jump to last line)")
	}
	lastCursor := m3.FeedCursor()

	// Press g — should jump back to first line (0).
	m4, _ := m3.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("g")})
	m5 := m4.(switchboard.Model)
	if m5.FeedCursor() != 0 {
		t.Errorf("feedCursor should be 0 after g (jump to first line), got %d", m5.FeedCursor())
	}

	// Press G again — should restore last-line index.
	m6, _ := m5.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("G")})
	m7 := m6.(switchboard.Model)
	if m7.FeedCursor() != lastCursor {
		t.Errorf("feedCursor after second G should match first G result: want %d, got %d", lastCursor, m7.FeedCursor())
	}
}

// ── step badge rendering ──────────────────────────────────────────────────────

func TestStepBadges_GlyphsPresent(t *testing.T) {
	m := switchboard.New()
	m2, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m3 := m2.(switchboard.Model)
	m3 = m3.AddFeedEntry("job1", "pipeline: test", switchboard.FeedRunning, nil)
	// Inject step statuses for all four states.
	for _, tc := range []struct {
		id     string
		status string
		glyph  string
	}{
		{"step-pending", "pending", "·"},
		{"step-running", "running", "»"},
		{"step-done", "done", "°"},
		{"step-failed", "failed", "×"},
	} {
		m3x, _ := m3.Update(switchboard.StepStatusMsg{FeedID: "job1", StepID: tc.id, Status: tc.status})
		m3 = m3x.(switchboard.Model)
	}
	view := m3.View()
	for _, glyph := range []string{"·", "»", "°", "×"} {
		if !strings.Contains(view, glyph) {
			t.Errorf("View() missing step glyph %q in feed output:\n%s", glyph, view)
		}
	}
}

func TestStepBadges_SingleRowFewSteps(t *testing.T) {
	m := switchboard.New()
	m2, _ := m.Update(tea.WindowSizeMsg{Width: 200, Height: 40})
	m3 := m2.(switchboard.Model)
	m3 = m3.AddFeedEntry("job1", "pipeline: test", switchboard.FeedRunning, nil)
	for i := range 3 {
		id := fmt.Sprintf("step-%d", i)
		m3x, _ := m3.Update(switchboard.StepStatusMsg{FeedID: "job1", StepID: id, Status: "done"})
		m3 = m3x.(switchboard.Model)
	}
	view := m3.View()
	// With a wide terminal and only 3 short steps all badges should fit on one line.
	// Verify all three step IDs appear somewhere in the view.
	for i := range 3 {
		id := fmt.Sprintf("step-%d", i)
		if !strings.Contains(view, id) {
			t.Errorf("View() missing step id %q in rendered output", id)
		}
	}
}

func TestStepBadges_WrapsOnNarrowTerminal(t *testing.T) {
	m := switchboard.New()
	// Use a narrow terminal that cannot fit many step badges on one line.
	m2, _ := m.Update(tea.WindowSizeMsg{Width: 40, Height: 40})
	m3 := m2.(switchboard.Model)
	m3 = m3.AddFeedEntry("job1", "pipeline: test", switchboard.FeedRunning, nil)
	// Add enough steps to force wrapping at width 40.
	for i := range 6 {
		id := fmt.Sprintf("step-with-long-name-%d", i)
		m3x, _ := m3.Update(switchboard.StepStatusMsg{FeedID: "job1", StepID: id, Status: "done"})
		m3 = m3x.(switchboard.Model)
	}
	view := m3.View()
	// All step IDs must still appear (none truncated/dropped).
	for i := range 6 {
		id := fmt.Sprintf("step-with-long-name-%d", i)
		if !strings.Contains(view, id) {
			t.Errorf("View() missing step id %q after narrow-width wrap", id)
		}
	}
}

// ── step failure → FeedFailed promotion ──────────────────────────────────────

func TestJobDoneMsg_AllStepsDone_ProducesFeedDone(t *testing.T) {
	m := switchboard.New()
	m2, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m3 := m2.(switchboard.Model)
	m3 = m3.AddFeedEntry("job1", "pipeline: test", switchboard.FeedRunning, nil)
	// Record two done steps.
	for _, id := range []string{"step-a", "step-b"} {
		mx, _ := m3.Update(switchboard.StepStatusMsg{FeedID: "job1", StepID: id, Status: "done"})
		m3 = mx.(switchboard.Model)
	}
	// Simulate pipeline process exit 0.
	mx, _ := m3.Update(switchboard.MakeJobDoneMsg("job1"))
	m3 = mx.(switchboard.Model)

	status, ok := m3.FeedEntryStatus("job1")
	if !ok {
		t.Fatal("feed entry 'job1' not found after jobDoneMsg")
	}
	if status != switchboard.FeedDone {
		t.Errorf("expected FeedDone when all steps succeeded, got %v", status)
	}
}

func TestJobDoneMsg_AnyStepFailed_ProducesFeedFailed(t *testing.T) {
	m := switchboard.New()
	m2, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m3 := m2.(switchboard.Model)
	m3 = m3.AddFeedEntry("job1", "pipeline: test", switchboard.FeedRunning, nil)
	// One step done, one step failed.
	for _, tc := range []struct{ id, status string }{
		{"step-a", "done"},
		{"step-b", "failed"},
	} {
		mx, _ := m3.Update(switchboard.StepStatusMsg{FeedID: "job1", StepID: tc.id, Status: tc.status})
		m3 = mx.(switchboard.Model)
	}
	// Simulate pipeline process exit 0 (process succeeded, step failed).
	mx, _ := m3.Update(switchboard.MakeJobDoneMsg("job1"))
	m3 = mx.(switchboard.Model)

	status, ok := m3.FeedEntryStatus("job1")
	if !ok {
		t.Fatal("feed entry 'job1' not found after jobDoneMsg")
	}
	if status != switchboard.FeedFailed {
		t.Errorf("expected FeedFailed when a step failed, got %v", status)
	}
}

func TestJobDoneMsg_NoSteps_ProducesFeedDone(t *testing.T) {
	m := switchboard.New()
	m2, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m3 := m2.(switchboard.Model)
	m3 = m3.AddFeedEntry("job1", "pipeline: test", switchboard.FeedRunning, nil)
	// No step status messages — simulates a pipeline with no step events.
	mx, _ := m3.Update(switchboard.MakeJobDoneMsg("job1"))
	m3 = mx.(switchboard.Model)

	status, ok := m3.FeedEntryStatus("job1")
	if !ok {
		t.Fatal("feed entry 'job1' not found after jobDoneMsg")
	}
	if status != switchboard.FeedDone {
		t.Errorf("expected FeedDone when no steps recorded, got %v", status)
	}
}

// ── Agent modal SCHEDULE field (cron-recurring-ui-wiring) ────────────────────

// TestAgentModal_TabCyclesToScheduleSlot checks that Tab from slot 2 (PROMPT)
// reaches slot 3 (SCHEDULE).
func TestAgentModal_TabCyclesToScheduleSlot(t *testing.T) {
	m := switchboard.NewWithTestProviders()
	m2, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m3 := m2.(switchboard.Model)

	// Focus agent and open modal.
	m4, _ := m3.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")})
	m5, _ := m4.(switchboard.Model).Update(tea.KeyMsg{Type: tea.KeyEnter})
	m6 := m5.(switchboard.Model)

	if !m6.AgentModalOpen() {
		t.Fatal("expected agent modal to be open")
	}

	// Tab three times: slot 0 → 1 → 2 → 3.
	cur := m6
	for i := 0; i < 3; i++ {
		nx, _ := cur.Update(tea.KeyMsg{Type: tea.KeyTab})
		cur = nx.(switchboard.Model)
	}

	if got := cur.AgentModalFocus(); got != 3 {
		t.Errorf("expected agentModalFocus == 3 (SCHEDULE), got %d", got)
	}
}

// TestAgentModal_SubmitBlankSchedule checks that submit with a blank SCHEDULE
// field does not add a cron entry (run-now path) — model simply processes the
// submission without error.
func TestAgentModal_SubmitBlankSchedule(t *testing.T) {
	m := switchboard.NewWithTestProviders()
	m2, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m3 := m2.(switchboard.Model)

	// Focus agent and open modal.
	m4, _ := m3.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")})
	m5, _ := m4.(switchboard.Model).Update(tea.KeyMsg{Type: tea.KeyEnter})
	m6 := m5.(switchboard.Model)

	// Tab to PROMPT (slot 2) and type a prompt.
	m7, _ := m6.Update(tea.KeyMsg{Type: tea.KeyTab})
	m8, _ := m7.(switchboard.Model).Update(tea.KeyMsg{Type: tea.KeyTab})
	// Now at slot 2 — type some text.
	for _, r := range "test prompt" {
		nx, _ := m8.(switchboard.Model).Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		m8 = nx
	}

	// SCHEDULE is blank — submit via ctrl+s. The run-now path will try to
	// launch an agent (may fail without real provider). Just check modal closes
	// or, if prompt is empty for some reason, at least no crash.
	// We accept either modal-closed or modal-open-with-no-error.
	m9, _ := m8.(switchboard.Model).Update(tea.KeyMsg{Type: tea.KeyCtrlS})
	m10 := m9.(switchboard.Model)

	if m10.AgentScheduleErr() != "" {
		t.Errorf("expected no schedule error with blank schedule, got: %q", m10.AgentScheduleErr())
	}
}

// TestAgentModal_InvalidScheduleShowsError checks that an invalid cron expression
// sets the agentScheduleErr field.
func TestAgentModal_InvalidScheduleShowsError(t *testing.T) {
	m := switchboard.NewWithTestProviders()
	m2, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m3 := m2.(switchboard.Model)

	// Focus agent, open modal.
	m4, _ := m3.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")})
	m5, _ := m4.(switchboard.Model).Update(tea.KeyMsg{Type: tea.KeyEnter})
	m6 := m5.(switchboard.Model)

	// Tab to PROMPT (slot 2) and enter a prompt.
	m7, _ := m6.Update(tea.KeyMsg{Type: tea.KeyTab})
	m8, _ := m7.(switchboard.Model).Update(tea.KeyMsg{Type: tea.KeyTab})
	for _, r := range "hello" {
		nx, _ := m8.(switchboard.Model).Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		m8 = nx
	}

	// Tab to CWD (slot 3) then SCHEDULE (slot 4) and type an invalid cron expression.
	m9tmp, _ := m8.(switchboard.Model).Update(tea.KeyMsg{Type: tea.KeyTab})
	m9, _ := m9tmp.(switchboard.Model).Update(tea.KeyMsg{Type: tea.KeyTab})
	for _, r := range "not-a-valid-cron" {
		nx, _ := m9.(switchboard.Model).Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		m9 = nx
	}

	// Submit — should set schedule error.
	m10, _ := m9.(switchboard.Model).Update(tea.KeyMsg{Type: tea.KeyCtrlS})
	m11 := m10.(switchboard.Model)

	if m11.AgentScheduleErr() == "" {
		t.Error("expected agentScheduleErr to be set for invalid cron expression")
	}
	if !m11.AgentModalOpen() {
		t.Error("modal should remain open when schedule has an error")
	}
}

// ── Pipeline Launcher mode-select overlay ─────────────────────────────────────

// TestPipelineLauncher_EnterShowsModeSelect verifies that pressing Enter in the
// pipeline launcher (with pipelines available) opens the mode-select overlay.
func TestPipelineLauncher_EnterShowsModeSelect(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "my-pipe.pipeline.yaml"),
		[]byte("name: my-pipe\nsteps: []\n"), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	m := switchboard.NewWithPipelines(switchboard.ScanPipelines(dir))
	m2, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})

	// Press Enter in launcher — mode-select overlay should open.
	m3, _ := m2.(switchboard.Model).Update(tea.KeyMsg{Type: tea.KeyEnter})
	m4 := m3.(switchboard.Model)

	if got := m4.PipelineLaunchMode(); got != switchboard.PlModeSelect() {
		t.Errorf("expected PipelineLaunchMode == plModeSelect (%d), got %d", switchboard.PlModeSelect(), got)
	}
}

// TestPipelineLauncher_EscResetsModeSelect verifies that Esc from the
// mode-select overlay resets the launch mode to none.
func TestPipelineLauncher_EscResetsModeSelect(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "my-pipe.pipeline.yaml"), //nolint:errcheck
		[]byte("name: my-pipe\nsteps: []\n"), 0o600)
	m := switchboard.NewWithPipelines(switchboard.ScanPipelines(dir))
	m2, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})

	m3, _ := m2.(switchboard.Model).Update(tea.KeyMsg{Type: tea.KeyEnter})
	m4, _ := m3.(switchboard.Model).Update(tea.KeyMsg{Type: tea.KeyEscape})
	m5 := m4.(switchboard.Model)

	if got := m5.PipelineLaunchMode(); got != switchboard.PlModeNone() {
		t.Errorf("expected PipelineLaunchMode == plModeNone after Esc, got %d", got)
	}
}

// TestPipelineLauncher_DownMovesSelection verifies Down/j moves the cursor in
// the mode-select overlay.
func TestPipelineLauncher_DownMovesSelection(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "pipe.pipeline.yaml"), //nolint:errcheck
		[]byte("name: pipe\nsteps: []\n"), 0o600)
	m := switchboard.NewWithPipelines(switchboard.ScanPipelines(dir))
	m2, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})

	m3, _ := m2.(switchboard.Model).Update(tea.KeyMsg{Type: tea.KeyEnter})
	m4, _ := m3.(switchboard.Model).Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	m5 := m4.(switchboard.Model)

	if got := m5.PipelineModeSelected(); got != 1 {
		t.Errorf("expected pipelineModeSelected == 1 after j, got %d", got)
	}
}

// TestPipelineLauncher_SelectScheduleTransitionsToInput verifies that selecting
// "Schedule recurring" (item 1) in the mode-select overlay transitions to the
// schedule-input state.
func TestPipelineLauncher_SelectScheduleTransitionsToInput(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "pipe.pipeline.yaml"), //nolint:errcheck
		[]byte("name: pipe\nsteps: []\n"), 0o600)
	m := switchboard.NewWithPipelines(switchboard.ScanPipelines(dir))
	m2, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})

	// Enter mode-select, move to Schedule, press Enter.
	m3, _ := m2.(switchboard.Model).Update(tea.KeyMsg{Type: tea.KeyEnter})
	m4, _ := m3.(switchboard.Model).Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	m5, _ := m4.(switchboard.Model).Update(tea.KeyMsg{Type: tea.KeyEnter})
	m6 := m5.(switchboard.Model)

	if got := m6.PipelineLaunchMode(); got != switchboard.PlScheduleInput() {
		t.Errorf("expected PipelineLaunchMode == plScheduleInput (%d), got %d", switchboard.PlScheduleInput(), got)
	}
}

// TestPipelineScheduleInput_InvalidCronShowsError verifies that submitting an
// invalid cron expression in the schedule-input overlay sets PipelineScheduleErr.
func TestPipelineScheduleInput_InvalidCronShowsError(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "pipe.pipeline.yaml"), //nolint:errcheck
		[]byte("name: pipe\nsteps: []\n"), 0o600)
	m := switchboard.NewWithPipelines(switchboard.ScanPipelines(dir))
	m2, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})

	// Enter mode-select → schedule-input.
	m3, _ := m2.(switchboard.Model).Update(tea.KeyMsg{Type: tea.KeyEnter})
	m4, _ := m3.(switchboard.Model).Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	m5, _ := m4.(switchboard.Model).Update(tea.KeyMsg{Type: tea.KeyEnter})

	// Type an invalid cron expression.
	cur := m5
	for _, r := range "bad cron" {
		nx, _ := cur.(switchboard.Model).Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		cur = nx
	}
	// Submit.
	m6, _ := cur.(switchboard.Model).Update(tea.KeyMsg{Type: tea.KeyCtrlS})
	m7 := m6.(switchboard.Model)

	if m7.PipelineScheduleErr() == "" {
		t.Error("expected PipelineScheduleErr to be set for invalid cron")
	}
	if m7.PipelineLaunchMode() != switchboard.PlScheduleInput() {
		t.Errorf("expected to stay in schedule-input state on error, got mode %d", m7.PipelineLaunchMode())
	}
}

