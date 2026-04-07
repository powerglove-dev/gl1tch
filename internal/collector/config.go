package collector

import (
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// Default poll intervals shared by both the global LoadConfig path
// and the per-workspace LoadWorkspaceConfig path. Centralised here
// so the two loaders can never drift on what "default" means.
const (
	defaultGitInterval         = 60 * time.Second
	defaultClaudeInterval      = 120 * time.Second
	defaultCopilotInterval     = 120 * time.Second
	defaultGitHubInterval      = 300 * time.Second
	defaultMattermostInterval  = 60 * time.Second
	defaultDirectoriesInterval = 120 * time.Second
	defaultModel               = "llama3.2"
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

	Mattermost struct {
		URL      string        `yaml:"url"`      // server URL (or GLITCH_MATTERMOST_URL)
		Token    string        `yaml:"token"`    // bot/PAT (or GLITCH_MATTERMOST_TOKEN)
		Channels []string      `yaml:"channels"` // channel names to auto-join and monitor (empty = all)
		Interval time.Duration `yaml:"interval"` // poll interval (default 60s)
	} `yaml:"mattermost"`

	// Directories are project directories to scan for agents, skills,
	// provider configs, and project structure. Added via the desktop GUI
	// or manually in this config.
	Directories struct {
		Paths    []string      `yaml:"paths"`    // absolute paths to scan
		Interval time.Duration `yaml:"interval"` // re-scan interval (default 120s)
	} `yaml:"directories"`

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

// LoadConfig reads the global observer config. If the file doesn't
// exist, returns defaults. Per-workspace configs use
// LoadWorkspaceConfig instead.
func LoadConfig() (*Config, error) {
	path, err := DefaultConfigPath()
	if err != nil {
		return nil, err
	}
	return loadConfigFromPath(path)
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

# Mattermost channel monitoring (uses GLITCH_MATTERMOST_URL/TOKEN env vars if set).
mattermost:
  interval: 60s
  # url: "https://mattermost.example.com"
  # token: "your-bot-token"
  channels: []
  # - town-square
  # - engineering

# Directories to scan for agents, skills, provider configs, and project structure.
# Added via the desktop GUI or manually here.
directories:
  interval: 120s
  paths: []
  # - %s/Projects/gl1tch
  # - %s/Projects/my-other-project
`, home, home, home, home)

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(defaultCfg), 0o644)
}
