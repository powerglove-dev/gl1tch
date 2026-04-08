package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	"github.com/8op-org/gl1tch/glitchctx"
)

func main() {
	code, err := run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr, os.Getenv, execOpencode)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
	}
	os.Exit(code)
}

// execOpencode is the production executor: runs the real opencode binary.
// Uses --format json and extracts only "text" event parts so that tool calls,
// step markers, and other internal events never reach the caller's writer.
func execOpencode(model, prompt string, stdout, stderr io.Writer) int {
	pr, pw, err := os.Pipe()
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}

	cmd := exec.Command("opencode", "run", "--model", model, "--format", "json", "--", prompt)
	cmd.Stdout = pw
	cmd.Stderr = stderr

	if err := cmd.Start(); err != nil {
		pw.Close()
		pr.Close()
		fmt.Fprintln(stderr, err)
		return 1
	}

	pw.Close() // parent no longer writes; child holds the write end

	// Stream only text content from the JSON event line.
	scanner := bufio.NewScanner(pr)
	for scanner.Scan() {
		var event struct {
			Type string `json:"type"`
			Part struct {
				Text string `json:"text"`
			} `json:"part"`
		}
		if json.Unmarshal(scanner.Bytes(), &event) == nil && event.Type == "text" && event.Part.Text != "" {
			fmt.Fprint(stdout, event.Part.Text)
		}
	}
	pr.Close()

	if err := cmd.Wait(); err != nil {
		if exit, ok := err.(*exec.ExitError); ok {
			return exit.ExitCode()
		}
		fmt.Fprintln(stderr, err)
		return 1
	}
	return 0
}

// run is the testable entry point.
// executor is injectable so tests can replace opencode with a stub.
func run(
	args []string,
	stdin io.Reader,
	stdout io.Writer,
	stderr io.Writer,
	getenv func(string) string,
	executor func(model, prompt string, stdout, stderr io.Writer) int,
) (int, error) {
	fs := flag.NewFlagSet("orcai-opencode", flag.ContinueOnError)
	fs.SetOutput(stderr)
	modelFlag := fs.String("model", "", "Model in provider/model format (e.g. ollama/llama3.2)")
	if err := fs.Parse(args); err != nil {
		return 1, nil // flag already wrote the error
	}

	// Read prompt from stdin.
	promptBytes, err := io.ReadAll(stdin)
	if err != nil {
		return 1, fmt.Errorf("reading stdin: %w", err)
	}
	prompt := strings.TrimSpace(string(promptBytes))
	if prompt == "" {
		return 1, fmt.Errorf("prompt is required: no input received on stdin")
	}

	// Resolve model: --model flag takes precedence over GLITCH_MODEL env var.
	model := *modelFlag
	if model == "" {
		model = getenv("GLITCH_MODEL")
	}
	if model == "" {
		return 1, fmt.Errorf("model is required: set --model flag or GLITCH_MODEL environment variable")
	}

	// For Ollama-backed models, pull the model if not already present.
	if err := glitchctx.PullOllamaModel(model, stderr); err != nil {
		return 1, fmt.Errorf("pulling Ollama model: %w", err)
	}

	// opencode is an agentic CLI with native tool access: it can read
	// files, run commands, and inspect the working directory by itself.
	// We deliberately do NOT inject a hardcoded shell-context preamble
	// (cwd, directory listing, git status) here — if the model needs any
	// of that it can call its own tools, and baking it into every prompt
	// is exactly the kind of "hardcoded pre-context" the AI-first
	// redesign removed.
	//
	// The output protocol instructions remain because they're a
	// rendering contract (how the desktop chat parses replies into
	// structured blocks), not a context manual. Drop this if/when we
	// switch opencode's output to a native structured format.
	fullPrompt := glitchctx.OutputProtocolInstructions + "\n## User Request\n" + prompt
	code := executor(model, fullPrompt, stdout, stderr)
	return code, nil
}
