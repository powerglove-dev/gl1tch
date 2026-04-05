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
	code, err := run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr, execCopilot)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
	}
	os.Exit(code)
}

// knownModels is the static list of GitHub Copilot models (as of 2026).
var knownModels = []struct {
	ID    string `json:"id"`
	Label string `json:"label"`
}{
	{"claude-sonnet-4.6", "Claude Sonnet 4.6"},
	{"claude-sonnet-4.5", "Claude Sonnet 4.5"},
	{"claude-haiku-4.5", "Claude Haiku 4.5"},
	{"claude-opus-4.6", "Claude Opus 4.6"},
	{"gpt-4.1", "GPT-4.1"},
	{"gpt-5.2", "GPT-5.2"},
	{"gemini-3-pro-preview", "Gemini 3 Pro (Preview)"},
}

// execCopilot runs `copilot --prompt <prompt> [--model <model>]`.
// --allow-all is intentionally omitted: it causes the CLI to explore the local
// codebase before answering, which blocks indefinitely for large prompts.
func execCopilot(model, prompt string, stdout, stderr io.Writer) int {
	args := []string{"--prompt", prompt}
	if model != "" {
		args = append(args, "--model", model)
	}
	cmd := exec.Command("copilot", args...)
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

// run is the testable entry point.
func run(
	args []string,
	stdin io.Reader,
	stdout io.Writer,
	stderr io.Writer,
	executor func(model, prompt string, stdout, stderr io.Writer) int,
) (int, error) {
	for _, arg := range args {
		if arg == "--list-models" {
			out, err := json.Marshal(knownModels)
			if err != nil {
				return 1, fmt.Errorf("marshalling models: %w", err)
			}
			fmt.Fprintln(stdout, string(out))
			return 0, nil
		}
	}

	if _, err := exec.LookPath("copilot"); err != nil {
		return 1, fmt.Errorf("copilot binary not found in PATH: install it from https://github.com/github/copilot-cli")
	}

	promptBytes, err := io.ReadAll(stdin)
	if err != nil {
		return 1, fmt.Errorf("reading stdin: %w", err)
	}
	prompt := strings.TrimSpace(string(promptBytes))
	if prompt == "" {
		return 1, fmt.Errorf("prompt is required: no input received on stdin")
	}

	fullPrompt := glitchctx.ProtocolInstructions + glitchctx.BuildShellContext() + "\n## User Request\n" + prompt
	model := os.Getenv("GLITCH_MODEL")
	return executor(model, fullPrompt, stdout, stderr), nil
}
