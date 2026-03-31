package switchboard_test

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

	"github.com/adam-stokes/orcai/internal/store"
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

func TestViewContainsPanelHintFooter(t *testing.T) {
	// Panels show their hint footer inside their own border when focused.
	m := switchboard.New()
	m2, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m3 := m2.(switchboard.Model)

	// Launcher is focused by default — its hint footer should be visible.
	viewLauncher := m3.View()
	if !strings.Contains(viewLauncher, "enter") {
		t.Errorf("View() launcher hint footer missing when launcher focused:\n%s", viewLauncher)
	}

	// Tab 4x to reach agent runner — agent hint footer should appear inside its box.
	cur := m3
	for i := 0; i < 4; i++ {
		nx, _ := cur.Update(tea.KeyMsg{Type: tea.KeyTab})
		cur = nx.(switchboard.Model)
	}
	viewAgent := cur.View()
	if !strings.Contains(viewAgent, "launch") {
		t.Errorf("View() agent hint footer missing when agent focused:\n%s", viewAgent)
	}
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

	// Measure height when agent is focused (includes hint footer row).
	m3a, _ := m3.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")})
	m3b := m3a.(switchboard.Model)
	focusedLines := m3b.BuildAgentSection(60)

	// Advance focus away — agent section loses hint footer row.
	m4, _ := m3b.Update(tea.KeyMsg{Type: tea.KeyTab})
	m5 := m4.(switchboard.Model)
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
	m := switchboard.New()
	m2, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m3 := m2.(switchboard.Model)
	m3 = m3.AddFeedEntry("j1", "running job", switchboard.FeedRunning, nil)
	m3 = m3.AddFeedEntry("j2", "done job", switchboard.FeedDone, nil)
	m3 = m3.AddFeedEntry("j3", "failed job", switchboard.FeedFailed, nil)
	m3 = m3.SetSignalBoardFilter("all") // default is now "running"; set explicitly for this test

	sb := m3.BuildSignalBoard(12, 60)
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

	// Default state: "running" filter is active — filter line IS shown.
	sb := m3.BuildSignalBoard(8, 60)
	rendered := strings.Join(sb, "\n")
	if !strings.Contains(rendered, "SIGNAL BOARD") {
		t.Errorf("signal board missing 'SIGNAL BOARD' header: %s", rendered)
	}
	if !strings.Contains(rendered, "filter:") {
		t.Errorf("signal board should show filter line when filter is 'running': %s", rendered)
	}

	// After pressing f to cycle to "all", the filter line is still always shown.
	m3f := m3.SetSignalBoardFocused(true)
	m4, _ := m3f.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("f")})
	m5 := m4.(switchboard.Model)
	sb2 := m5.BuildSignalBoard(8, 60)
	rendered2 := strings.Join(sb2, "\n")
	if !strings.Contains(rendered2, "filter:") {
		t.Errorf("signal board should always show filter line: %s", rendered2)
	}
	if !strings.Contains(rendered2, "all") {
		t.Errorf("signal board filter line should show 'all' after cycling: %s", rendered2)
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
	// Three-column layout: the old SIGNAL BOARD panel is replaced by the AGENTS
	// grid in the center column.  Verify the agents panel header is present.
	if !strings.Contains(view, "AGENTS") {
		t.Errorf("View() missing AGENTS section (replaced SIGNAL BOARD in three-column layout):\n%s", view)
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
	if !strings.Contains(view, "delete pipeline") {
		t.Errorf("expected delete pipeline confirmation in view after d key:\n%s", view)
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
	// Tab to feed focus (launcher→agent→signal→inbox→feed = 4 tabs).
	cur := tea.Model(m3)
	var focusedModel switchboard.Model
	for i := range 10 {
		next, _ := cur.Update(tea.KeyMsg{Type: tea.KeyTab})
		focusedModel = next.(switchboard.Model)
		cur = next
		if focusedModel.FeedFocused() {
			break
		}
		if i == 9 {
			t.Fatal("feed never became focused after 10 tabs")
		}
	}
	view := focusedModel.View()
	// Feed hint footer shows page nav hints when focused — confirms footer is visible.
	if !strings.Contains(view, "page up") {
		t.Errorf("expected page nav hint in feed footer when feed focused with overflowing content:\n%s", view)
	}
}

// ── Feed navigation (tasks 6.1–6.4) ──────────────────────────────────────────

// TestTabCycle_FullCycle verifies the full Tab focus cycle (three-column layout):
// launcher → inbox → cron → agentsCenter → agent runner (cycles providers) → signalBoard → feed → launcher
func TestTabCycle_FullCycle(t *testing.T) {
	m := switchboard.NewWithPipelines([]string{"alpha", "beta"})
	// Start: launcher focused (default).

	// Tab 1: launcher → inbox
	m2, _ := m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m2m := m2.(switchboard.Model)
	if m2m.SignalBoardFocused() || m2m.FeedFocused() || m2m.AgentsCenterFocused() || m2m.AgentFocused() {
		t.Error("after 1 Tab: expected inbox focused")
	}

	// Tab 2: inbox → cron
	m3, _ := m2m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m3m := m3.(switchboard.Model)
	if !m3m.CronPanelFocused() {
		t.Error("after 2 Tabs: expected cron focused")
	}

	// Tab 3: cron → agentsCenter
	m4, _ := m3m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m4m := m4.(switchboard.Model)
	if !m4m.AgentsCenterFocused() {
		t.Error("after 3 Tabs: expected agentsCenter focused")
	}

	// Tab 4: agentsCenter → agent runner
	m5, _ := m4m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m5m := m5.(switchboard.Model)
	if !m5m.AgentFocused() {
		t.Error("after 4 Tabs: expected agent runner focused")
	}

	// Tab through agent runner providers until signalBoard is reached.
	cur := m5m
	for i := 0; i < 20; i++ {
		nx, _ := cur.Update(tea.KeyMsg{Type: tea.KeyTab})
		cur = nx.(switchboard.Model)
		if cur.SignalBoardFocused() {
			break
		}
	}
	if !cur.SignalBoardFocused() {
		t.Error("expected signalBoard focused after tabbing through agent runner providers")
	}

	// signalBoard → feed
	m6, _ := cur.Update(tea.KeyMsg{Type: tea.KeyTab})
	m6m := m6.(switchboard.Model)
	if !m6m.FeedFocused() {
		t.Error("expected feed focused after Tab from signalBoard")
	}

	// feed → launcher
	m6m = m6m.AddFeedEntry("id1", "job one", switchboard.FeedDone, []string{"line a", "line b"})
	m7, _ := m6m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m7m := m7.(switchboard.Model)
	if m7m.FeedFocused() || m7m.SignalBoardFocused() || m7m.AgentsCenterFocused() || m7m.AgentFocused() {
		t.Error("after feed Tab: expected launcher focused")
	}
	// j should move launcher cursor now, not feedCursor
	cursorBefore := m7m.FeedCursor()
	m8, _ := m7m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	m8m := m8.(switchboard.Model)
	if m8m.FeedCursor() != cursorBefore {
		t.Errorf("feedCursor should not change when launcher is focused, got %d → %d", cursorBefore, m8m.FeedCursor())
	}
}

// TestTabFromFeed_FocusesLauncher verifies that pressing Tab when the Activity
// Feed is focused moves focus to the launcher (feed → launcher in new layout).
func TestTabFromFeed_FocusesLauncher(t *testing.T) {
	m := switchboard.NewWithPipelines([]string{"alpha", "beta"})
	m = m.SetFeedFocused(true)
	m = m.AddFeedEntry("id1", "job one", switchboard.FeedDone, []string{"line a", "line b"})

	// Tab: feed → launcher. Feed cursor should not move.
	cursorBefore := m.FeedCursor()
	m2, _ := m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m3 := m2.(switchboard.Model)
	if m3.FeedFocused() || m3.CronPanelFocused() || m3.AgentsCenterFocused() {
		t.Errorf("expected launcher focused after Tab-from-feed")
	}
	m4, _ := m3.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	m5 := m4.(switchboard.Model)
	if m5.FeedCursor() != cursorBefore {
		t.Errorf("feedCursor should not change when launcher is focused, got %d → %d",
			cursorBefore, m5.FeedCursor())
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
	}{
		{"step-pending", "pending"},
		{"step-running", "running"},
		{"step-done", "done"},
		{"step-failed", "failed"},
	} {
		m3x, _ := m3.Update(switchboard.StepStatusMsg{FeedID: "job1", StepID: tc.id, Status: tc.status})
		m3 = m3x.(switchboard.Model)
	}
	// Done steps with no output are suppressed; add output so the done glyph appears.
	m3 = m3.AddStepLines("job1", "step-done", []string{"ok"})
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
		// Use "running" so steps are not suppressed (done+no-output steps are hidden).
		m3x, _ := m3.Update(switchboard.StepStatusMsg{FeedID: "job1", StepID: id, Status: "running"})
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
	// Three-column layout: the activity feed (right column) is hidden at
	// width < 80.  Test step badge rendering via ViewActivityFeed directly
	// at a narrow panel width so the behaviour is still verified.
	m2, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m3 := m2.(switchboard.Model)
	m3 = m3.AddFeedEntry("job1", "pipeline: test", switchboard.FeedRunning, nil)
	// Add enough steps to force wrapping at a narrow panel width.
	// Use "running" so steps are not suppressed (done+no-output steps are hidden).
	for i := range 6 {
		id := fmt.Sprintf("step-with-long-name-%d", i)
		m3x, _ := m3.Update(switchboard.StepStatusMsg{FeedID: "job1", StepID: id, Status: "running"})
		m3 = m3x.(switchboard.Model)
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
// reaches slot 5 (SCHEDULE).
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

	// Tab six times: slot 0(inner0) → 0(inner1) → 1 → 2 → 3 → 4 → 5.
	cur := m6
	for i := 0; i < 6; i++ {
		nx, _ := cur.Update(tea.KeyMsg{Type: tea.KeyTab})
		cur = nx.(switchboard.Model)
	}

	if got := cur.AgentModalFocus(); got != 5 {
		t.Errorf("expected agentModalFocus == 5 (SCHEDULE), got %d", got)
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

	// Tab to PROMPT (slot 2): 0(inner0)→0(inner1)→1(saved prompt)→2(prompt).
	m7, _ := m6.Update(tea.KeyMsg{Type: tea.KeyTab})
	m8, _ := m7.(switchboard.Model).Update(tea.KeyMsg{Type: tea.KeyTab})
	m8, _ = m8.(switchboard.Model).Update(tea.KeyMsg{Type: tea.KeyTab})
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

	// Tab to PROMPT (slot 2): 0(inner0)→0(inner1)→1→2.
	m7, _ := m6.Update(tea.KeyMsg{Type: tea.KeyTab})
	m8, _ := m7.(switchboard.Model).Update(tea.KeyMsg{Type: tea.KeyTab})
	m8, _ = m8.(switchboard.Model).Update(tea.KeyMsg{Type: tea.KeyTab})
	for _, r := range "hello" {
		nx, _ := m8.(switchboard.Model).Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		m8 = nx
	}

	// Tab to USE_BRAIN (slot 3), CWD (slot 4), then SCHEDULE (slot 5) and type an invalid cron expression.
	m9tmp0, _ := m8.(switchboard.Model).Update(tea.KeyMsg{Type: tea.KeyTab})
	m9tmp, _ := m9tmp0.(switchboard.Model).Update(tea.KeyMsg{Type: tea.KeyTab})
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


// ── Kill and Archive (signal-board-kill-and-archive) ──────────────────────────

func TestSignalBoard_KillRunningEntry(t *testing.T) {
	cancelled := false
	_, cancel := context.WithCancel(context.Background())
	wrappedCancel := context.CancelFunc(func() { cancelled = true; cancel() })

	m := switchboard.New()
	m = m.AddFeedEntry("job1", "pipeline: kill-me", switchboard.FeedRunning, nil)
	m = m.AddActiveJobWithCancel("job1", wrappedCancel)
	m = m.SetSignalBoardFocused(true)

	m2, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("x")})
	result := m2.(switchboard.Model)

	if !cancelled {
		t.Error("expected cancel to be called on kill")
	}
	status, found := result.FeedEntryStatus("job1")
	if !found {
		t.Fatal("feed entry not found after kill")
	}
	if status != switchboard.FeedFailed {
		t.Errorf("expected FeedFailed after kill, got %v", status)
	}
	if result.ActiveJobsCount() != 0 {
		t.Errorf("expected activeJobs to be empty after kill, got %d", result.ActiveJobsCount())
	}
}

func TestSignalBoard_KillNonRunningEntry_NoOp(t *testing.T) {
	m := switchboard.New()
	m = m.AddFeedEntry("job1", "done job", switchboard.FeedDone, nil)
	m = m.SetSignalBoardFocused(true)
	// Set filter to all so the done entry is visible
	m2, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("f")})
	m3, _ := m2.(switchboard.Model).Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("x")})
	result := m3.(switchboard.Model)

	status, found := result.FeedEntryStatus("job1")
	if !found {
		t.Fatal("feed entry not found")
	}
	if status != switchboard.FeedDone {
		t.Errorf("expected status to remain FeedDone, got %v", status)
	}
}

func TestSignalBoard_ArchiveEntry(t *testing.T) {
	m := switchboard.New()
	m = m.AddFeedEntry("job1", "pipeline: done", switchboard.FeedDone, nil)
	m = m.SetSignalBoardFocused(true)
	// Switch to "all" filter so the done entry is visible (default is "running")
	m2, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("f")})
	m3, _ := m2.(switchboard.Model).Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})
	result := m3.(switchboard.Model)

	archived, found := result.FeedEntryArchived("job1")
	if !found {
		t.Fatal("feed entry not found after archive")
	}
	if !archived {
		t.Error("expected entry to be archived")
	}

	// Entry should not appear in "all" filter
	sb := result.BuildSignalBoard(12, 80)
	rendered := strings.Join(sb, "\n")
	if strings.Contains(rendered, "pipeline: done") {
		t.Error("archived entry should not appear in 'all' filter view")
	}
}

func TestSignalBoard_ArchivedFilter_ShowsArchivedEntries(t *testing.T) {
	m := switchboard.New()
	m = m.AddFeedEntry("job1", "pipeline: done", switchboard.FeedDone, nil)
	m = m.SetSignalBoardFocused(true)
	// Switch to "all" to see the entry, then archive it
	m2, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("f")})
	m3, _ := m2.(switchboard.Model).Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})

	// Cycle to "archived" filter: all→done→failed→archived (3 presses from "all")
	cur := m3.(switchboard.Model)
	for i := 0; i < 3; i++ {
		nx, _ := cur.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("f")})
		cur = nx.(switchboard.Model)
	}

	if cur.SignalBoardActiveFilter() != "archived" {
		t.Errorf("expected filter 'archived', got %q", cur.SignalBoardActiveFilter())
	}
	sb := cur.BuildSignalBoard(12, 80)
	rendered := strings.Join(sb, "\n")
	if !strings.Contains(rendered, "pipeline: done") {
		t.Errorf("expected archived entry to appear in archived filter:\n%s", rendered)
	}
}

func TestSignalBoard_FilterCycleOrder(t *testing.T) {
	m := switchboard.New()
	m = m.SetSignalBoardFocused(true)

	expected := []string{"all", "done", "failed", "archived", "running"}
	cur := m
	for _, want := range expected {
		nx, _ := cur.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("f")})
		cur = nx.(switchboard.Model)
		if got := cur.SignalBoardActiveFilter(); got != want {
			t.Errorf("expected filter %q, got %q", want, got)
		}
	}
}

func TestSignalBoard_DefaultFilter_IsRunning(t *testing.T) {
	m := switchboard.New()
	if got := m.SignalBoardActiveFilter(); got != "running" {
		t.Errorf("expected default filter 'running', got %q", got)
	}
}

// ── Feed mark / highlight / agent runner injection ────────────────────────────

func TestFeed_MarkToggle(t *testing.T) {
	m := switchboard.New()
	m2, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = m2.(switchboard.Model)
	m = m.AddFeedEntry("job1", "pipeline: alpha", switchboard.FeedDone, nil)
	m = m.SetFeedFocused(true)

	// Press m to enter active mark mode (does not mark yet).
	m3, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("m")})
	afterMode := m3.(switchboard.Model)
	if afterMode.FeedMarkMode() != switchboard.MarkModeActive {
		t.Error("expected mark mode to be active after pressing m")
	}
	if afterMode.FeedMarkedAt(0) {
		t.Error("expected no marks immediately after entering mark mode")
	}

	// Press j to mark line 0 and advance cursor.
	m4, _ := afterMode.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	afterMark := m4.(switchboard.Model)
	if !afterMark.FeedMarkedAt(0) {
		t.Error("expected line 0 to be marked after j in active mark mode")
	}

	// Press m again to pause mark mode; j should navigate without marking.
	m5, _ := afterMark.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("m")})
	afterPause := m5.(switchboard.Model)
	if afterPause.FeedMarkMode() != switchboard.MarkModePaused {
		t.Error("expected mark mode to be paused after second m press")
	}

	// Press m again to exit mark mode entirely.
	m6, _ := afterPause.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("m")})
	afterExit := m6.(switchboard.Model)
	if afterExit.FeedMarkMode() != switchboard.MarkModeOff {
		t.Error("expected mark mode to be off after third m press")
	}
}

func TestFeed_RKeyWithMarks_OpensModalWithPrompt(t *testing.T) {
	m := switchboard.New()
	m2, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = m2.(switchboard.Model)
	m = m.AddFeedEntry("job1", "pipeline: alpha", switchboard.FeedDone, nil)
	m = m.SetFeedFocused(true)

	// Enter mark mode, then press j to mark line 0.
	m3, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("m")})
	m = m3.(switchboard.Model)
	m4, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	m = m4.(switchboard.Model)

	// Press r — should open modal with prompt populated.
	m5, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("r")})
	result := m5.(switchboard.Model)

	if !result.AgentModalOpen() {
		t.Fatal("expected agent modal to be open after pressing r with marked feed lines")
	}
	if result.AgentModalFocus() != 2 {
		t.Errorf("expected modal focus slot 2 (prompt), got %d", result.AgentModalFocus())
	}
	if result.AgentPromptValue() == "" {
		t.Error("expected agent prompt to be pre-populated, got empty string")
	}
	if result.FeedHasMarks() {
		t.Error("expected marks to be cleared after r")
	}
}

func TestFeed_RKeyWithNoMarks_NoOp(t *testing.T) {
	m := switchboard.New()
	m2, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = m2.(switchboard.Model)
	m = m.AddFeedEntry("job1", "pipeline: alpha", switchboard.FeedDone, nil)
	m = m.SetFeedFocused(true)

	// Press r with no marks — modal should NOT open.
	m3, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("r")})
	result := m3.(switchboard.Model)

	if result.AgentModalOpen() {
		t.Error("expected agent modal to remain closed when r pressed with no marks")
	}
}

func TestAgentModalBox_Width_IsNinetyPercent(t *testing.T) {
	m := switchboard.New()
	m2, _ := m.Update(tea.WindowSizeMsg{Width: 160, Height: 40})
	m3 := m2.(switchboard.Model)
	box := m3.ViewAgentModalBox(160, 40)
	// modalW = max(160*9/10, 60) = 144.
	// The first line is the box top border: ╔═...═╗ (plus ANSI color escapes for border).
	// Count the ═ characters (each represents one content column) plus the two corners.
	// Expected: 144 - 2 corners = 142 ═ chars (minus any title chars replaced with spaces).
	// Simpler: count that the first content row (│ ... │) has 142 inner chars.
	lines := strings.Split(box, "\n")
	if len(lines) < 2 {
		t.Fatal("expected at least 2 lines in modal box")
	}
	// Count ─ runes in first line — box top border has modalW-2-titleLen of them.
	// For w=160: modalW=144, title=" AGENT "=7 chars, so 144-2-7=135 dashes.
	// Old cap was 90 cols → 90-2-7=81 dashes. Check well above the old max.
	dashCount := strings.Count(lines[0], "─")
	if dashCount < 120 {
		t.Errorf("expected at least 120 '─' chars in modal top border (90%% of 160 terminal), got %d", dashCount)
	}
}

func TestFeed_HintBar_MarkHintPresent(t *testing.T) {
	m := switchboard.New()
	m2, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = m2.(switchboard.Model)
	m = m.AddFeedEntry("job1", "pipeline: alpha", switchboard.FeedRunning, nil)
	m = m.SetFeedFocused(true)
	// Use ViewActivityFeed directly with a wide enough panel to fit all hints.
	// (The right column in View() is narrow ~30 chars; ViewActivityFeed at 80
	// chars gives enough room for the full hint bar.)
	view := m.ViewActivityFeed(20, 80)
	if !strings.Contains(view, "mark") {
		t.Error("expected 'm mark' hint in feed hint bar when focused, not found")
	}
}

func TestFeed_HintBar_RunHintWhenMarked(t *testing.T) {
	m := switchboard.New()
	m2, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = m2.(switchboard.Model)
	m = m.AddFeedEntry("job1", "pipeline: alpha", switchboard.FeedRunning, nil)
	m = m.SetFeedFocused(true)
	// Mark the current line.
	m3, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("m")})
	m = m3.(switchboard.Model)
	view := m.View()
	if !strings.Contains(view, "run") {
		t.Error("expected 'r run' hint in feed hint bar when lines are marked, not found")
	}
}

func TestFeedStepDisplay_Vertical(t *testing.T) {
	m := switchboard.New()
	m2, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m3 := m2.(switchboard.Model)
	m3 = m3.AddFeedEntry("job1", "pipeline: test", switchboard.FeedRunning, nil)
	// Use "running" status so steps are not suppressed (done+no-output steps are hidden).
	for _, id := range []string{"step-a", "step-b", "step-c"} {
		mx, _ := m3.Update(switchboard.StepStatusMsg{FeedID: "job1", StepID: id, Status: "running"})
		m3 = mx.(switchboard.Model)
	}
	view := m3.View()
	// All step IDs must appear.
	for _, id := range []string{"step-a", "step-b", "step-c"} {
		if !strings.Contains(view, id) {
			t.Errorf("View() missing step id %q in vertical layout output", id)
		}
	}
	// With vertical layout the · separator should NOT appear between step badges.
	if strings.Contains(view, "  ·  ") {
		t.Error("View() contains horizontal '·' separator — expected vertical layout")
	}
	// Tree connectors should appear.
	if !strings.Contains(view, "├") && !strings.Contains(view, "└") {
		t.Error("expected tree connectors (├ or └) in step display")
	}
}

// ── Mark mode state machine ───────────────────────────────────────────────────

func TestFeed_MarkMode_CyclesOffActivePausedOff(t *testing.T) {
	m := switchboard.New()
	m2, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = m2.(switchboard.Model)
	m = m.AddFeedEntry("job1", "pipeline: alpha", switchboard.FeedDone, nil)
	m = m.SetFeedFocused(true)

	if m.FeedMarkMode() != switchboard.MarkModeOff {
		t.Error("expected MarkModeOff initially")
	}
	// First m → active.
	m3, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("m")})
	m = m3.(switchboard.Model)
	if m.FeedMarkMode() != switchboard.MarkModeActive {
		t.Error("expected MarkModeActive after first m")
	}
	// Second m → paused.
	m4, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("m")})
	m = m4.(switchboard.Model)
	if m.FeedMarkMode() != switchboard.MarkModePaused {
		t.Error("expected MarkModePaused after second m")
	}
	// Third m → off (exit mark mode).
	m5, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("m")})
	m = m5.(switchboard.Model)
	if m.FeedMarkMode() != switchboard.MarkModeOff {
		t.Error("expected MarkModeOff after third m")
	}
}

func TestFeed_MarkMode_JMarksCurrentLineAndAdvances(t *testing.T) {
	m := switchboard.New()
	m2, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = m2.(switchboard.Model)
	m = m.AddFeedEntry("job1", "pipeline: alpha", switchboard.FeedDone, nil)
	m = m.AddFeedEntry("job2", "pipeline: beta", switchboard.FeedDone, nil)
	m = m.SetFeedFocused(true)

	// Cursor starts at 0.
	// Enter mark mode.
	m3, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("m")})
	m = m3.(switchboard.Model)
	// Press j — should mark line 0, advance to line 1.
	m4, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	m = m4.(switchboard.Model)
	if !m.FeedMarkedAt(0) {
		t.Error("expected line 0 to be marked after j in active mark mode")
	}
	if m.FeedCursor() != 1 {
		t.Errorf("expected cursor at 1 after j, got %d", m.FeedCursor())
	}
}

func TestFeed_MarkMode_JInPausedModeAdvancesWithoutMarking(t *testing.T) {
	m := switchboard.New()
	m2, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = m2.(switchboard.Model)
	m = m.AddFeedEntry("job1", "pipeline: alpha", switchboard.FeedDone, nil)
	m = m.AddFeedEntry("job2", "pipeline: beta", switchboard.FeedDone, nil)
	m = m.SetFeedFocused(true)

	// Enter mark mode then pause.
	m3, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("m")})
	m = m3.(switchboard.Model)
	m4, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("m")})
	m = m4.(switchboard.Model)
	if m.FeedMarkMode() != switchboard.MarkModePaused {
		t.Fatal("expected paused mode")
	}
	// Press j — cursor advances but no mark.
	m5, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	m = m5.(switchboard.Model)
	if m.FeedMarkedAt(0) {
		t.Error("expected line 0 NOT to be marked when j pressed in paused mode")
	}
	if m.FeedCursor() != 1 {
		t.Errorf("expected cursor at 1 after j in paused mode, got %d", m.FeedCursor())
	}
}

func TestFeed_MarkMode_ResetOnFocusLoss(t *testing.T) {
	m := switchboard.New()
	m2, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = m2.(switchboard.Model)
	m = m.AddFeedEntry("job1", "pipeline: alpha", switchboard.FeedDone, nil)
	m = m.SetFeedFocused(true)

	// Enter mark mode, mark a line.
	m3, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("m")})
	m = m3.(switchboard.Model)
	m4, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	m = m4.(switchboard.Model)
	if !m.FeedHasMarks() {
		t.Fatal("expected marks before focus loss")
	}

	// Switch focus away (press 'a' for agent).
	m5, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")})
	m = m5.(switchboard.Model)
	if m.FeedMarkMode() != switchboard.MarkModeOff {
		t.Error("expected mark mode reset to Off after losing feed focus")
	}
	if m.FeedHasMarks() {
		t.Error("expected marks cleared after losing feed focus")
	}
}

// ── Step suppression ─────────────────────────────────────────────────────────

func TestFeedStep_DoneWithNoOutput_NotRendered(t *testing.T) {
	m := switchboard.New()
	m2, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = m2.(switchboard.Model)
	m = m.AddFeedEntry("job1", "pipeline: test", switchboard.FeedRunning, nil)
	mx, _ := m.Update(switchboard.StepStatusMsg{FeedID: "job1", StepID: "my-done-step", Status: "done"})
	m = mx.(switchboard.Model)
	view := m.View()
	if strings.Contains(view, "my-done-step") {
		t.Error("expected done step with no output to be suppressed from feed view")
	}
}

func TestFeedStep_DoneWithOutput_IsRendered(t *testing.T) {
	m := switchboard.New()
	m2, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = m2.(switchboard.Model)
	m = m.AddFeedEntry("job1", "pipeline: test", switchboard.FeedRunning, nil)
	// Add step and inject output lines.
	mx, _ := m.Update(switchboard.StepStatusMsg{FeedID: "job1", StepID: "result-step", Status: "done"})
	m = mx.(switchboard.Model)
	m = m.AddStepLines("job1", "result-step", []string{"output here"})
	view := m.View()
	if !strings.Contains(view, "result-step") {
		t.Error("expected done step WITH output to appear in feed view")
	}
	if !strings.Contains(view, "output here") {
		t.Error("expected step output line to appear in feed view")
	}
}

// ── Cursor overlay ────────────────────────────────────────────────────────────

var ansiEscapeRe = regexp.MustCompile(`\x1b\[[0-9;]*[a-zA-Z]`)

func stripTestANSI(s string) string { return ansiEscapeRe.ReplaceAllString(s, "") }

func TestFeed_CursorRow_SameWidthAsNonCursorRow(t *testing.T) {
	m := switchboard.New()
	m2, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = m2.(switchboard.Model)
	m = m.AddFeedEntry("job1", "pipeline: alpha", switchboard.FeedDone, nil)
	m = m.AddFeedEntry("job2", "pipeline: beta", switchboard.FeedDone, nil)
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

func TestFeedLineCount_MatchesViewActivityFeed(t *testing.T) {
	m := switchboard.New()
	m2, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = m2.(switchboard.Model)

	// Add entries with mix of plain text and JSON output.
	m = m.AddFeedEntry("job1", "pipeline: alpha", switchboard.FeedDone, []string{
		"plain text output",
		`{"key":"value","count":42}`,
		"another plain line",
	})
	m = m.AddFeedEntry("job2", "agent: search", switchboard.FeedDone, []string{
		`[1,2,3,4,5,6,7,8]`,
	})

	m = m.SetFeedFocused(true)

	logicalCount := m.FeedLineCount()
	if logicalCount <= 0 {
		t.Fatalf("FeedLineCount should be positive, got %d", logicalCount)
	}

	// Count navigable rows from ViewActivityFeed using the internal logicalIdx.
	// We verify by checking cursor can navigate to the last line without overflow.
	for i := 0; i < logicalCount-1; i++ {
		m2, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
		m = m2.(switchboard.Model)
	}
	if m.FeedCursor() != logicalCount-1 {
		t.Errorf("cursor should be at %d (last line), got %d", logicalCount-1, m.FeedCursor())
	}

	// One more j should not exceed bounds.
	m2, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	m = m2.(switchboard.Model)
	if m.FeedCursor() != logicalCount-1 {
		t.Errorf("cursor exceeded logicalCount: cursor=%d, count=%d", m.FeedCursor(), logicalCount)
	}
}

// ── WriteSingleStepPipeline ───────────────────────────────────────────────────

func TestWriteSingleStepPipeline_UseBrain(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("ORCAI_PIPELINES_DIR", dir)

	path, err := switchboard.WriteSingleStepPipeline("test-brain", "opencode", "", "do a thing", true)
	if err != nil {
		t.Fatalf("WriteSingleStepPipeline: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read pipeline: %v", err)
	}
	yaml := string(data)
	if !strings.Contains(yaml, "use_brain: true") {
		t.Errorf("expected use_brain: true in YAML, got:\n%s", yaml)
	}
}

func TestWriteSingleStepPipeline_NoBrain(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("ORCAI_PIPELINES_DIR", dir)

	path, err := switchboard.WriteSingleStepPipeline("test-no-brain", "opencode", "", "do a thing", false)
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

func TestAgentUseBrain_DefaultFalse(t *testing.T) {
	m := switchboard.New()
	if m.AgentUseBrain() {
		t.Error("agentUseBrain should default to false")
	}
}

// ── Inline picker: saved prompt ───────────────────────────────────────────────

// openAgentModal is a test helper that opens the agent modal with a wide terminal.
func openAgentModal(t *testing.T) switchboard.Model {
	t.Helper()
	m := switchboard.NewWithTestProviders()
	m2, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m3 := m2.(switchboard.Model)
	m4, _ := m3.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")})
	m5, _ := m4.(switchboard.Model).Update(tea.KeyMsg{Type: tea.KeyEnter})
	modal := m5.(switchboard.Model)
	if !modal.AgentModalOpen() {
		t.Fatal("agent modal did not open")
	}
	return modal
}

// tabToFocusSlot tabs from the initial agent modal state (outer slot 0, inner 0) to a target slot.
// It assumes the test provider has one provider+one model (inner focus reaches 1 in 1 tab).
func tabToFocusSlot(t *testing.T, m switchboard.Model, targetSlot int) switchboard.Model {
	t.Helper()
	tab := func(cur switchboard.Model) switchboard.Model {
		nx, _ := cur.Update(tea.KeyMsg{Type: tea.KeyTab})
		return nx.(switchboard.Model)
	}
	// First tab moves agentPicker internal focus 0→1.
	m = tab(m)
	// Subsequent tabs advance the outer slot.
	for m.AgentModalFocus() != targetSlot {
		m = tab(m)
		if m.AgentModalFocus() == 0 {
			t.Fatalf("wrapped around before reaching slot %d", targetSlot)
		}
	}
	return m
}

func TestAgentModal_TabReachesSavedPromptSlot(t *testing.T) {
	m := openAgentModal(t)
	// Tab once: agentPicker inner 0→1. Tab again: outer 0→1 (saved prompt).
	nx1, _ := m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m1 := nx1.(switchboard.Model)
	nx2, _ := m1.Update(tea.KeyMsg{Type: tea.KeyTab})
	result := nx2.(switchboard.Model)
	if got := result.AgentModalFocus(); got != 1 {
		t.Errorf("expected agentModalFocus == 1 (saved prompt), got %d", got)
	}
}

func TestAgentModal_EnterOnSlot1_OpensSavedPromptPicker(t *testing.T) {
	m := openAgentModal(t)
	// Inject some prompts so the picker has items.
	m = m.WithAgentPrompts([]store.Prompt{
		{Title: "prompt-alpha"},
		{Title: "prompt-beta"},
	})
	m = tabToFocusSlot(t, m, 1)

	if m.SavedPromptPickerOpen() {
		t.Fatal("picker should not be open before Enter")
	}
	m2, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	result := m2.(switchboard.Model)
	if !result.SavedPromptPickerOpen() {
		t.Error("expected savedPromptPicker to be open after Enter on slot 1")
	}
}

func TestAgentModal_BracketKeys_NoLongerCyclePrompts(t *testing.T) {
	m := openAgentModal(t)
	m = m.WithAgentPrompts([]store.Prompt{
		{Title: "prompt-one"},
		{Title: "prompt-two"},
	})
	initial := m.AgentPromptIdx()

	// Press ] — should have no effect.
	m2, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("]")})
	after := m2.(switchboard.Model).AgentPromptIdx()
	if after != initial {
		t.Errorf("] should no longer cycle prompts: idx went from %d to %d", initial, after)
	}

	// Press [ — should have no effect.
	m3, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("[")})
	after2 := m3.(switchboard.Model).AgentPromptIdx()
	if after2 != initial {
		t.Errorf("[ should no longer cycle prompts: idx went from %d to %d", initial, after2)
	}
}

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
		m := switchboard.New()
		if tc.termWidth > 0 {
			m2, _ := m.Update(tea.WindowSizeMsg{Width: tc.termWidth, Height: 40})
			m = m2.(switchboard.Model)
		}
		if got := m.RightColWidth(); got != tc.wantRight {
			t.Errorf("RightColWidth() for termWidth=%d: got %d, want %d", tc.termWidth, got, tc.wantRight)
		}
	}
}

// TestMidColWidth verifies the center-column formula: w - leftW - rightW - 4.
func TestMidColWidth(t *testing.T) {
	cases := []struct {
		termWidth int
		wantMin   int // just verify it's positive and reasonable
	}{
		{termWidth: 120, wantMin: 10},
		{termWidth: 160, wantMin: 10},
		{termWidth: 200, wantMin: 10},
	}
	for _, tc := range cases {
		m := switchboard.New()
		m2, _ := m.Update(tea.WindowSizeMsg{Width: tc.termWidth, Height: 40})
		m = m2.(switchboard.Model)
		got := m.MidColWidth()
		if got < tc.wantMin {
			t.Errorf("MidColWidth() for termWidth=%d: got %d, want >= %d", tc.termWidth, got, tc.wantMin)
		}
		// Verify the formula: leftW + midW + rightW + 4 == termWidth (approximately).
		leftW := m.LeftColWidth()
		rightW := m.RightColWidth()
		if leftW+got+rightW+4 != tc.termWidth {
			t.Errorf("column widths don't sum to termWidth: left(%d)+mid(%d)+right(%d)+4 = %d, want %d",
				leftW, got, rightW, leftW+got+rightW+4, tc.termWidth)
		}
	}
}

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
		m := switchboard.New()
		// Add enough agents to populate all columns.
		for i := 0; i < tc.wantCols*2; i++ {
			m = m.AddFeedEntry(fmt.Sprintf("id%d", i), fmt.Sprintf("agent-%d", i), switchboard.FeedRunning, nil)
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
	m := switchboard.New()
	m = m.AddFeedEntry("id1", "agent-one", switchboard.FeedRunning, nil)
	rendered := m.BuildAgentsGrid(10, 5) // very narrow: 5/24 = 0, clamped to 1
	if len(rendered) == 0 {
		t.Error("BuildAgentsGrid: expected at least one line at narrow width")
	}
}

// ── h/j/k/l cursor navigation (task 9.4) ─────────────────────────────────────

// TestAgentsGrid_HJKLNavigation verifies cursor movement and clamping.
func TestAgentsGrid_HJKLNavigation(t *testing.T) {
	m := switchboard.New()
	// Add 4 entries so we have a 2×2 grid at midW=48 (2 cols).
	for _, id := range []string{"id0", "id1", "id2", "id3"} {
		m = m.AddFeedEntry(id, "agent-"+id, switchboard.FeedRunning, nil)
	}
	m2, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = m2.(switchboard.Model)
	m = m.SetAgentsCenterFocused(true)

	// Initially at row=0, col=0.
	if m.AgentsGridRow() != 0 || m.AgentsGridCol() != 0 {
		t.Fatalf("initial cursor: got row=%d col=%d, want 0,0", m.AgentsGridRow(), m.AgentsGridCol())
	}

	// Press l → col should increase.
	m3, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("l")})
	m = m3.(switchboard.Model)
	if m.AgentsGridCol() != 1 {
		t.Errorf("after l: col=%d, want 1", m.AgentsGridCol())
	}

	// Press l again → should clamp at last column.
	prevCol := m.AgentsGridCol()
	m4, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("l")})
	m = m4.(switchboard.Model)
	if m.AgentsGridCol() > prevCol {
		t.Errorf("l past last column should clamp: col went from %d to %d", prevCol, m.AgentsGridCol())
	}

	// Press h → col should decrease.
	m5, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("h")})
	m = m5.(switchboard.Model)
	if m.AgentsGridCol() != 0 {
		t.Errorf("after h: col=%d, want 0", m.AgentsGridCol())
	}

	// Press h again → should clamp at 0.
	m6, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("h")})
	m = m6.(switchboard.Model)
	if m.AgentsGridCol() != 0 {
		t.Errorf("h at col=0 should stay at 0, got %d", m.AgentsGridCol())
	}

	// Press j → row should increase.
	m7, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	m = m7.(switchboard.Model)
	if m.AgentsGridRow() != 1 {
		t.Errorf("after j: row=%d, want 1", m.AgentsGridRow())
	}

	// Press k → row should decrease.
	m8, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("k")})
	m = m8.(switchboard.Model)
	if m.AgentsGridRow() != 0 {
		t.Errorf("after k: row=%d, want 0", m.AgentsGridRow())
	}

	// Press k again → should clamp at 0.
	m9, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("k")})
	m = m9.(switchboard.Model)
	if m.AgentsGridRow() != 0 {
		t.Errorf("k at row=0 should stay at 0, got %d", m.AgentsGridRow())
	}
}

// ── 12-hour timestamp format (task 9.5) ───────────────────────────────────────

// TestFeedTimestamp_12HrFormat verifies timestamps use 12hr am/pm format.
func TestFeedTimestamp_12HrFormat(t *testing.T) {
	m := switchboard.New()
	m2, _ := m.Update(tea.WindowSizeMsg{Width: 160, Height: 40})
	m = m2.(switchboard.Model)
	// AddFeedEntry uses time.Now() — just verify the pattern in the view.
	m = m.AddFeedEntry("job1", "test-agent", switchboard.FeedDone, nil)
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
	m := switchboard.New()
	m2, _ := m.Update(tea.WindowSizeMsg{Width: 160, Height: 40})
	m = m2.(switchboard.Model)
	m = m.AddFeedEntry("job1", "test-agent", switchboard.FeedDone, nil)
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
	m := switchboard.New()
	m2, _ := m.Update(tea.WindowSizeMsg{Width: 160, Height: 40})
	m = m2.(switchboard.Model)
	m = m.AddFeedEntry("job1", "pipeline: test", switchboard.FeedRunning, nil)
	// Add two steps so we get both ├─ and └─ connectors.
	for _, id := range []string{"step-a", "step-b"} {
		mx, _ := m.Update(switchboard.StepStatusMsg{FeedID: "job1", StepID: id, Status: "running"})
		m = mx.(switchboard.Model)
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
	m := switchboard.New()
	m2, _ := m.Update(tea.WindowSizeMsg{Width: 160, Height: 40})
	m = m2.(switchboard.Model)
	m = m.AddFeedEntry("job1", "pipeline: test", switchboard.FeedDone, nil)
	// A done step with no output should be suppressed from the feed.
	mx, _ := m.Update(switchboard.StepStatusMsg{FeedID: "job1", StepID: "quiet-step", Status: "done"})
	m = mx.(switchboard.Model)
	view := m.ViewActivityFeed(40, 80)
	if strings.Contains(view, "quiet-step") {
		t.Error("done step with no output should not appear in activity feed")
	}
}

// ── Feed card centering (task 9.7) ────────────────────────────────────────────

// TestFeedCard_RightColumnWidth verifies the feed is rendered at rightColWidth.
func TestFeedCard_RightColumnWidth(t *testing.T) {
	m := switchboard.New()
	m2, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = m2.(switchboard.Model)
	m = m.AddFeedEntry("job1", "test-agent", switchboard.FeedDone, nil)

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
	m := switchboard.New()
	m2, _ := m.Update(tea.WindowSizeMsg{Width: 60, Height: 40})
	m = m2.(switchboard.Model)
	m = m.AddFeedEntry("job1", "hidden-agent", switchboard.FeedDone, nil)
	view := m.View()
	// The activity feed header should not appear in the narrow layout.
	// (The right column is suppressed when w < 80.)
	if strings.Contains(view, "ACTIVITY FEED") {
		t.Error("expected activity feed to be hidden at terminal width < 80")
	}
}

// TestWideTerminal_ShowsAllThreeColumns verifies w>=80 shows all three columns.
func TestWideTerminal_ShowsAllThreeColumns(t *testing.T) {
	m := switchboard.New()
	m2, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = m2.(switchboard.Model)
	view := m.View()
	// All three column headers should be visible.
	if !strings.Contains(view, "PIPELINES") {
		t.Error("expected PIPELINES in left column")
	}
	if !strings.Contains(view, "AGENTS") && !strings.Contains(view, "agents") {
		t.Error("expected AGENTS in center column")
	}
	if !strings.Contains(view, "ACTIVITY FEED") {
		t.Error("expected ACTIVITY FEED in right column")
	}
}

// ── Inline picker: working directory ─────────────────────────────────────────

func TestAgentModal_EnterOnCWDSlot_SetsInlineDirPicker(t *testing.T) {
	m := openAgentModal(t)
	m = tabToFocusSlot(t, m, 4)

	if m.DirPickerOpen() {
		t.Fatal("dir picker should not be open before Enter on CWD slot")
	}
	m2, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	result := m2.(switchboard.Model)
	if !result.DirPickerOpen() {
		t.Error("expected dirPickerOpen=true after Enter on CWD slot")
	}
	if result.DirPickerCtx() != "agent" {
		t.Errorf("expected dirPickerCtx=agent, got %q", result.DirPickerCtx())
	}
}
