// workspace_config.go is the per-workspace counterpart to config.go.
//
// Each workspace has its own collectors.yaml at
// ~/.config/glitch/workspaces/<id>/collectors.yaml. The schema is the
// same Config struct used by the global observer.yaml — same fields,
// same defaults — so loaders, editors, and tests can use one type
// across both surfaces.
//
// The split lets each workspace declare its own git repos, mattermost
// channels, github org, etc. independently. The supervisor's pod
// manager (Phase 4) loads one config per active workspace and runs a
// dedicated set of collector goroutines for each.
//
// Behavior on the global config:
//
//   - The global ~/.config/glitch/observer.yaml file still exists
//   - It is read-only by default; the desktop popup edits per-workspace
//     files and only force-overwrites global when the user explicitly
//     opts in
//   - Per the "everything is workspace-scoped unless noted" rule, the
//     global file is for fallback defaults / always-on collectors —
//     not for primary day-to-day config edits
package collector

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// WorkspaceConfigPath returns the absolute path of the collectors.yaml
// file for the given workspace id. The directory is not created — use
// EnsureWorkspaceConfig if you need the file to exist.
//
// Returns an error when workspaceID is empty (callers should fall back
// to the global DefaultConfigPath in that case) or when the home dir
// cannot be resolved.
func WorkspaceConfigPath(workspaceID string) (string, error) {
	if strings.TrimSpace(workspaceID) == "" {
		return "", fmt.Errorf("workspace id is required")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "glitch", "workspaces", workspaceID, "collectors.yaml"), nil
}

// LoadWorkspaceConfig reads ~/.config/glitch/workspaces/<id>/collectors.yaml
// and returns the parsed Config. Missing files return the same default
// Config that LoadConfig produces — empty repos lists, sensible
// intervals — so a brand-new workspace can start collecting nothing
// without errors and the user can add sources via the popup.
//
// Same env-var fallbacks as LoadConfig: GLITCH_MATTERMOST_URL /
// GLITCH_MATTERMOST_TOKEN apply to per-workspace configs too. The home
// dir tilde expansion in Git.Repos is also applied.
func LoadWorkspaceConfig(workspaceID string) (*Config, error) {
	path, err := WorkspaceConfigPath(workspaceID)
	if err != nil {
		return nil, err
	}
	return loadConfigFromPath(path)
}

// loadConfigFromPath is the shared parser used by both LoadConfig
// (global) and LoadWorkspaceConfig (per-workspace). Pulled out so
// the two paths can't drift on defaults, env fallbacks, or tilde
// expansion.
func loadConfigFromPath(path string) (*Config, error) {
	cfg := defaultConfig()

	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return cfg, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read collector config: %w", err)
	}

	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parse collector config: %w", err)
	}

	if cfg.Mattermost.URL == "" {
		cfg.Mattermost.URL = os.Getenv("GLITCH_MATTERMOST_URL")
	}
	if cfg.Mattermost.Token == "" {
		cfg.Mattermost.Token = os.Getenv("GLITCH_MATTERMOST_TOKEN")
	}

	home, _ := os.UserHomeDir()
	for i, r := range cfg.Git.Repos {
		if len(r) > 0 && r[0] == '~' {
			cfg.Git.Repos[i] = filepath.Join(home, r[1:])
		}
	}

	return cfg, nil
}

// defaultConfig returns a Config populated with the in-memory
// fallback values used when no YAML file is present. Per the
// "collectors should be opt-in" rule every collector defaults to
// disabled — the user has to explicitly populate a repo list or
// flip an enabled flag to start collecting. Intervals still get
// sane defaults so the user doesn't have to type them when they
// do enable a section.
func defaultConfig() *Config {
	cfg := &Config{}
	cfg.Elasticsearch.Address = "http://localhost:9200"
	cfg.Git.Interval = defaultGitInterval
	cfg.Claude.Enabled = false
	cfg.Claude.Interval = defaultClaudeInterval
	cfg.Copilot.Enabled = false
	cfg.Copilot.Interval = defaultCopilotInterval
	cfg.GitHub.Interval = defaultGitHubInterval
	cfg.Mattermost.Interval = defaultMattermostInterval
	cfg.Directories.Interval = defaultDirectoriesInterval
	cfg.CodeIndex.Enabled = false
	cfg.CodeIndex.Interval = defaultCodeIndexInterval
	cfg.CodeIndex.ChunkSize = 1500
	cfg.CodeIndex.EmbedProvider = "ollama"
	cfg.CodeIndex.EmbedModel = "nomic-embed-text"
	cfg.CodeIndex.EmbedBaseURL = "http://localhost:11434"
	cfg.CodeIndex.Extensions = []string{".go", ".ts", ".py", ".md"}
	cfg.Model = defaultModel
	return cfg
}

// EnsureWorkspaceConfig writes a starter collectors.yaml for the
// workspace if one doesn't exist. The starter file is intentionally
// minimal: a comment header explaining what to add, plus the
// elasticsearch address (which the workspace inherits from the
// system rather than re-discovering it). The user fills in repos,
// channels, etc. via the editor popup.
//
// Returns nil on success or if the file already exists.
func EnsureWorkspaceConfig(workspaceID string) error {
	path, err := WorkspaceConfigPath(workspaceID)
	if err != nil {
		return err
	}
	if _, err := os.Stat(path); err == nil {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	starter := fmt.Sprintf(`# gl1tch workspace collectors
#
# This file is per-workspace; gl1tch maintains one of these for every
# workspace you create. Edit it via the desktop's "configure collectors"
# popup or directly here.
#
# All collectors are DISABLED by default. Enable each one explicitly
# by populating its section (add repos, set enabled: true, etc.) so
# fresh workspaces never auto-index without you opting in.

elasticsearch:
  address: "http://localhost:9200"

# Ollama model used for query/synthesis when this workspace is active.
model: "%s"

git:
  interval: 60s
  repos: []

claude:
  enabled: false
  interval: 120s

copilot:
  enabled: false
  interval: 120s

github:
  interval: 300s
  repos: []

mattermost:
  interval: 60s
  channels: []

directories:
  interval: 120s
  paths: []

code_index:
  enabled: false
  interval: 30m
  paths: []
  extensions: [.go, .ts, .py, .md]
  chunk_size: 1500
  embed_provider: ollama
  embed_model: nomic-embed-text
  embed_base_url: http://localhost:11434
`, defaultModel)
	return os.WriteFile(path, []byte(starter), 0o644)
}

// WriteWorkspaceConfig validates and writes new content to a workspace's
// collectors.yaml. Validation parses the YAML into the same Config
// struct collectors load at runtime; if parsing fails the file is NOT
// written so a typo can never corrupt a running workspace's config.
//
// Returns nil on success. On parse failure returns the underlying
// yaml error so the editor popup can render it inline.
func WriteWorkspaceConfig(workspaceID, content string) error {
	var probe Config
	if err := yaml.Unmarshal([]byte(content), &probe); err != nil {
		return fmt.Errorf("invalid yaml: %w", err)
	}
	path, err := WorkspaceConfigPath(workspaceID)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(content), 0o644)
}

// DeleteWorkspaceConfig removes a workspace's collectors.yaml file
// and the enclosing workspaces/<id> directory if it ends up empty.
// Used when a workspace is deleted from the desktop so its collector
// config doesn't linger as orphaned config noise.
//
// Idempotent: missing files are not an error.
func DeleteWorkspaceConfig(workspaceID string) error {
	path, err := WorkspaceConfigPath(workspaceID)
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	// Best-effort cleanup of the parent dir. We ignore the error
	// (which fires when the dir is non-empty) because that case is
	// fine — there might be future per-workspace state living next
	// to collectors.yaml.
	_ = os.Remove(filepath.Dir(path))
	return nil
}
