package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
)

// knownClaudeModels is the static list of Claude models for --list-models.
var knownClaudeModels = []struct {
	ID    string `json:"id"`
	Label string `json:"label"`
}{
	{"claude-opus-4-6", "Opus 4.6"},
	{"claude-sonnet-4-6", "Sonnet 4.6"},
	{"claude-haiku-4-5-20251001", "Haiku 4.5"},
}

func main() {
	// Handle --list-models before everything else.
	for _, arg := range os.Args[1:] {
		if arg == "--list-models" {
			out, err := json.Marshal(knownClaudeModels)
			if err != nil {
				fmt.Fprintln(os.Stderr, err)
				os.Exit(1)
			}
			fmt.Println(string(out))
			os.Exit(0)
		}
	}

	code, err := run(os.Stdin, os.Stdout, os.Stderr, os.Getenv, execClaude)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
	}
	os.Exit(code)
}

// execClaude runs `claude --print --dangerously-skip-permissions [--model <model>]` with the prompt on stdin.
func execClaude(model, prompt string, stdout, stderr io.Writer) int {
	args := []string{"--print", "--dangerously-skip-permissions"}
	if model != "" {
		args = append(args, "--model", model)
	}
	cmd := exec.Command("claude", args...)
	cmd.Stdin = strings.NewReader(prompt)
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
	stdin io.Reader,
	stdout io.Writer,
	stderr io.Writer,
	getenv func(string) string,
	executor func(model, prompt string, stdout, stderr io.Writer) int,
) (int, error) {
	if _, err := exec.LookPath("claude"); err != nil {
		return 1, fmt.Errorf("claude CLI not found in PATH: install it from https://claude.ai/code")
	}

	promptBytes, err := io.ReadAll(stdin)
	if err != nil {
		return 1, fmt.Errorf("reading stdin: %w", err)
	}
	prompt := strings.TrimSpace(string(promptBytes))
	if prompt == "" {
		return 1, fmt.Errorf("prompt is required: no input received on stdin")
	}

	model := getenv("GLITCH_MODEL")
	return executor(model, prompt, stdout, stderr), nil
}
