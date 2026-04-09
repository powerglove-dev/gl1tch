// execute.go is the unified execution entry point for gl1tch-desktop.
// It replaces the previous spread of AskScoped, AskProvider, RunChain,
// StepThroughStartFromChain, and RunWorkflow with a single method that
// routes based on the shape of the request:
//
//   - steps present + step_through  → step-through session
//   - steps present                 → straight chain run
//   - freeform text, no steps       → research loop → thread
//   - line starts with /            → slash command (flat)
//
// Provider/model is orthogonal: it applies to the LLM step of whatever
// runs. The research loop's intelligence stages (plan, score) stay on
// local Ollama; the draft stage respects the user's provider choice.
package glitchd

import (
	"encoding/json"
)

// ExecuteOpts is the single wire type the frontend sends for every
// user message. The routing logic lives server-side so the frontend
// is a dumb pipe: build the opts, call Execute, listen for events.
type ExecuteOpts struct {
	WorkspaceID string          `json:"workspace_id"`
	Input       string          `json:"input"`                  // user text
	Steps       json.RawMessage `json:"steps,omitempty"`        // []ChainStep, optional
	ThreadID    string          `json:"thread_id,omitempty"`    // follow-up in existing thread
	Provider    string          `json:"provider,omitempty"`     // user-selected provider
	Model       string          `json:"model,omitempty"`        // user-selected model
	AgentPath   string          `json:"agent_path,omitempty"`   // optional agent context
	StepThrough bool            `json:"step_through,omitempty"` // pause between steps?
}

// hasSteps returns true when the opts carry a non-empty chain.
func (o ExecuteOpts) HasSteps() bool {
	if len(o.Steps) == 0 {
		return false
	}
	// Reject "null" and "[]" — both mean "no chain."
	trimmed := string(o.Steps)
	return trimmed != "null" && trimmed != "[]"
}
