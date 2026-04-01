package pipeline

import (
	"bytes"
	"context"
	"io"

	"github.com/powerglove-dev/gl1tch/internal/executor"
)

// StepExecutor is the unified interface for all step types: builtins, plugins, and
// future extension points. The runner calls Init once, then Execute (possibly multiple
// times for retries), then Cleanup — Cleanup is always called even on failure.
type StepExecutor interface {
	Init(ctx context.Context) error
	Execute(ctx context.Context, args map[string]any) (map[string]any, error)
	Cleanup(ctx context.Context) error
}

// registeredExecutor adapts the executor.Executor interface to StepExecutor.
// Execute collects output into a buffer and returns {"value": <string>}.
type registeredExecutor struct {
	exec   executor.Executor
	input  string
	vars   map[string]string
	w      io.Writer
	Prompt string // the resolved prompt sent to the executor; set at construction
}

func newRegisteredExecutor(exec executor.Executor, input string, vars map[string]string, w io.Writer) *registeredExecutor {
	return &registeredExecutor{exec: exec, input: input, vars: vars, w: w, Prompt: input}
}

func (re *registeredExecutor) Init(_ context.Context) error { return nil }

func (re *registeredExecutor) Execute(ctx context.Context, _ map[string]any) (map[string]any, error) {
	var buf bytes.Buffer
	err := re.exec.Execute(ctx, re.input, re.vars, &buf)
	out := map[string]any{"value": buf.String()}
	if err != nil {
		// Return partial output alongside the error so callers (e.g. brain note
		// parsing) can inspect whatever was written before the failure.
		return out, err
	}
	// Mirror output to the pipeline writer if provided.
	if re.w != nil {
		_, _ = re.w.Write(buf.Bytes())
	}
	return out, nil
}

func (re *registeredExecutor) Cleanup(_ context.Context) error { return nil }

