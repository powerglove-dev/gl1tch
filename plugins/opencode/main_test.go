package main

import (
	"bytes"
	"io"
	"strings"
	"testing"
)

// stubExecutor records calls and returns a fixed exit code.
type stubExecutor struct {
	calledModel  string
	calledPrompt string
	exitCode     int
	output       string
}

func (s *stubExecutor) exec(model, prompt string, stdout, stderr io.Writer) int {
	s.calledModel = model
	s.calledPrompt = prompt
	if s.output != "" {
		stdout.Write([]byte(s.output))
	}
	return s.exitCode
}

func noEnv(string) string { return "" }

func envMap(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

func TestRun_EmptyStdin(t *testing.T) {
	stub := &stubExecutor{}
	var stdout, stderr bytes.Buffer
	code, err := run([]string{"--model", "ollama/llama3.2"}, strings.NewReader("  \n  "), &stdout, &stderr, noEnv, stub.exec)
	if err == nil || code == 0 {
		t.Fatal("expected non-zero exit and error for empty stdin")
	}
	if !strings.Contains(err.Error(), "stdin") && !strings.Contains(err.Error(), "input") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestRun_MissingModel(t *testing.T) {
	stub := &stubExecutor{}
	var stdout, stderr bytes.Buffer
	code, err := run([]string{}, strings.NewReader("hello"), &stdout, &stderr, noEnv, stub.exec)
	if err == nil || code == 0 {
		t.Fatal("expected non-zero exit and error for missing model")
	}
	if !strings.Contains(err.Error(), "model") {
		t.Errorf("expected 'model' in error, got: %v", err)
	}
}

func TestRun_ModelFromFlag(t *testing.T) {
	stub := &stubExecutor{exitCode: 0, output: "response text"}
	var stdout, stderr bytes.Buffer
	code, err := run([]string{"--model", "ollama/llama3.2"}, strings.NewReader("ping"), &stdout, &stderr, noEnv, stub.exec)
	if err != nil || code != 0 {
		t.Fatalf("unexpected error=%v code=%d", err, code)
	}
	if stub.calledModel != "ollama/llama3.2" {
		t.Errorf("expected model ollama/llama3.2, got %q", stub.calledModel)
	}
	// run() wraps the user's prompt with shell context + the output protocol,
	// so just check the original survives the wrap.
	if !strings.Contains(stub.calledPrompt, "ping") {
		t.Errorf("expected user prompt 'ping' inside wrapped prompt, got %q", stub.calledPrompt)
	}
	if !strings.Contains(stub.calledPrompt, "<<GLITCH_TEXT>>") {
		t.Errorf("expected output protocol injected, got %q", stub.calledPrompt)
	}
	if stdout.String() != "response text" {
		t.Errorf("expected 'response text' in stdout, got %q", stdout.String())
	}
}

func TestRun_ModelFromEnv(t *testing.T) {
	stub := &stubExecutor{exitCode: 0, output: "ok"}
	var stdout, stderr bytes.Buffer
	code, err := run(
		[]string{},
		strings.NewReader("hello"),
		&stdout, &stderr,
		envMap(map[string]string{"GLITCH_MODEL": "ollama/qwen3.5"}),
		stub.exec,
	)
	if err != nil || code != 0 {
		t.Fatalf("unexpected error=%v code=%d", err, code)
	}
	if stub.calledModel != "ollama/qwen3.5" {
		t.Errorf("expected model ollama/qwen3.5, got %q", stub.calledModel)
	}
}

func TestRun_FlagOverridesEnv(t *testing.T) {
	stub := &stubExecutor{exitCode: 0}
	var stdout, stderr bytes.Buffer
	code, err := run(
		[]string{"--model", "ollama/llama3.2"},
		strings.NewReader("hi"),
		&stdout, &stderr,
		envMap(map[string]string{"GLITCH_MODEL": "ollama/qwen3.5"}),
		stub.exec,
	)
	if err != nil || code != 0 {
		t.Fatalf("unexpected: %v %d", err, code)
	}
	if stub.calledModel != "ollama/llama3.2" {
		t.Errorf("flag should override env: expected llama3.2, got %q", stub.calledModel)
	}
}

func TestRun_ExitCodePropagated(t *testing.T) {
	stub := &stubExecutor{exitCode: 2}
	var stdout, stderr bytes.Buffer
	code, _ := run(
		[]string{"--model", "ollama/llama3.2"},
		strings.NewReader("hi"),
		&stdout, &stderr,
		noEnv,
		stub.exec,
	)
	if code != 2 {
		t.Errorf("expected exit code 2, got %d", code)
	}
}
