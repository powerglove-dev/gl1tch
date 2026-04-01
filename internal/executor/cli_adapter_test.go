package executor_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/powerglove-dev/gl1tch/internal/executor"
)

func TestCliAdapter_Name(t *testing.T) {
	a := executor.NewCliAdapter("echo-tool", "A simple echo tool", "echo")
	if a.Name() != "echo-tool" {
		t.Errorf("expected 'echo-tool', got %q", a.Name())
	}
}

func TestCliAdapter_Execute(t *testing.T) {
	// cat reads stdin and echoes it to stdout — available on macOS/Linux.
	a := executor.NewCliAdapter("cat-tool", "echoes input", "cat")
	var buf bytes.Buffer
	err := a.Execute(context.Background(), "hello world\n", nil, &buf)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(buf.String(), "hello world") {
		t.Errorf("expected output to contain 'hello world', got %q", buf.String())
	}
}

func TestCliAdapter_Execute_ContextCancel(t *testing.T) {
	// sleep 10 will be cancelled by ctx before it finishes.
	a := executor.NewCliAdapter("sleep-tool", "sleeps", "sleep", "10")
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately
	var buf bytes.Buffer
	err := a.Execute(ctx, "", nil, &buf)
	if err == nil {
		t.Error("expected an error when context is cancelled")
	}
}

func writeTempSidecar(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "tool.yaml")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("writeTempSidecar: %v", err)
	}
	return path
}

func TestNewCliAdapterFromSidecar_Valid(t *testing.T) {
	path := writeTempSidecar(t, `
name: my-tool
description: A test tool
command: echo
args: ["-n"]
input_schema: "string"
output_schema: "string"
`)
	a, err := executor.NewCliAdapterFromSidecar(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if a.Name() != "my-tool" {
		t.Errorf("expected name 'my-tool', got %q", a.Name())
	}
	if a.Description() != "A test tool" {
		t.Errorf("expected description 'A test tool', got %q", a.Description())
	}
}

func TestNewCliAdapterFromSidecar_MissingCommand(t *testing.T) {
	path := writeTempSidecar(t, `
name: broken-tool
description: missing command
`)
	_, err := executor.NewCliAdapterFromSidecar(path)
	if err == nil {
		t.Error("expected error for missing command, got nil")
	}
}

func TestNewCliAdapterFromSidecar_FileNotFound(t *testing.T) {
	_, err := executor.NewCliAdapterFromSidecar("/nonexistent/path/tool.yaml")
	if err == nil {
		t.Error("expected error for missing file, got nil")
	}
}

func TestCliAdapter_Capabilities_FromSidecar(t *testing.T) {
	path := writeTempSidecar(t, `
name: schema-tool
command: cat
input_schema: "text/plain"
output_schema: "text/plain"
`)
	a, err := executor.NewCliAdapterFromSidecar(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	caps := a.Capabilities()
	if len(caps) != 1 {
		t.Fatalf("expected 1 capability, got %d", len(caps))
	}
	if caps[0].InputSchema != "text/plain" {
		t.Errorf("expected InputSchema 'text/plain', got %q", caps[0].InputSchema)
	}
	if caps[0].OutputSchema != "text/plain" {
		t.Errorf("expected OutputSchema 'text/plain', got %q", caps[0].OutputSchema)
	}
}

func TestCliAdapter_Capabilities_NoSidecar(t *testing.T) {
	a := executor.NewCliAdapter("plain-tool", "no schema", "echo")
	if a.Capabilities() != nil {
		t.Errorf("expected nil capabilities for non-sidecar adapter, got %v", a.Capabilities())
	}
}

func TestCliAdapter_Execute_VarsAsEnv(t *testing.T) {
	// Use `sh -c 'echo $ORCAI_MY_KEY'` to verify the env var is set on the subprocess.
	a := executor.NewCliAdapter("sh-tool", "shell", "sh", "-c", "echo $ORCAI_MY_KEY")
	var buf bytes.Buffer
	err := a.Execute(context.Background(), "", map[string]string{"my_key": "hello-from-var"}, &buf)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(buf.String(), "hello-from-var") {
		t.Errorf("expected ORCAI_MY_KEY in subprocess output, got %q", buf.String())
	}
}

func TestCliAdapter_Execute_FilterViaEnv(t *testing.T) {
	// Simulate the jq-sidecar pattern: sh -c 'jq "$GLITCH_FILTER"' with JSON on stdin.
	a := executor.NewCliAdapter("jq-sidecar", "jq via env", "sh", "-c", `jq "$GLITCH_FILTER"`)
	var buf bytes.Buffer
	err := a.Execute(context.Background(), `{"name":"glitch"}`, map[string]string{"filter": ".name"}, &buf)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(buf.String(), "glitch") {
		t.Errorf("expected jq output to contain 'orcai', got %q", buf.String())
	}
}
