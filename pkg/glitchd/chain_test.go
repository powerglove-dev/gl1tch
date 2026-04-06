package glitchd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestBuildPipelineFromChain_PromptOnly verifies a single prompt step compiles
// to a single executor step with no previous-output ref.
func TestBuildPipelineFromChain_PromptOnly(t *testing.T) {
	steps := []ChainStep{
		{Type: "prompt", Label: "summarize", Body: "Summarize the codebase"},
	}
	opts := RunChainOpts{DefaultProvider: "github-copilot", DefaultModel: "claude-haiku-4-5"}

	p, err := buildPipelineFromChain(steps, opts)
	if err != nil {
		t.Fatalf("buildPipelineFromChain: %v", err)
	}
	if len(p.Steps) != 1 {
		t.Fatalf("want 1 step, got %d", len(p.Steps))
	}
	got := p.Steps[0]
	if got.Executor != "github-copilot" {
		t.Errorf("executor: want github-copilot, got %q", got.Executor)
	}
	if got.Model != "claude-haiku-4-5" {
		t.Errorf("model: want claude-haiku-4-5, got %q", got.Model)
	}
	if !strings.Contains(got.Prompt, "Summarize the codebase") {
		t.Errorf("prompt missing body: %q", got.Prompt)
	}
	if strings.Contains(got.Prompt, "Previous step output") {
		t.Errorf("first step should not reference previous output, got: %q", got.Prompt)
	}
	if _, ok := got.Outputs["value"]; !ok {
		t.Errorf("step should declare outputs.value")
	}
	if !got.NoClarify {
		t.Errorf("chain steps should be NoClarify")
	}
}

// TestBuildPipelineFromChain_TwoPromptsThreaded verifies that the second prompt
// step's output template references the first step's value AND that step-1
// declares step-0 as a dependency (so the DAG runner runs them sequentially).
func TestBuildPipelineFromChain_TwoPromptsThreaded(t *testing.T) {
	steps := []ChainStep{
		{Type: "prompt", Label: "first", Body: "List the apps"},
		{Type: "prompt", Label: "second", Body: "Critique the list"},
	}
	opts := RunChainOpts{DefaultProvider: "ollama", DefaultModel: "llama3.2"}

	p, err := buildPipelineFromChain(steps, opts)
	if err != nil {
		t.Fatalf("buildPipelineFromChain: %v", err)
	}
	if len(p.Steps) != 2 {
		t.Fatalf("want 2 steps, got %d", len(p.Steps))
	}
	if p.Steps[0].ID != "step-0" || p.Steps[1].ID != "step-1" {
		t.Errorf("unexpected step IDs: %q, %q", p.Steps[0].ID, p.Steps[1].ID)
	}
	// Sequential execution: step-1 must depend on step-0.
	if len(p.Steps[0].Needs) != 0 {
		t.Errorf("first step should have no Needs, got %v", p.Steps[0].Needs)
	}
	if len(p.Steps[1].Needs) != 1 || p.Steps[1].Needs[0] != "step-0" {
		t.Errorf("second step Needs: want [step-0], got %v", p.Steps[1].Needs)
	}
	if !strings.Contains(p.Steps[1].Prompt, "{{ steps.step-0.value }}") {
		t.Errorf("second step should reference {{ steps.step-0.value }}, got: %q", p.Steps[1].Prompt)
	}
	if !strings.Contains(p.Steps[1].Prompt, "Critique the list") {
		t.Errorf("second step prompt missing body: %q", p.Steps[1].Prompt)
	}
}

// TestBuildPipelineFromChain_ThreePromptsAllSequential verifies the entire
// chain is linearized via Needs dependencies.
func TestBuildPipelineFromChain_ThreePromptsAllSequential(t *testing.T) {
	steps := []ChainStep{
		{Type: "prompt", Label: "a", Body: "first"},
		{Type: "prompt", Label: "b", Body: "second"},
		{Type: "prompt", Label: "c", Body: "third"},
	}
	p, err := buildPipelineFromChain(steps, RunChainOpts{DefaultProvider: "ollama"})
	if err != nil {
		t.Fatalf("buildPipelineFromChain: %v", err)
	}
	if len(p.Steps) != 3 {
		t.Fatalf("want 3 steps, got %d", len(p.Steps))
	}
	if len(p.Steps[0].Needs) != 0 {
		t.Errorf("step 0 should have no Needs, got %v", p.Steps[0].Needs)
	}
	if len(p.Steps[1].Needs) != 1 || p.Steps[1].Needs[0] != "step-0" {
		t.Errorf("step 1 Needs: want [step-0], got %v", p.Steps[1].Needs)
	}
	if len(p.Steps[2].Needs) != 1 || p.Steps[2].Needs[0] != "step-1" {
		t.Errorf("step 2 Needs: want [step-1], got %v", p.Steps[2].Needs)
	}
}

// TestBuildPipelineFromChain_PerStepOverride verifies that a prompt step's
// executorOverride wins over the default provider.
func TestBuildPipelineFromChain_PerStepOverride(t *testing.T) {
	steps := []ChainStep{
		{Type: "prompt", Label: "default", Body: "step 1"},
		{Type: "prompt", Label: "override", Body: "step 2", ExecutorOverride: "ollama", ModelOverride: "llama3.2"},
	}
	opts := RunChainOpts{DefaultProvider: "github-copilot", DefaultModel: "claude-haiku-4-5"}

	p, err := buildPipelineFromChain(steps, opts)
	if err != nil {
		t.Fatalf("buildPipelineFromChain: %v", err)
	}
	if p.Steps[0].Executor != "github-copilot" {
		t.Errorf("step 0 executor: want github-copilot, got %q", p.Steps[0].Executor)
	}
	if p.Steps[1].Executor != "ollama" {
		t.Errorf("step 1 executor: want ollama (overridden), got %q", p.Steps[1].Executor)
	}
	if p.Steps[1].Model != "llama3.2" {
		t.Errorf("step 1 model: want llama3.2 (overridden), got %q", p.Steps[1].Model)
	}
}

// TestBuildPipelineFromChain_AgentAttachesToNextPrompt verifies an agent step
// is consumed by the following prompt step (BuildAgentPrompt is called).
func TestBuildPipelineFromChain_AgentAttachesToNextPrompt(t *testing.T) {
	// Use a path that doesn't exist; BuildAgentPrompt is tolerant of missing
	// files (returns the user prompt unchanged or with an error message).
	steps := []ChainStep{
		{Type: "agent", Label: "copilot", Invoke: "/nonexistent/agent.md"},
		{Type: "prompt", Label: "task", Body: "do the thing"},
	}
	opts := RunChainOpts{DefaultProvider: "github-copilot"}

	p, err := buildPipelineFromChain(steps, opts)
	if err != nil {
		t.Fatalf("buildPipelineFromChain: %v", err)
	}
	if len(p.Steps) != 1 {
		t.Fatalf("want 1 executable step (agent attaches), got %d", len(p.Steps))
	}
	// The body should still contain the original prompt text.
	if !strings.Contains(p.Steps[0].Prompt, "do the thing") {
		t.Errorf("agent-attached prompt missing body: %q", p.Steps[0].Prompt)
	}
}

// TestBuildPipelineFromChain_NoExecutorErrors verifies that a prompt step with
// no override and no default provider returns a useful error.
func TestBuildPipelineFromChain_NoExecutorErrors(t *testing.T) {
	steps := []ChainStep{
		{Type: "prompt", Label: "orphan", Body: "no provider"},
	}
	opts := RunChainOpts{} // no default

	_, err := buildPipelineFromChain(steps, opts)
	if err == nil {
		t.Fatal("expected error for prompt step with no executor, got nil")
	}
	if !strings.Contains(err.Error(), "no executor") {
		t.Errorf("error should mention 'no executor', got: %v", err)
	}
}

// TestBuildPipelineFromChain_SystemContextOnFirstStepOnly verifies that
// SystemCtx is prepended only to the first executable step.
func TestBuildPipelineFromChain_SystemContextOnFirstStepOnly(t *testing.T) {
	steps := []ChainStep{
		{Type: "prompt", Label: "a", Body: "first"},
		{Type: "prompt", Label: "b", Body: "second"},
	}
	opts := RunChainOpts{
		DefaultProvider: "ollama",
		SystemCtx:       "GLITCH-SYSCTX-MARKER",
	}

	p, err := buildPipelineFromChain(steps, opts)
	if err != nil {
		t.Fatalf("buildPipelineFromChain: %v", err)
	}
	if !strings.Contains(p.Steps[0].Prompt, "GLITCH-SYSCTX-MARKER") {
		t.Errorf("step 0 should contain system context, got: %q", p.Steps[0].Prompt)
	}
	if strings.Contains(p.Steps[1].Prompt, "GLITCH-SYSCTX-MARKER") {
		t.Errorf("step 1 should NOT contain system context, got: %q", p.Steps[1].Prompt)
	}
}

// TestBuildPipelineFromChain_EmptyChainErrors verifies an empty chain is rejected.
func TestBuildPipelineFromChain_EmptyChainErrors(t *testing.T) {
	_, err := buildPipelineFromChain(nil, RunChainOpts{DefaultProvider: "ollama"})
	if err == nil {
		t.Fatal("expected error for empty chain, got nil")
	}
}

// TestResolveLegacyWorkflowPath verifies that saved chain steps with the
// pre-refactor .glitch/pipelines/X.pipeline.yaml layout transparently fall
// back to the current .glitch/workflows/X.workflow.yaml layout.
func TestResolveLegacyWorkflowPath(t *testing.T) {
	repo := t.TempDir()
	workflowsDir := filepath.Join(repo, ".glitch", "workflows")
	if err := os.MkdirAll(workflowsDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	currentPath := filepath.Join(workflowsDir, "audit.workflow.yaml")
	if err := os.WriteFile(currentPath, []byte("name: audit\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	// Legacy path the saved chain JSON would carry.
	legacyPath := filepath.Join(repo, ".glitch", "pipelines", "audit.pipeline.yaml")

	got, err := resolveLegacyWorkflowPath(legacyPath)
	if err != nil {
		t.Fatalf("resolveLegacyWorkflowPath: %v", err)
	}
	if got != currentPath {
		t.Errorf("resolved %q, want %q", got, currentPath)
	}

	// Current path passes through unchanged.
	got, err = resolveLegacyWorkflowPath(currentPath)
	if err != nil {
		t.Fatalf("current path: %v", err)
	}
	if got != currentPath {
		t.Errorf("passthrough returned %q, want %q", got, currentPath)
	}

	// Truly missing path errors.
	if _, err := resolveLegacyWorkflowPath(filepath.Join(repo, "nope.workflow.yaml")); err == nil {
		t.Error("expected error for missing file, got nil")
	}
}

// TestBuildPipelineFromChain_LegacyPipelinePathFallback verifies that a chain
// step embedding a pre-refactor .glitch/pipelines/X.pipeline.yaml path is
// transparently resolved to the new .glitch/workflows/X.workflow.yaml layout
// when buildPipelineFromChain loads the sub-workflow.
func TestBuildPipelineFromChain_LegacyPipelinePathFallback(t *testing.T) {
	repo := t.TempDir()
	workflowsDir := filepath.Join(repo, ".glitch", "workflows")
	if err := os.MkdirAll(workflowsDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	currentPath := filepath.Join(workflowsDir, "audit.workflow.yaml")
	yaml := `name: audit
version: "1"
steps:
  - id: scan
    executor: shell
    outputs: { value: string }
    vars:
      cmd: "echo hello"
`
	if err := os.WriteFile(currentPath, []byte(yaml), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	// Mimic a saved chain JSON from before the refactor — embeds the legacy
	// path that no longer exists on disk.
	legacyPath := filepath.Join(repo, ".glitch", "pipelines", "audit.pipeline.yaml")
	steps := []ChainStep{
		{Type: "pipeline", Label: "audit", Path: legacyPath},
	}

	p, err := buildPipelineFromChain(steps, RunChainOpts{DefaultProvider: "ollama"})
	if err != nil {
		t.Fatalf("buildPipelineFromChain: %v", err)
	}
	if len(p.Steps) == 0 {
		t.Fatal("expected at least one step from the resolved sub-workflow")
	}
	// Sub-workflow steps are prefixed; verify the inner "scan" step made it through.
	found := false
	for _, s := range p.Steps {
		if strings.HasSuffix(s.ID, "scan") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected merged sub-workflow to include a 'scan' step, got %d steps", len(p.Steps))
	}
}

// TestBuildPipelineFromChain_AgentOnlyErrors verifies that an agent without a
// following prompt produces an empty pipeline (which should error).
func TestBuildPipelineFromChain_AgentOnlyErrors(t *testing.T) {
	steps := []ChainStep{
		{Type: "agent", Label: "lonely", Invoke: "/some/path"},
	}
	_, err := buildPipelineFromChain(steps, RunChainOpts{DefaultProvider: "ollama"})
	if err == nil {
		t.Fatal("expected error when chain has only an agent step, got nil")
	}
}

// TestRunChain_RejectsBadJSON verifies the JSON wire format is validated.
func TestRunChain_RejectsBadJSON(t *testing.T) {
	tokenCh := make(chan string, 8)
	err := RunChain(t.Context(), RunChainOpts{
		StepsJSON:       "{not json",
		DefaultProvider: "ollama",
	}, tokenCh)
	if err == nil {
		t.Fatal("expected JSON parse error, got nil")
	}
}

// TestChainStep_RoundTripJSON verifies the wire format matches the frontend
// ChainStep type (camelCase fields).
func TestChainStep_RoundTripJSON(t *testing.T) {
	original := []ChainStep{
		{Type: "prompt", Label: "scan", Body: "Look for issues", ExecutorOverride: "ollama", ModelOverride: "llama3.2"},
		{Type: "agent", Label: "copilot", Invoke: "/path/to/agent.md", Kind: "agent"},
		{Type: "pipeline", Label: "audit", Path: "/repo/.glitch/workflows/audit.workflow.yaml"},
	}
	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	// Verify field names match frontend expectations.
	js := string(data)
	for _, want := range []string{`"type"`, `"label"`, `"body"`, `"executorOverride"`, `"modelOverride"`, `"invoke"`, `"path"`} {
		if !strings.Contains(js, want) {
			t.Errorf("JSON missing expected field %s: %s", want, js)
		}
	}

	var roundTrip []ChainStep
	if err := json.Unmarshal(data, &roundTrip); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(roundTrip) != 3 {
		t.Fatalf("want 3 steps, got %d", len(roundTrip))
	}
	if roundTrip[0].ExecutorOverride != "ollama" {
		t.Errorf("executorOverride round-trip failed")
	}
}
