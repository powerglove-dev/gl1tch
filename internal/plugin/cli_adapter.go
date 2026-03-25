package plugin

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	"gopkg.in/yaml.v3"
)

// SidecarSchema is the structure of a ~/.config/orcai/wrappers/<name>.yaml file.
type SidecarSchema struct {
	Name         string   `yaml:"name"`
	Description  string   `yaml:"description"`
	Command      string   `yaml:"command"`
	Args         []string `yaml:"args"`
	InputSchema  string   `yaml:"input_schema"`
	OutputSchema string   `yaml:"output_schema"`
}

// CliAdapter wraps an arbitrary CLI tool as a Tier 2 Plugin.
// Input is written to the subprocess stdin; stdout/stderr is streamed to the writer.
// args are fixed command-line arguments prepended to every Execute call.
type CliAdapter struct {
	name string
	desc string
	cmd  string
	args []string
	caps []Capability
}

// NewCliAdapter creates a Tier 2 plugin that wraps cmd.
func NewCliAdapter(name, description, cmd string, args ...string) *CliAdapter {
	return &CliAdapter{name: name, desc: description, cmd: cmd, args: args}
}

// NewCliAdapterFromSidecar loads a CliAdapter from a sidecar YAML file.
func NewCliAdapterFromSidecar(path string) (*CliAdapter, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("cli adapter sidecar: read %s: %w", path, err)
	}
	var schema SidecarSchema
	if err := yaml.Unmarshal(data, &schema); err != nil {
		return nil, fmt.Errorf("cli adapter sidecar: parse %s: %w", path, err)
	}
	if schema.Command == "" {
		return nil, fmt.Errorf("cli adapter sidecar: %s: command is required", path)
	}
	caps := []Capability{
		{Name: schema.Name, InputSchema: schema.InputSchema, OutputSchema: schema.OutputSchema},
	}
	return &CliAdapter{
		name: schema.Name,
		desc: schema.Description,
		cmd:  schema.Command,
		args: schema.Args,
		caps: caps,
	}, nil
}

func (c *CliAdapter) Name() string              { return c.name }
func (c *CliAdapter) Description() string        { return c.desc }
func (c *CliAdapter) Capabilities() []Capability { return c.caps }
func (c *CliAdapter) Close() error               { return nil }

// Execute spawns the subprocess, writes input to stdin, and streams stdout to w.
// vars is accepted for interface compatibility but is not passed to the subprocess;
// use sidecar YAML args for fixed flags.
func (c *CliAdapter) Execute(ctx context.Context, input string, _ map[string]string, w io.Writer) error {
	cmd := exec.CommandContext(ctx, c.cmd, c.args...)
	cmd.Stdin = strings.NewReader(input)
	cmd.Stdout = w
	cmd.Stderr = w
	return cmd.Run()
}
