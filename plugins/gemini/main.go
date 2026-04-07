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

// knownGeminiModels is the static list of Gemini models for --list-models.
var knownGeminiModels = []struct {
	ID    string `json:"id"`
	Label string `json:"label"`
}{
	{"gemini-2.5-pro", "Gemini 2.5 Pro"},
	{"gemini-2.5-flash", "Gemini 2.5 Flash"},
	{"gemini-2.0-flash", "Gemini 2.0 Flash"},
	{"gemini-1.5-pro", "Gemini 1.5 Pro"},
}

func main() {
	// Handle --list-models before everything else.
	for _, arg := range os.Args[1:] {
		if arg == "--list-models" {
			out, err := json.Marshal(knownGeminiModels)
			if err != nil {
				fmt.Fprintln(os.Stderr, err)
				os.Exit(1)
			}
			fmt.Println(string(out))
			os.Exit(0)
		}
	}

	code, err := run(os.Stdin, os.Stdout, os.Stderr, os.Getenv, execGemini)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
	}
	os.Exit(code)
}

// execGemini runs `gemini -p --approval-mode yolo [--model <model>]` with the prompt on stdin.
func execGemini(model, prompt string, stdout, stderr io.Writer) int {
	args := []string{"-p", "--approval-mode", "yolo"}
	if model != "" {
		args = append(args, "--model", model)
	}
	cmd := exec.Command("gemini", args...)
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
	if _, err := exec.LookPath("gemini"); err != nil {
		return 1, fmt.Errorf("gemini CLI not found in PATH: install it from https://ai.google.dev/gemini-api/docs/gemini-cli")
	}

	promptBytes, err := io.ReadAll(stdin)
	if err != nil {
		return 1, fmt.Errorf("reading stdin: %w", err)
	}
	prompt := strings.TrimSpace(string(promptBytes))
	if prompt == "" {
		return 1, fmt.Errorf("prompt is required: no input received on stdin")
	}

	// gemini-cli is an agentic CLI with full tool access — only inject the
	// OUTPUT protocol so its replies render as structured chat blocks.
	fullPrompt := glitchctx.OutputProtocolInstructions +
		"\n## User Request\n" + prompt

	model := getenv("GLITCH_MODEL")
	return executor(model, fullPrompt, stdout, stderr), nil
}
