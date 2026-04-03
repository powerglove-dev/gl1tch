package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"testing"
)

// captureExecutor records calls for assertion in tests.
type captureExecutor struct {
	calledModel  string
	calledPrompt string
	exitCode     int
}

func (c *captureExecutor) exec(model, prompt string, stdout, stderr io.Writer) int {
	c.calledModel = model
	c.calledPrompt = prompt
	return c.exitCode
}

// stdinRun is run() with the codex binary check bypassed, for unit testing
// stdin/model/executor behavior in isolation. It mirrors run()'s post-check logic.
func stdinRun(
	stdin io.Reader,
	stdout io.Writer,
	stderr io.Writer,
	modelEnv string,
	executor func(model, prompt string, stdout, stderr io.Writer) int,
) (int, error) {
	promptBytes, err := io.ReadAll(stdin)
	if err != nil {
		return 1, fmt.Errorf("reading stdin: %w", err)
	}
	prompt := strings.TrimSpace(string(promptBytes))
	if prompt == "" {
		return 1, fmt.Errorf("prompt is required: no input received on stdin")
	}
	return executor(modelEnv, prompt, stdout, stderr), nil
}

func TestListModels_OutputsJSON(t *testing.T) {
	var stdout bytes.Buffer
	code, err := run([]string{"--list-models"}, strings.NewReader(""), &stdout, io.Discard, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if code != 0 {
		t.Fatalf("expected exit 0, got %d", code)
	}
	var models []struct {
		ID    string `json:"id"`
		Label string `json:"label"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(stdout.String())), &models); err != nil {
		t.Fatalf("invalid JSON: %v — output: %s", err, stdout.String())
	}
	if len(models) == 0 {
		t.Fatal("expected at least one model")
	}
}

func TestListModels_NoBinaryRequired(t *testing.T) {
	// --list-models must not invoke the executor (codex binary not needed).
	var stdout bytes.Buffer
	code, err := run([]string{"--list-models"}, strings.NewReader(""), &stdout, io.Discard, nil)
	if err != nil || code != 0 {
		t.Fatalf("--list-models failed: code=%d err=%v", code, err)
	}
}

func TestEmptyStdin_ReturnsError(t *testing.T) {
	cap := &captureExecutor{}
	_, err := stdinRun(strings.NewReader("   \n"), io.Discard, io.Discard, "", cap.exec)
	if err == nil {
		t.Fatal("expected error for empty stdin")
	}
	if !strings.Contains(err.Error(), "prompt is required") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestPromptForwarded(t *testing.T) {
	cap := &captureExecutor{exitCode: 0}
	code, err := stdinRun(strings.NewReader("explain this code"), io.Discard, io.Discard, "", cap.exec)
	if err != nil || code != 0 {
		t.Fatalf("unexpected: code=%d err=%v", code, err)
	}
	if cap.calledPrompt != "explain this code" {
		t.Fatalf("prompt not forwarded: got %q", cap.calledPrompt)
	}
}

func TestModelInjection(t *testing.T) {
	cap := &captureExecutor{exitCode: 0}
	_, err := stdinRun(strings.NewReader("hello"), io.Discard, io.Discard, "o4-mini", cap.exec)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cap.calledModel != "o4-mini" {
		t.Fatalf("expected model o4-mini, got %q", cap.calledModel)
	}
}
