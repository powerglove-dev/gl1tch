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
	code, err := run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr, execClaude)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
	}
	os.Exit(code)
}

var knownModels = []struct {
	ID    string `json:"id"`
	Label string `json:"label"`
}{
	{"claude-haiku-4-5-20251001", "Claude Haiku 4.5"},
	{"claude-sonnet-4-5-20251001", "Claude Sonnet 4.5"},
	{"claude-sonnet-4-6", "Claude Sonnet 4.6"},
}

func execClaude(model, prompt string, stdout, stderr io.Writer) int {
	args := []string{"-p", prompt, "--output-format", "text"}
	if model != "" {
		args = append(args, "--model", model)
	}
	cmd := exec.Command("claude", args...)
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

func run(args []string, stdin io.Reader, stdout io.Writer, stderr io.Writer, executor func(model, prompt string, stdout, stderr io.Writer) int) (int, error) {
	for _, arg := range args {
		if arg == "--list-models" {
			out, _ := json.Marshal(knownModels)
			fmt.Fprintln(stdout, string(out))
			return 0, nil
		}
	}
	if _, err := exec.LookPath("claude"); err != nil {
		return 1, fmt.Errorf("claude binary not found in PATH")
	}
	promptBytes, err := io.ReadAll(stdin)
	if err != nil {
		return 1, fmt.Errorf("reading stdin: %w", err)
	}
	prompt := strings.TrimSpace(string(promptBytes))
	if prompt == "" {
		return 1, fmt.Errorf("prompt is required")
	}
	// claude-code is an agentic CLI with full tool access, so we only inject
	// the OUTPUT protocol — its replies need to land in the gl1tch chat as
	// structured blocks (notes, tables, status pings) rather than raw stdout
	// the splitter has to reverse-engineer.
	fullPrompt := glitchctx.OutputProtocolInstructions +
		"\n## User Request\n" + prompt
	model := os.Getenv("GLITCH_MODEL")
	return executor(model, fullPrompt, stdout, stderr), nil
}
