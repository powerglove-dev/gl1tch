// Package providers manages bundled and user-defined AI provider profiles for
// the glitch plugin system. Profiles describe how to launch an AI CLI tool as a
// tmux session, which models it supports, and how billing is tracked.
package providers

import (
	"fmt"
	"os"

	"github.com/8op-org/gl1tch/internal/brainrag"
)

// EmbedConfig describes which embedding backend to use for brainrag operations.
type EmbedConfig struct {
	Provider   string `yaml:"provider"`    // "ollama" | "openai" | "voyage"
	Model      string `yaml:"model"`
	APIKeyEnv  string `yaml:"api_key_env"`
	BaseURL    string `yaml:"base_url"`   // Ollama only
	Dimensions int    `yaml:"dimensions"` // OpenAI only
}

// NewEmbedder constructs a brainrag.Embedder from cfg.
// If cfg is zero value or Provider is "" or "ollama", an OllamaEmbedder is returned.
func NewEmbedder(cfg EmbedConfig) (brainrag.Embedder, error) {
	switch cfg.Provider {
	case "", "ollama":
		return brainrag.NewOllamaEmbedder(cfg.BaseURL, cfg.Model), nil
	case "openai":
		key := os.Getenv(cfg.APIKeyEnv)
		if key == "" {
			return nil, fmt.Errorf("providers: openai embed: env var %q is not set", cfg.APIKeyEnv)
		}
		return brainrag.NewOpenAIEmbedder(key, cfg.Model, cfg.Dimensions), nil
	case "voyage":
		key := os.Getenv(cfg.APIKeyEnv)
		if key == "" {
			return nil, fmt.Errorf("providers: voyage embed: env var %q is not set", cfg.APIKeyEnv)
		}
		return brainrag.NewVoyageEmbedder(key, cfg.Model), nil
	default:
		return nil, fmt.Errorf("providers: unknown embed provider %q", cfg.Provider)
	}
}

// Profile describes a single AI provider integration.
type Profile struct {
	Name        string         `yaml:"name"`
	Binary      string         `yaml:"binary"`
	DisplayName string         `yaml:"display_name"`
	APIKeyEnv   string         `yaml:"api_key_env"`
	Models      []Model        `yaml:"models"`
	Session     SessionConfig  `yaml:"session"`
	Pipeline    PipelineConfig `yaml:"pipeline"`
	Embed       EmbedConfig    `yaml:"embed"`
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
