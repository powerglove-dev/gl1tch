// Package providers manages bundled and user-defined AI provider profiles for
// the orcai plugin system. Profiles describe how to launch an AI CLI tool as a
// tmux session, which models it supports, and how billing is tracked.
package providers

// Profile describes a single AI provider integration.
type Profile struct {
	Name        string         `yaml:"name"`
	Binary      string         `yaml:"binary"`
	DisplayName string         `yaml:"display_name"`
	APIKeyEnv   string         `yaml:"api_key_env"`
	Models      []Model        `yaml:"models"`
	Session     SessionConfig  `yaml:"session"`
	Pipeline    PipelineConfig `yaml:"pipeline"`
}

// Model describes a single model offered by a provider, including its billing rates.
type Model struct {
	ID              string  `yaml:"id"`
	Display         string  `yaml:"display"`
	CostInputPer1M  float64 `yaml:"cost_input_per_1m"`
	CostOutputPer1M float64 `yaml:"cost_output_per_1m"`
}

// SessionConfig describes how to launch a provider binary inside a tmux window.
// Env values that are empty strings are treated as pass-through from the host
// environment (the variable is not explicitly set in the child process).
type SessionConfig struct {
	WindowName string            `yaml:"window_name"`
	LaunchArgs []string          `yaml:"launch_args"`
	Env        map[string]string `yaml:"env"`
}

// PipelineConfig describes how to invoke a provider binary as a non-interactive
// pipeline executor (i.e. without a TTY or tmux session).
type PipelineConfig struct {
	// Args are prepended to the CLI invocation when the provider is used as a
	// pipeline step executor. Use this to switch the binary into a headless
	// mode (e.g. ["run"] for opencode).
	Args []string `yaml:"args"`
}
