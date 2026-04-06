package collector

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v3"
)

// Config holds the collector configuration, loaded from ~/.config/glitch/observer.yaml.
type Config struct {
	Elasticsearch struct {
		Address string `yaml:"address"` // default: http://localhost:9200
	} `yaml:"elasticsearch"`

	Git struct {
		Repos    []string      `yaml:"repos"`    // absolute paths to watch
		Interval time.Duration `yaml:"interval"` // poll interval (default 60s)
	} `yaml:"git"`

	Claude struct {
		Enabled  bool          `yaml:"enabled"`  // index Claude Code conversations
		Interval time.Duration `yaml:"interval"` // poll interval (default 120s)
	} `yaml:"claude"`

	Copilot struct {
		Enabled  bool          `yaml:"enabled"`  // index Copilot CLI history + logs
		Interval time.Duration `yaml:"interval"` // poll interval (default 120s)
	} `yaml:"copilot"`

	GitHub struct {
		Repos    []string      `yaml:"repos"`    // "owner/repo" format
		Interval time.Duration `yaml:"interval"` // poll interval (default 300s)
	} `yaml:"github"`

	// Model is the Ollama model used for query generation and synthesis.
	Model string `yaml:"model"` // default: llama3.2
}

// DefaultConfigPath returns ~/.config/glitch/observer.yaml.
func DefaultConfigPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "glitch", "observer.yaml"), nil
}

// LoadConfig reads the observer config. If the file doesn't exist, returns defaults.
func LoadConfig() (*Config, error) {
	path, err := DefaultConfigPath()
	if err != nil {
		return nil, err
	}

	cfg := &Config{}
	cfg.Elasticsearch.Address = "http://localhost:9200"
	cfg.Git.Interval = 60 * time.Second
	cfg.Claude.Enabled = true
	cfg.Claude.Interval = 120 * time.Second
	cfg.Copilot.Enabled = true
	cfg.Copilot.Interval = 120 * time.Second
	cfg.GitHub.Interval = 300 * time.Second
	cfg.Model = "llama3.2"

	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return cfg, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read observer config: %w", err)
	}

	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parse observer config: %w", err)
	}

	// Expand ~ in repo paths.
	home, _ := os.UserHomeDir()
	for i, r := range cfg.Git.Repos {
		if len(r) > 0 && r[0] == '~' {
			cfg.Git.Repos[i] = filepath.Join(home, r[1:])
		}
	}

	return cfg, nil
}

// EnsureDefaultConfig writes a starter observer.yaml if none exists.
func EnsureDefaultConfig() error {
	path, err := DefaultConfigPath()
	if err != nil {
		return err
	}

	if _, err := os.Stat(path); err == nil {
		return nil // already exists
	}

	home, _ := os.UserHomeDir()
	defaultCfg := fmt.Sprintf(`# gl1tch observer configuration
# All your dev activity, indexed and searchable.

elasticsearch:
  address: "http://localhost:9200"

# Ollama model for query generation and answer synthesis.
model: "llama3.2"

# Git repos to watch for new commits.
git:
  interval: 60s
  repos:
    - %s/Projects/gl1tch
    # - %s/Projects/your-other-repo

# Claude Code conversation indexing (~/.claude/history.jsonl + project sessions).
claude:
  enabled: true
  interval: 120s

# GitHub Copilot CLI history and logs (~/.copilot/).
copilot:
  enabled: true
  interval: 120s

# GitHub PRs and issues (requires gh CLI, authenticated).
github:
  interval: 300s
  repos: []
  # - elastic/ensemble
  # - 8op-org/gl1tch
`, home, home)

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(defaultCfg), 0o644)
}
