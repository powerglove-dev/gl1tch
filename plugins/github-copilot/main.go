package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	"github.com/8op-org/gl1tch/glitchctx"
)

func main() {
	code, err := run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr, os.Getenv, execCopilot)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
	}
	os.Exit(code)
}

// modelEntry is the shape emitted by --list-models. Kept as a small local
// type so the JSON shape is stable across hosted and local entries.
type modelEntry struct {
	ID    string `json:"id"`
	Label string `json:"label"`
}

// hostedModels is the static catalog of GitHub-hosted Copilot models.
// Local Ollama models are appended at --list-models time via
// glitchctx.QueryOllamaModels so the picker sees them both.
var hostedModels = []modelEntry{
	{"claude-sonnet-4.6", "Claude Sonnet 4.6"},
	{"claude-sonnet-4.5", "Claude Sonnet 4.5"},
	{"claude-haiku-4.5", "Claude Haiku 4.5"},
	{"claude-opus-4.6", "Claude Opus 4.6"},
	{"gpt-4.1", "GPT-4.1"},
	{"gpt-5.2", "GPT-5.2"},
	{"gemini-3-pro-preview", "Gemini 3 Pro (Preview)"},
}

// listModels returns hostedModels plus any local Ollama tags, each with the
// "ollama/" scheme so the BYOK path kicks in when one is selected.
func listModels() []modelEntry {
	out := make([]modelEntry, 0, len(hostedModels)+4)
	out = append(out, hostedModels...)
	for _, name := range glitchctx.QueryOllamaModels() {
		out = append(out, modelEntry{
			ID:    glitchctx.OllamaPrefix + name,
			Label: name + " (local)",
		})
	}
	return out
}

// execCopilot runs `copilot --prompt <prompt> [--model <model>]`.
//
// For hosted models the model is passed via --model. For BYOK/Ollama models
// (anything with an "ollama/" prefix) the copilot CLI is configured via
// COPILOT_PROVIDER_BASE_URL / COPILOT_MODEL / COPILOT_OFFLINE env vars
// instead; --model is omitted in that path because it targets the hosted
// catalog only. Extra env entries are merged with os.Environ() so the child
// still sees PATH, HOME, GH_TOKEN, etc.
//
// --allow-all is intentionally omitted: it causes the CLI to explore the
// local codebase before answering, which blocks indefinitely for large
// prompts.
func execCopilot(model, prompt string, extraEnv []string, stdout, stderr io.Writer) int {
	if _, err := exec.LookPath("copilot"); err != nil {
		fmt.Fprintln(stderr, "copilot binary not found in PATH: install it from https://github.com/github/copilot-cli")
		return 1
	}
	args := []string{"--prompt", prompt}
	if model != "" && !glitchctx.IsOllamaModel(model) {
		args = append(args, "--model", model)
	}
	cmd := exec.Command("copilot", args...)
	if len(extraEnv) > 0 {
		cmd.Env = append(os.Environ(), extraEnv...)
	}
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = stderr
	if err := cmd.Run(); err != nil {
		if exit, ok := err.(*exec.ExitError); ok {
			return exit.ExitCode()
		}
		fmt.Fprintln(stderr, err)
		return 1
	}

	out := glitchctx.ProcessBlocks(buf.String(), stdout, stderr)
	fmt.Fprint(stdout, out)
	return 0
}

// byokEnv returns the COPILOT_PROVIDER_* variables required to route a
// local Ollama model through copilot CLI's BYOK path. getenv lets callers
// override the endpoint (e.g. a remote Ollama or vLLM) via
// GLITCH_COPILOT_BASE_URL without touching shell exports.
//
// Returns nil for non-ollama models so the caller can branch on len().
func byokEnv(model string, getenv func(string) string) []string {
	if !glitchctx.IsOllamaModel(model) {
		return nil
	}
	base := getenv("GLITCH_COPILOT_BASE_URL")
	if base == "" {
		base = glitchctx.DefaultOllamaBaseURL + "/v1"
	}
	return []string{
		"COPILOT_PROVIDER_BASE_URL=" + base,
		"COPILOT_MODEL=" + glitchctx.StripOllamaPrefix(model),
		"COPILOT_OFFLINE=true",
	}
}

// run is the testable entry point.
func run(
	args []string,
	stdin io.Reader,
	stdout io.Writer,
	stderr io.Writer,
	getenv func(string) string,
	executor func(model, prompt string, extraEnv []string, stdout, stderr io.Writer) int,
) (int, error) {
	for _, arg := range args {
		if arg == "--list-models" {
			data, err := json.Marshal(listModels())
			if err != nil {
				return 1, fmt.Errorf("marshalling models: %w", err)
			}
			fmt.Fprintln(stdout, string(data))
			return 0, nil
		}
	}

	promptBytes, err := io.ReadAll(stdin)
	if err != nil {
		return 1, fmt.Errorf("reading stdin: %w", err)
	}
	prompt := strings.TrimSpace(string(promptBytes))
	if prompt == "" {
		return 1, fmt.Errorf("prompt is required: no input received on stdin")
	}

	// Inject both the input protocol (GLITCH_WRITE/RUN — lets the model
	// trigger side effects) and the output protocol (GLITCH_TEXT/NOTE/…
	// — lets the gl1tch chat render structured blocks). The order matters:
	// instructions first, then shell snapshot, then the user's request.
	fullPrompt := glitchctx.ProtocolInstructions +
		glitchctx.OutputProtocolInstructions +
		glitchctx.BuildShellContext() +
		"\n## User Request\n" + prompt

	model := getenv("GLITCH_MODEL")

	// For Ollama-backed models, pull before handing off so copilot doesn't
	// error on first request with "model not found". No-op for hosted IDs.
	if err := glitchctx.PullOllamaModel(model, stderr); err != nil {
		return 1, fmt.Errorf("pulling Ollama model: %w", err)
	}

	extraEnv := byokEnv(model, getenv)
	return executor(model, fullPrompt, extraEnv, stdout, stderr), nil
}
