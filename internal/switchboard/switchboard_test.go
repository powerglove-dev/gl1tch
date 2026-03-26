package switchboard_test

import (
	"context"
	"os"
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

// ── Agent runner form steps ───────────────────────────────────────────────────

func TestAgentFormStepAdvancement(t *testing.T) {
	// We need providers for step advancement. Use a model with mock providers.
	m := switchboard.NewWithTestProviders()

	// Focus agent section first.
	m2, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")})
	m3 := m2.(switchboard.Model)
	if step := m3.AgentFormStep(); step != 0 {
		t.Fatalf("expected step 0 after focusing agent, got %d", step)
	}

	// Tab advances to step 1 (provider has models in test provider).
	m4, _ := m3.Update(tea.KeyMsg{Type: tea.KeyTab})
	m5 := m4.(switchboard.Model)
	if step := m5.AgentFormStep(); step != 1 {
		t.Errorf("expected step 1 after Tab on step 0, got %d", step)
	}

	// Tab again advances to step 2.
	m6, _ := m5.Update(tea.KeyMsg{Type: tea.KeyTab})
	m7 := m6.(switchboard.Model)
	if step := m7.AgentFormStep(); step != 2 {
		t.Errorf("expected step 2 after Tab on step 1, got %d", step)
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
	if !strings.Contains(view, "╔") {
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
	m := switchboard.New()
	m2, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	view := m2.(switchboard.Model).View()
	if !strings.Contains(view, "launch") {
		t.Errorf("View() bottom bar missing 'launch' hint:\n%s", view)
	}
	if !strings.Contains(view, "quit") {
		t.Errorf("View() bottom bar missing 'quit' hint:\n%s", view)
	}
}
