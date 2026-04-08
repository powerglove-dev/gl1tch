package main

import (
	"bytes"
	"encoding/json"
	"io"
	"strings"
	"testing"
)

// stubExecutor records calls into the executor so assertions can confirm
// the BYOK env + model args were wired correctly.
type stubExecutor struct {
	calledModel  string
	calledPrompt string
	calledEnv    []string
	exitCode     int
	output       string
}

func (s *stubExecutor) run(model, prompt string, extraEnv []string, stdout, stderr io.Writer) int {
	s.calledModel = model
	s.calledPrompt = prompt
	s.calledEnv = extraEnv
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
	code, err := run(nil, strings.NewReader("   \n  "), &stdout, &stderr, noEnv, stub.run)
	if err == nil || code == 0 {
		t.Fatalf("expected error for empty stdin, got code=%d err=%v", code, err)
	}
}

func TestRun_HostedModel_UsesModelFlag(t *testing.T) {
	stub := &stubExecutor{}
	var stdout, stderr bytes.Buffer
	getenv := envMap(map[string]string{"GLITCH_MODEL": "claude-sonnet-4.6"})
	code, err := run(nil, strings.NewReader("hello"), &stdout, &stderr, getenv, stub.run)
	if err != nil || code != 0 {
		t.Fatalf("unexpected: code=%d err=%v", code, err)
	}
	if stub.calledModel != "claude-sonnet-4.6" {
		t.Errorf("expected hosted model passed through, got %q", stub.calledModel)
	}
	if len(stub.calledEnv) != 0 {
		t.Errorf("hosted model should not set BYOK env, got %v", stub.calledEnv)
	}
	if !strings.Contains(stub.calledPrompt, "hello") {
		t.Errorf("user prompt missing from wrapped prompt: %q", stub.calledPrompt)
	}
}

func TestRun_OllamaModel_SetsBYOKEnv(t *testing.T) {
	stub := &stubExecutor{}
	var stdout, stderr bytes.Buffer
	getenv := envMap(map[string]string{"GLITCH_MODEL": "ollama/qwen2.5:7b"})
	code, err := run(nil, strings.NewReader("ping"), &stdout, &stderr, getenv, stub.run)
	if err != nil || code != 0 {
		t.Fatalf("unexpected: code=%d err=%v", code, err)
	}
	// Model passed through verbatim; executor is responsible for suppressing
	// --model when it sees an ollama/ prefix.
	if stub.calledModel != "ollama/qwen2.5:7b" {
		t.Errorf("expected model ollama/qwen2.5:7b, got %q", stub.calledModel)
	}
	want := map[string]string{
		"COPILOT_PROVIDER_BASE_URL": "http://localhost:11434/v1",
		"COPILOT_MODEL":             "qwen2.5:7b",
		"COPILOT_OFFLINE":           "true",
	}
	got := parseEnv(stub.calledEnv)
	for k, v := range want {
		if got[k] != v {
			t.Errorf("env %s = %q, want %q", k, got[k], v)
		}
	}
}

func TestRun_OllamaModel_CustomBaseURL(t *testing.T) {
	stub := &stubExecutor{}
	var stdout, stderr bytes.Buffer
	getenv := envMap(map[string]string{
		"GLITCH_MODEL":            "ollama/llama3.2:latest",
		"GLITCH_COPILOT_BASE_URL": "http://10.0.0.2:8080/v1",
	})
	code, err := run(nil, strings.NewReader("hi"), &stdout, &stderr, getenv, stub.run)
	if err != nil || code != 0 {
		t.Fatalf("unexpected: code=%d err=%v", code, err)
	}
	got := parseEnv(stub.calledEnv)
	if got["COPILOT_PROVIDER_BASE_URL"] != "http://10.0.0.2:8080/v1" {
		t.Errorf("custom base URL not honoured: %q", got["COPILOT_PROVIDER_BASE_URL"])
	}
	if got["COPILOT_MODEL"] != "llama3.2:latest" {
		t.Errorf("expected stripped model, got %q", got["COPILOT_MODEL"])
	}
}

func TestRun_ListModels_IncludesHosted(t *testing.T) {
	stub := &stubExecutor{}
	var stdout, stderr bytes.Buffer
	code, err := run([]string{"--list-models"}, strings.NewReader(""), &stdout, &stderr, noEnv, stub.run)
	if err != nil || code != 0 {
		t.Fatalf("unexpected: code=%d err=%v", code, err)
	}
	var entries []modelEntry
	if err := json.Unmarshal(bytes.TrimSpace(stdout.Bytes()), &entries); err != nil {
		t.Fatalf("invalid JSON from --list-models: %v (%q)", err, stdout.String())
	}
	// Every hosted entry must be present; local entries are appended best-effort
	// (Ollama may or may not be running in the test sandbox, so we don't assert
	// on their count — only that the hosted catalog survived the merge).
	seen := make(map[string]bool)
	for _, e := range entries {
		seen[e.ID] = true
	}
	for _, h := range hostedModels {
		if !seen[h.ID] {
			t.Errorf("hosted model %q missing from --list-models output", h.ID)
		}
	}
	if stub.calledPrompt != "" {
		t.Error("--list-models should not invoke the executor")
	}
}

// parseEnv turns a []string of "KEY=VALUE" back into a map for assertions.
func parseEnv(env []string) map[string]string {
	out := make(map[string]string, len(env))
	for _, kv := range env {
		if idx := strings.IndexByte(kv, '='); idx >= 0 {
			out[kv[:idx]] = kv[idx+1:]
		}
	}
	return out
}
