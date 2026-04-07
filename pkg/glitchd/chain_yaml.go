// chain_yaml.go converts a desktop builder chain (ChainStep[]) into a
// gl1tch workflow YAML string. This is the serialization half of the
// "single source of truth = YAML files on disk" rule from Phase 3 of
// the editor-popup work — chains saved from the chat bar no longer
// live in SQLite, they become real .workflow.yaml files in
// <workspace>/.glitch/workflows/.
//
// The serializer reuses buildPipelineFromChain so the wire shape it
// produces is exactly the same as what the runner would execute. That
// guarantees a saved chain runs identically whether you load it back
// into the chat bar or run the YAML directly.
package glitchd

import (
	"encoding/json"
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

// workflowYAML is a slim, omitempty-friendly mirror of pipeline.Pipeline
// used only for *writing* workflow files. We don't reuse pipeline.Pipeline
// directly because its yaml tags lack omitempty and a fresh marshal would
// emit a wall of empty keys (description: "", version: "", etc.) — fine
// for the runner, ugly for a file the user might open and read.
//
// Workflows written through this type are still loadable by pipeline.Load
// because the loader accepts any superset of the fields it needs.
type workflowYAML struct {
	Name        string        `yaml:"name"`
	Version     string        `yaml:"version,omitempty"`
	Description string        `yaml:"description,omitempty"`
	Steps       []workflowStep `yaml:"steps"`
}

// workflowStep mirrors pipeline.Step with omitempty on every optional
// field so the rendered YAML stays lean. Field order matches the order
// users typically read: id → executor/model → prompt → wiring.
type workflowStep struct {
	ID        string            `yaml:"id"`
	Executor  string            `yaml:"executor,omitempty"`
	Model     string            `yaml:"model,omitempty"`
	Prompt    string            `yaml:"prompt,omitempty"`
	Needs     []string          `yaml:"needs,omitempty"`
	Outputs   map[string]string `yaml:"outputs,omitempty"`
	NoClarify bool              `yaml:"no_clarify,omitempty"`
	Vars      map[string]string `yaml:"vars,omitempty"`
}

// ChainStepsToYAML serializes a desktop chain into a workflow YAML
// string. defaultProvider/defaultModel are baked into any prompt step
// that doesn't have its own override — that locks in the picker state
// at save time so the file is fully self-contained. Users who want a
// different provider can edit the YAML in the popup later (Phase 2).
//
// Returns an error if no provider can be resolved for any prompt step,
// rather than silently writing a YAML the runner will reject. Better
// to fail loudly at the save button than to land an unrunnable file.
func ChainStepsToYAML(stepsJSON, name, description, defaultProvider, defaultModel string) (string, error) {
	var steps []ChainStep
	if err := json.Unmarshal([]byte(stepsJSON), &steps); err != nil {
		return "", fmt.Errorf("chain yaml: parse steps: %w", err)
	}
	if len(steps) == 0 {
		return "", fmt.Errorf("chain yaml: no steps to serialize")
	}
	if strings.TrimSpace(name) == "" {
		return "", fmt.Errorf("chain yaml: name is required")
	}

	// Reuse the runner's chain → pipeline expansion so the saved file
	// behaves identically to a chat-bar run. We pass an empty SystemCtx
	// because the runner re-injects it at execution time anyway, and
	// baking it into the file would freeze the system context the user
	// had at save time (which is exactly what we don't want — the file
	// should re-pick up the *current* context every run).
	pl, err := buildPipelineFromChain(steps, RunChainOpts{
		DefaultProvider: defaultProvider,
		DefaultModel:    defaultModel,
	})
	if err != nil {
		// buildPipelineFromChain errors are already user-readable; pass
		// them through unwrapped so the desktop save button can display
		// them verbatim.
		return "", err
	}

	out := workflowYAML{
		Name:        name,
		Version:     "1",
		Description: description,
		Steps:       make([]workflowStep, 0, len(pl.Steps)),
	}
	for _, s := range pl.Steps {
		// Drop the cwd var injected by buildPipelineFromChain — that's a
		// runtime concern (set per-run by the chain bar from the active
		// workspace), not something to bake into a saved file. Otherwise
		// every saved workflow would point at whatever cwd was active at
		// save time and run there forever.
		vars := s.Vars
		if vars != nil {
			cleaned := map[string]string{}
			for k, v := range vars {
				if k == "cwd" {
					continue
				}
				cleaned[k] = v
			}
			if len(cleaned) == 0 {
				vars = nil
			} else {
				vars = cleaned
			}
		}
		out.Steps = append(out.Steps, workflowStep{
			ID:        s.ID,
			Executor:  s.Executor,
			Model:     s.Model,
			Prompt:    s.Prompt,
			Needs:     s.Needs,
			Outputs:   s.Outputs,
			NoClarify: s.NoClarify,
			Vars:      vars,
		})
	}

	raw, err := yaml.Marshal(out)
	if err != nil {
		return "", fmt.Errorf("chain yaml: marshal: %w", err)
	}
	return string(raw), nil
}
