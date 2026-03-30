package switchboard

import (
	"os"
	"path/filepath"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/adam-stokes/orcai/internal/modal"
	"github.com/adam-stokes/orcai/internal/store"
)

// ── agentRunModelSlug ─────────────────────────────────────────────────────────

func TestAgentRunModelSlug_ReturnsSlug(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("ORCAI_PIPELINES_DIR", dir)

	agentsDir := filepath.Join(dir, ".agents")
	if err := os.MkdirAll(agentsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	content := "name: agent-claude-123\nversion: \"1\"\nsteps:\n  - id: run\n    executor: claude\n    model: claude-sonnet-4-6\n    prompt: |\n      hello\n"
	if err := os.WriteFile(filepath.Join(agentsDir, "agent-claude-123.pipeline.yaml"), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	if got := agentRunModelSlug("agent-claude-123"); got != "claude/claude-sonnet-4-6" {
		t.Errorf("expected claude/claude-sonnet-4-6, got %q", got)
	}
}

func TestAgentRunModelSlug_NoModel_ReturnsExecutorOnly(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("ORCAI_PIPELINES_DIR", dir)

	agentsDir := filepath.Join(dir, ".agents")
	if err := os.MkdirAll(agentsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	content := "name: agent-opencode-456\nversion: \"1\"\nsteps:\n  - id: run\n    executor: opencode\n    prompt: |\n      hello\n"
	if err := os.WriteFile(filepath.Join(agentsDir, "agent-opencode-456.pipeline.yaml"), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	if got := agentRunModelSlug("agent-opencode-456"); got != "opencode" {
		t.Errorf("expected opencode, got %q", got)
	}
}

func TestAgentRunModelSlug_MissingFile_ReturnsEmpty(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("ORCAI_PIPELINES_DIR", dir)

	if got := agentRunModelSlug("nonexistent-run"); got != "" {
		t.Errorf("expected empty string for missing file, got %q", got)
	}
}

func TestAgentRunModelSlug_RegularPipeline_ReturnsEmpty(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("ORCAI_PIPELINES_DIR", dir)

	// Regular pipeline lives in root, not .agents/ — slug helper should return "".
	content := "name: my-pipeline\nversion: \"1\"\nsteps:\n  - id: run\n    executor: shell\n    prompt: hello\n"
	if err := os.WriteFile(filepath.Join(dir, "my-pipeline.pipeline.yaml"), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	if got := agentRunModelSlug("my-pipeline"); got != "" {
		t.Errorf("expected empty string for regular pipeline (not in .agents/), got %q", got)
	}
}

// ── submitRerun path resolution ───────────────────────────────────────────────

// submitRerunFindsAgentsDir verifies that a run whose YAML lives in .agents/
// does not produce a "file not found" feed entry when rerun is confirmed.
func TestSubmitRerun_FindsYAMLInAgentsDir(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("ORCAI_PIPELINES_DIR", dir)

	// Write the YAML into .agents/.
	agentsDir := filepath.Join(dir, ".agents")
	if err := os.MkdirAll(agentsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	content := "name: agent-test-123\nversion: \"1\"\nsteps:\n  - id: run\n    executor: shell\n    prompt: |\n      hello\n"
	if err := os.WriteFile(filepath.Join(agentsDir, "agent-test-123.pipeline.yaml"), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	// Build a run that matches the YAML name (pipeline runner always records kind="pipeline").
	run := store.Run{ID: 1, Kind: "pipeline", Name: "agent-test-123", Steps: []store.StepRecord{}}

	m := New()
	m, _ = m.submitRerun(modal.RerunConfirmedMsg{
		Run:        run,
		ProviderID: "shell",
		ModelID:    "",
		CWD:        dir,
	})

	// If the file was found, no error feed entry should be prepended.
	for _, e := range m.feed {
		if e.status == FeedFailed {
			t.Errorf("unexpected FeedFailed entry: %q", e.title)
		}
	}
}

func TestSubmitRerun_MissingYAML_ProducesFeedError(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("ORCAI_PIPELINES_DIR", dir)

	run := store.Run{ID: 2, Kind: "pipeline", Name: "does-not-exist", Steps: []store.StepRecord{}}

	m := New()
	m, _ = m.submitRerun(modal.RerunConfirmedMsg{
		Run:        run,
		ProviderID: "shell",
		CWD:        dir,
	})

	found := false
	for _, e := range m.feed {
		if e.status == FeedFailed {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected FeedFailed entry for missing pipeline YAML, got none")
	}
}

// ── rerun modal confirms on enter ─────────────────────────────────────────────

func TestRerunModal_EnterFromTextareaZone_Confirms(t *testing.T) {
	// Pressing enter while in the default (textarea) focus zone should confirm,
	// not insert a newline silently.
	m := New()
	run := store.Run{ID: 1, Kind: "pipeline", Name: "test-run", Steps: []store.StepRecord{}}
	m.rerunModal = modal.NewRerunModal(run, nil, "/tmp")
	m.showRerun = true

	// Simulate enter key going through handleKey → rerunModal.Update.
	m2, cmd := m.handleKey(tea.KeyMsg{Type: tea.KeyEnter})
	_ = m2
	if cmd == nil {
		t.Fatal("expected a cmd (confirmedCmd) on enter from textarea focus")
	}
	msg := cmd()
	if _, ok := msg.(modal.RerunConfirmedMsg); !ok {
		t.Errorf("expected RerunConfirmedMsg, got %T", msg)
	}
}
