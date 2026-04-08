package glitchd

import (
	"os"
)

// BuildAgentPrompt reads an agent's instructions and prepends them to the
// user prompt. Used by chain steps that attach a workspace agent file to
// the next prompt step.
//
// The old BuildSystemContext helper that used to live in this file —
// a 100-line hardcoded prose block teaching every model about workflow
// YAML syntax, executor names, and response rules — has been deleted.
// That kind of pre-context is exactly what the AI-first redesign is
// ripping out: the model handles context itself via tool use, and the
// user's skills/capabilities are the only contextual surface the
// assistant needs. Chain runs no longer inject a system prompt at all.
func BuildAgentPrompt(agentPath, userPrompt string) string {
	data, err := os.ReadFile(agentPath)
	if err != nil {
		return userPrompt
	}
	return string(data) + "\n\n---\n\nUser request:\n" + userPrompt
}
