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
	defaultDirectoriesInterval = 120 * time.Second
	defaultCodeIndexInterval   = 30 * time.Minute
	defaultModel               = "llama3.2"
)

// Config holds the collector configuration, loaded from ~/.config/glitch/observer.yaml.
//
// Both yaml and json tags are present on every field. The yaml tags
// drive the on-disk file format; the json tags are read by the
// desktop's structured collector-config modal, which round-trips this
// struct as JSON via GetCollectorsConfigJSON / WriteCollectorsConfigJSON.
// The two tag sets MUST stay in sync — the schema in the frontend
// (collectorSchema.ts) addresses fields by their lowercase JSON path
// (e.g. "code_index.embed_provider"), so a missing or mismatched tag
// silently makes that field unreachable in the modal.
type Config struct {
	Elasticsearch struct {
		Address string `yaml:"address" json:"address"` // default: http://localhost:9200
	} `yaml:"elasticsearch" json:"elasticsearch"`

	Git struct {
		Repos    []string      `yaml:"repos" json:"repos"`       // absolute paths to watch
		Interval time.Duration `yaml:"interval" json:"interval"` // poll interval (default 60s)
	} `yaml:"git" json:"git"`

	Claude struct {
		Enabled  bool          `yaml:"enabled" json:"enabled"`   // index Claude Code conversations
		Interval time.Duration `yaml:"interval" json:"interval"` // poll interval (default 120s)
	} `yaml:"claude" json:"claude"`

	Copilot struct {
		Enabled  bool          `yaml:"enabled" json:"enabled"`   // index Copilot CLI history + logs
		Interval time.Duration `yaml:"interval" json:"interval"` // poll interval (default 120s)
	} `yaml:"copilot" json:"copilot"`

	GitHub struct {
		Repos    []string      `yaml:"repos" json:"repos"`       // "owner/repo" format
		Interval time.Duration `yaml:"interval" json:"interval"` // poll interval (default 300s)
	} `yaml:"github" json:"github"`

	// Directories are project directories to scan for agents, skills,
	// provider configs, and project structure. Added via the desktop GUI
	// or manually in this config.
	Directories struct {
		Paths    []string      `yaml:"paths" json:"paths"`       // absolute paths to scan
		Interval time.Duration `yaml:"interval" json:"interval"` // re-scan interval (default 120s)
	} `yaml:"directories" json:"directories"`

	// CodeIndex is the optional semantic code-search collector. When
	// enabled it walks Paths on Interval, chunks every file matching
	// Extensions, embeds each chunk via the configured embedder, and
	// upserts the result into the glitch-vectors index. Each root path
	// gets its own RAG scope so brain queries can target one tree.
	//
	// Disabled by default — embedding a large tree on first run can
	// take several minutes and pin the local Ollama, so users must
	// explicitly opt in by setting Enabled and adding paths.
	CodeIndex struct {
		Enabled       bool          `yaml:"enabled" json:"enabled"`               // opt-in
		Paths         []string      `yaml:"paths" json:"paths"`                   // absolute paths to index
		Extensions    []string      `yaml:"extensions" json:"extensions"`         // default: .go .ts .py .md
		ChunkSize     int           `yaml:"chunk_size" json:"chunk_size"`         // chars per chunk (default 1500)
		Interval      time.Duration `yaml:"interval" json:"interval"`             // re-index interval (default 30m)
		EmbedProvider string        `yaml:"embed_provider" json:"embed_provider"` // ollama|openai|voyage (default ollama)
		EmbedModel    string        `yaml:"embed_model" json:"embed_model"`       // provider-specific model name
		EmbedBaseURL  string        `yaml:"embed_base_url" json:"embed_base_url"` // ollama base URL override
		EmbedAPIKey   string        `yaml:"embed_api_key" json:"embed_api_key"`   // literal or "$ENV_VAR"
	} `yaml:"code_index" json:"code_index"`

	// Model is the Ollama model used for query generation and synthesis.
	Model string `yaml:"model" json:"model"` // default: llama3.2
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

# Directories to scan for agents, skills, provider configs, and project structure.
# Added via the desktop GUI or manually here.
directories:
  interval: 120s
  paths: []
  # - %s/Projects/gl1tch
  # - %s/Projects/my-other-project

# Semantic code indexing — walks each path, chunks source files, and stores
# embeddings in glitch-vectors so the brain can answer "where is this logic"
# style questions. Disabled by default; embedding a large tree on first run
# can take several minutes.
code_index:
  enabled: false
  interval: 30m
  paths: []
  # - %s/Projects/gl1tch
  extensions: [.go, .ts, .py, .md]
  chunk_size: 1500
  embed_provider: ollama
  embed_model: nomic-embed-text
  embed_base_url: http://localhost:11434
`, home, home, home, home, home)

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(defaultCfg), 0o644)
}
