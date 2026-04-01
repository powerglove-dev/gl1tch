package executor

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	"gopkg.in/yaml.v3"
)

// SidecarModel is a single model entry declared in a sidecar YAML.
type SidecarModel struct {
	ID    string `yaml:"id"`
	Label string `yaml:"label"`
}

// SidecarSchema is the structure of a ~/.config/glitch/wrappers/<name>.yaml file.
type SidecarSchema struct {
	Name         string         `yaml:"name"`
	Description  string         `yaml:"description"`
	Command      string         `yaml:"command"`
	Args         []string       `yaml:"args"`
	Models       []SidecarModel `yaml:"models"`
	InputSchema  string         `yaml:"input_schema"`
	OutputSchema string         `yaml:"output_schema"`
	// Category is an optional hierarchical prefix (e.g. "providers.claude").
	// When set, the adapter is also registered under "category.name" in the Manager.
	Category string `yaml:"category"`
	// Kind categorises the executor. Valid values: "agent" (default), "tool".
	// Executors without a kind field default to "agent" for backwards compatibility.
	Kind string `yaml:"kind"`
}

// CliAdapter wraps an arbitrary CLI tool as a Tier 2 Executor.
// Input is written to the subprocess stdin; stdout/stderr is streamed to the writer.
// args are fixed command-line arguments prepended to every Execute call.
type CliAdapter struct {
	name     string
	desc     string
	cmd      string
	args     []string
	models   []SidecarModel
	caps     []Capability
	category string // optional; set from sidecar YAML
	kind     string // "agent" or "tool"; defaults to "agent"
}

// NewCliAdapter creates a Tier 2 executor that wraps cmd.
func NewCliAdapter(name, description, cmd string, args ...string) *CliAdapter {
	return &CliAdapter{name: name, desc: description, cmd: cmd, args: args}
}

// NewCliAdapterFromSidecar loads a CliAdapter from a sidecar YAML file.
// If the sidecar has a category field, Category is set on the adapter so that
// callers (e.g. Manager.LoadWrappersFromDir) can call RegisterCategory.
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
	models := schema.Models
	if models == nil {
		models = []SidecarModel{}
	}
	kind := schema.Kind
	if kind == "" {
		kind = "agent"
	}
	return &CliAdapter{
		name:     schema.Name,
		desc:     schema.Description,
		cmd:      schema.Command,
		args:     schema.Args,
		models:   models,
		caps:     caps,
		category: schema.Category,
		kind:     kind,
	}, nil
}

func (c *CliAdapter) Name() string              { return c.name }
func (c *CliAdapter) Description() string        { return c.desc }
func (c *CliAdapter) Capabilities() []Capability { return c.caps }
func (c *CliAdapter) Close() error               { return nil }
func (c *CliAdapter) Command() string            { return c.cmd }

// Category returns the optional hierarchical category prefix. Empty if not set.
func (c *CliAdapter) Category() string { return c.category }

// Kind returns the executor kind ("agent" or "tool"). Never empty; defaults to "agent".
func (c *CliAdapter) Kind() string { return c.kind }

// Models returns the models declared in the sidecar YAML. Never nil.
func (c *CliAdapter) Models() []SidecarModel { return c.models }

// Execute spawns the subprocess, writes input to stdin, and streams stdout to w.
// All entries in vars are passed as GLITCH_<KEY>=<value> environment variables so
// that any sidecar binary or shell command can read them without special-casing.
// Additionally, if vars contains a non-empty "model" key, "--model <value>" is
// appended to the command arguments for backwards compatibility with AI provider
// CLIs (e.g. claude, opencode) that accept the model as a flag.
func (c *CliAdapter) Execute(ctx context.Context, input string, vars map[string]string, w io.Writer) error {
	args := c.args
	if model := vars["model"]; model != "" {
		args = append(append([]string{}, args...), "--model", model)
	}
	cmd := exec.CommandContext(ctx, c.cmd, args...)
	cmd.Stdin = strings.NewReader(input)
	cmd.Stdout = w
	cmd.Stderr = w

	// Set the working directory if provided.
	if cwd := vars["cwd"]; cwd != "" {
		cmd.Dir = cwd
	}

	// Inherit the current environment then overlay GLITCH_* vars.
	env := os.Environ()
	for k, v := range vars {
		key := "GLITCH_" + strings.ToUpper(k)
		env = append(env, key+"="+v)
	}
	cmd.Env = env

	return cmd.Run()
}
