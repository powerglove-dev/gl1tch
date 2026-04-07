package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	"github.com/8op-org/gl1tch/glitchctx"
)

func main() {
	code, err := run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr, execCodex)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
	}
	os.Exit(code)
}

// knownModels is the static list of OpenAI Codex models.
var knownModels = []struct {
	ID    string `json:"id"`
	Label string `json:"label"`
}{
	{"codex-mini-latest", "Codex Mini (Latest)"},
	{"o4-mini", "o4-mini"},
	{"o3", "o3"},
}

// execCodex runs `codex --prompt <prompt> [--model <model>]` with full tool access.
func execCodex(model, prompt string, stdout, stderr io.Writer) int {
	args := []string{"--prompt", prompt, "--dangerously-bypass-approvals-and-sandbox"}
	if model != "" {
		args = append(args, "--model", model)
	}
	cmd := exec.Command("codex", args...)
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	if err := cmd.Run(); err != nil {
		if exit, ok := err.(*exec.ExitError); ok {
			return exit.ExitCode()
		}
		fmt.Fprintln(stderr, err)
		return 1
	}
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
	// Handle --list-models before any binary checks.
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

	if _, err := exec.LookPath("codex"); err != nil {
		return 1, fmt.Errorf("codex binary not found in PATH: install it from https://github.com/openai/codex")
	}

	promptBytes, err := io.ReadAll(stdin)
	if err != nil {
		return 1, fmt.Errorf("reading stdin: %w", err)
	}
	prompt := strings.TrimSpace(string(promptBytes))
	if prompt == "" {
		return 1, fmt.Errorf("prompt is required: no input received on stdin")
	}

	// codex is an agentic CLI with full tool access — only inject the
	// OUTPUT protocol so its replies render as structured chat blocks.
	fullPrompt := glitchctx.OutputProtocolInstructions +
		"\n## User Request\n" + prompt

	model := os.Getenv("GLITCH_MODEL")
	return executor(model, fullPrompt, stdout, stderr), nil
}
