package pipeline

import (
	"bytes"
	"context"
	"io"

	"github.com/adam-stokes/orcai/internal/plugin"
)

// StepExecutor is the unified interface for all step types: builtins, plugins, and
// future extension points. The runner calls Init once, then Execute (possibly multiple
// times for retries), then Cleanup — Cleanup is always called even on failure.
type StepExecutor interface {
	Init(ctx context.Context) error
	Execute(ctx context.Context, args map[string]any) (map[string]any, error)
	Cleanup(ctx context.Context) error
}

// pluginExecutor adapts the legacy plugin.Plugin interface to StepExecutor.
// Execute collects output into a buffer and returns {"value": <string>}.
type pluginExecutor struct {
	pl     plugin.Plugin
	input  string
	vars   map[string]string
	w      io.Writer
	Prompt string // the resolved prompt sent to the plugin; set at construction
}

func newPluginExecutor(pl plugin.Plugin, input string, vars map[string]string, w io.Writer) *pluginExecutor {
	return &pluginExecutor{pl: pl, input: input, vars: vars, w: w, Prompt: input}
}

func (pe *pluginExecutor) Init(_ context.Context) error { return nil }

func (pe *pluginExecutor) Execute(ctx context.Context, _ map[string]any) (map[string]any, error) {
	var buf bytes.Buffer
	err := pe.pl.Execute(ctx, pe.input, pe.vars, &buf)
	out := map[string]any{"value": buf.String()}
	if err != nil {
		// Return partial output alongside the error so callers (e.g. brain note
		// parsing) can inspect whatever was written before the failure.
		return out, err
	}
	// Mirror output to the pipeline writer if provided.
	if pe.w != nil {
		_, _ = pe.w.Write(buf.Bytes())
	}
	return out, nil
}

func (pe *pluginExecutor) Cleanup(_ context.Context) error { return nil }

