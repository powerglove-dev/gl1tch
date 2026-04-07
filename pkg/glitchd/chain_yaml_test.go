package glitchd

import (
	"strings"
	"testing"

	"github.com/8op-org/gl1tch/internal/pipeline"
)

// TestChainStepsToYAML_RoundTrip is the load-bearing test for the YAML
// unification: anything we serialize must be loadable by pipeline.Load
// without losing the executor/model/prompt the runner needs.
func TestChainStepsToYAML_RoundTrip(t *testing.T) {
	stepsJSON := `[
		{"type":"prompt","label":"first","body":"summarize the repo"},
		{"type":"prompt","label":"second","body":"now write tests"}
	]`

	yamlStr, err := ChainStepsToYAML(stepsJSON, "audit", "two-step audit", "ollama", "llama3.2")
	if err != nil {
		t.Fatalf("ChainStepsToYAML: %v", err)
	}
	if yamlStr == "" {
		t.Fatal("got empty YAML")
	}

	pl, err := pipeline.Load(strings.NewReader(yamlStr))
	if err != nil {
		t.Fatalf("pipeline.Load: %v\nyaml:\n%s", err, yamlStr)
	}
	if pl.Name != "audit" {
		t.Errorf("name = %q, want audit", pl.Name)
	}
	if len(pl.Steps) != 2 {
		t.Fatalf("got %d steps, want 2", len(pl.Steps))
	}
	for i, s := range pl.Steps {
		if s.Executor != "ollama" {
			t.Errorf("step %d: executor = %q, want ollama", i, s.Executor)
		}
		if s.Model != "llama3.2" {
			t.Errorf("step %d: model = %q, want llama3.2", i, s.Model)
		}
		if s.Prompt == "" {
			t.Errorf("step %d: empty prompt", i)
		}
	}
	// Second step must depend on first so the runner doesn't fan them
	// out in parallel and lose the previous-step-output threading.
	if len(pl.Steps[1].Needs) == 0 || pl.Steps[1].Needs[0] != pl.Steps[0].ID {
		t.Errorf("step 1 needs = %v, want [%s]", pl.Steps[1].Needs, pl.Steps[0].ID)
	}
}

// TestChainStepsToYAML_DropsCWDVar verifies that the runtime cwd var
// (set per-run by the chain bar from the active workspace) is *not*
// baked into the saved YAML — otherwise every saved workflow would
// hard-code the cwd that was active at save time.
func TestChainStepsToYAML_DropsCWDVar(t *testing.T) {
	stepsJSON := `[{"type":"prompt","label":"a","body":"hi"}]`
	yamlStr, err := ChainStepsToYAML(stepsJSON, "x", "", "ollama", "llama3.2")
	if err != nil {
		t.Fatalf("ChainStepsToYAML: %v", err)
	}
	if strings.Contains(yamlStr, "cwd:") {
		t.Errorf("YAML contains baked-in cwd var:\n%s", yamlStr)
	}
}

// TestChainStepsToYAML_RequiresProvider checks that we fail loud at
// save time when no executor can be resolved for a prompt step,
// rather than writing an unrunnable file.
func TestChainStepsToYAML_RequiresProvider(t *testing.T) {
	stepsJSON := `[{"type":"prompt","label":"a","body":"hi"}]`
	_, err := ChainStepsToYAML(stepsJSON, "x", "", "", "")
	if err == nil {
		t.Fatal("want error when no provider is set, got nil")
	}
	if !strings.Contains(err.Error(), "executor") {
		t.Errorf("error %q should mention 'executor'", err.Error())
	}
}

// TestChainStepsToYAML_AgentFlattening checks that an agent step
// followed by a prompt step gets folded into a single executable
// step (mirrors buildPipelineFromChain's behavior so the YAML matches
// what the runner would actually do).
func TestChainStepsToYAML_AgentFlattening(t *testing.T) {
	stepsJSON := `[
		{"type":"agent","name":"copilot","label":"copilot","kind":"agent","invoke":"@copilot"},
		{"type":"prompt","label":"do thing","body":"do the thing"}
	]`
	yamlStr, err := ChainStepsToYAML(stepsJSON, "x", "", "ollama", "llama3.2")
	if err != nil {
		t.Fatalf("ChainStepsToYAML: %v", err)
	}
	pl, err := pipeline.Load(strings.NewReader(yamlStr))
	if err != nil {
		t.Fatalf("pipeline.Load: %v", err)
	}
	if len(pl.Steps) != 1 {
		t.Fatalf("got %d steps, want 1 (agent should be folded into prompt)", len(pl.Steps))
	}
	if !strings.Contains(pl.Steps[0].Prompt, "do the thing") {
		t.Errorf("flattened prompt missing user body: %q", pl.Steps[0].Prompt)
	}
}

func TestChainStepsToYAML_EmptyName(t *testing.T) {
	_, err := ChainStepsToYAML(`[{"type":"prompt","label":"a","body":"hi"}]`, "", "", "ollama", "x")
	if err == nil {
		t.Fatal("want error for empty name, got nil")
	}
}

func TestChainStepsToYAML_EmptySteps(t *testing.T) {
	_, err := ChainStepsToYAML(`[]`, "x", "", "ollama", "x")
	if err == nil {
		t.Fatal("want error for empty steps, got nil")
	}
}
