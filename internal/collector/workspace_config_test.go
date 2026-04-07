package collector

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// withTempHome redirects os.UserHomeDir for the duration of a test by
// setting HOME to a t.TempDir(). Restores HOME on cleanup. Used so the
// workspace config tests touch a sandbox instead of the real
// ~/.config/glitch/.
func withTempHome(t *testing.T) string {
	t.Helper()
	original := os.Getenv("HOME")
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Cleanup(func() { _ = os.Setenv("HOME", original) })
	return tmp
}

func TestWorkspaceConfigPath(t *testing.T) {
	home := withTempHome(t)

	t.Run("returns path under workspaces dir", func(t *testing.T) {
		got, err := WorkspaceConfigPath("ws-1")
		if err != nil {
			t.Fatalf("WorkspaceConfigPath: %v", err)
		}
		want := filepath.Join(home, ".config", "glitch", "workspaces", "ws-1", "collectors.yaml")
		if got != want {
			t.Errorf("path = %q, want %q", got, want)
		}
	})

	t.Run("rejects empty workspace id", func(t *testing.T) {
		if _, err := WorkspaceConfigPath(""); err == nil {
			t.Error("want error for empty workspace id, got nil")
		}
		if _, err := WorkspaceConfigPath("   "); err == nil {
			t.Error("want error for whitespace workspace id, got nil")
		}
	})
}

func TestLoadWorkspaceConfig(t *testing.T) {
	withTempHome(t)

	t.Run("missing file returns defaults", func(t *testing.T) {
		cfg, err := LoadWorkspaceConfig("never-existed")
		if err != nil {
			t.Fatalf("LoadWorkspaceConfig: %v", err)
		}
		if cfg == nil {
			t.Fatal("want non-nil config")
		}
		if cfg.Elasticsearch.Address != "http://localhost:9200" {
			t.Errorf("default ES address mismatch: %q", cfg.Elasticsearch.Address)
		}
		if cfg.Model != "llama3.2" {
			t.Errorf("default model mismatch: %q", cfg.Model)
		}
		if cfg.Git.Interval != defaultGitInterval {
			t.Errorf("default git interval mismatch: %v", cfg.Git.Interval)
		}
		// Per the "collectors are opt-in" rule the user explicitly
		// flipped on, every collector defaults to disabled so a
		// fresh workspace doesn't auto-index without consent.
		if cfg.Claude.Enabled {
			t.Error("claude should default to disabled")
		}
		if cfg.Copilot.Enabled {
			t.Error("copilot should default to disabled")
		}
	})

	t.Run("parses written file", func(t *testing.T) {
		err := WriteWorkspaceConfig("ws-2", `
elasticsearch:
  address: http://localhost:9200
git:
  interval: 30s
  repos:
    - /tmp/repo1
    - /tmp/repo2
github:
  repos:
    - org/one
mattermost:
  url: https://mm.example.com
  channels:
    - dev
`)
		if err != nil {
			t.Fatalf("WriteWorkspaceConfig: %v", err)
		}

		cfg, err := LoadWorkspaceConfig("ws-2")
		if err != nil {
			t.Fatalf("LoadWorkspaceConfig: %v", err)
		}
		if len(cfg.Git.Repos) != 2 {
			t.Errorf("git repos = %d, want 2", len(cfg.Git.Repos))
		}
		if len(cfg.GitHub.Repos) != 1 || cfg.GitHub.Repos[0] != "org/one" {
			t.Errorf("github repos = %v", cfg.GitHub.Repos)
		}
		if cfg.Mattermost.URL != "https://mm.example.com" {
			t.Errorf("mattermost url mismatch: %q", cfg.Mattermost.URL)
		}
		if cfg.Git.Interval.Seconds() != 30 {
			t.Errorf("git interval mismatch: %v", cfg.Git.Interval)
		}
	})

	t.Run("malformed yaml errors", func(t *testing.T) {
		// Write invalid YAML directly so we test the load path's
		// error handling, not WriteWorkspaceConfig's validation.
		path, _ := WorkspaceConfigPath("ws-bad")
		_ = os.MkdirAll(filepath.Dir(path), 0o755)
		_ = os.WriteFile(path, []byte("not: valid: yaml: : :"), 0o644)
		_, err := LoadWorkspaceConfig("ws-bad")
		if err == nil {
			t.Error("want parse error for malformed yaml")
		}
	})

	t.Run("workspaces are isolated", func(t *testing.T) {
		_ = WriteWorkspaceConfig("ws-a", `git:
  repos:
    - /tmp/a
`)
		_ = WriteWorkspaceConfig("ws-b", `git:
  repos:
    - /tmp/b
`)
		a, _ := LoadWorkspaceConfig("ws-a")
		b, _ := LoadWorkspaceConfig("ws-b")
		if a.Git.Repos[0] != "/tmp/a" {
			t.Errorf("ws-a leaked: %v", a.Git.Repos)
		}
		if b.Git.Repos[0] != "/tmp/b" {
			t.Errorf("ws-b leaked: %v", b.Git.Repos)
		}
	})

	t.Run("env var fallback for mattermost", func(t *testing.T) {
		t.Setenv("GLITCH_MATTERMOST_URL", "https://env-mm.example.com")
		t.Setenv("GLITCH_MATTERMOST_TOKEN", "env-token")

		_ = WriteWorkspaceConfig("ws-env", `git:
  repos: []
`)
		cfg, _ := LoadWorkspaceConfig("ws-env")
		if cfg.Mattermost.URL != "https://env-mm.example.com" {
			t.Errorf("mattermost url should fall back to env, got %q", cfg.Mattermost.URL)
		}
		if cfg.Mattermost.Token != "env-token" {
			t.Errorf("mattermost token should fall back to env, got %q", cfg.Mattermost.Token)
		}
	})

	t.Run("yaml file value beats env var", func(t *testing.T) {
		t.Setenv("GLITCH_MATTERMOST_URL", "https://env-mm.example.com")
		_ = WriteWorkspaceConfig("ws-override", `mattermost:
  url: https://yaml-mm.example.com
`)
		cfg, _ := LoadWorkspaceConfig("ws-override")
		if cfg.Mattermost.URL != "https://yaml-mm.example.com" {
			t.Errorf("yaml should win over env, got %q", cfg.Mattermost.URL)
		}
	})
}

func TestEnsureWorkspaceConfig(t *testing.T) {
	withTempHome(t)

	t.Run("creates a parseable starter file", func(t *testing.T) {
		if err := EnsureWorkspaceConfig("ws-starter"); err != nil {
			t.Fatalf("EnsureWorkspaceConfig: %v", err)
		}
		path, _ := WorkspaceConfigPath("ws-starter")
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("starter file missing: %v", err)
		}
		// Round-trip: load it back and confirm defaults survived.
		cfg, err := LoadWorkspaceConfig("ws-starter")
		if err != nil {
			t.Fatalf("LoadWorkspaceConfig on starter: %v", err)
		}
		if cfg.Model == "" {
			t.Error("starter should populate model")
		}
		// Reading the file content too, just to make sure it's not empty.
		raw, _ := os.ReadFile(path)
		if !strings.Contains(string(raw), "elasticsearch:") {
			t.Error("starter missing elasticsearch section")
		}
	})

	t.Run("does not overwrite existing file", func(t *testing.T) {
		_ = WriteWorkspaceConfig("ws-keep", `git:
  repos:
    - /tmp/keep
`)
		// Ensure should be a no-op when the file already exists.
		if err := EnsureWorkspaceConfig("ws-keep"); err != nil {
			t.Fatalf("Ensure on existing: %v", err)
		}
		cfg, _ := LoadWorkspaceConfig("ws-keep")
		if len(cfg.Git.Repos) != 1 || cfg.Git.Repos[0] != "/tmp/keep" {
			t.Errorf("existing file got clobbered: %v", cfg.Git.Repos)
		}
	})
}

func TestWriteWorkspaceConfig(t *testing.T) {
	withTempHome(t)

	t.Run("rejects invalid yaml", func(t *testing.T) {
		err := WriteWorkspaceConfig("ws-bad", "this is not: : valid yaml")
		if err == nil {
			t.Error("want validation error for malformed yaml")
		}
		// File must NOT have been written.
		path, _ := WorkspaceConfigPath("ws-bad")
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Errorf("malformed yaml was written anyway: %v", err)
		}
	})

	t.Run("creates parent dir on demand", func(t *testing.T) {
		err := WriteWorkspaceConfig("ws-fresh", "git:\n  repos: []\n")
		if err != nil {
			t.Fatalf("WriteWorkspaceConfig: %v", err)
		}
		path, _ := WorkspaceConfigPath("ws-fresh")
		if _, err := os.Stat(path); err != nil {
			t.Errorf("file not written: %v", err)
		}
	})
}

func TestDeleteWorkspaceConfig(t *testing.T) {
	withTempHome(t)

	t.Run("removes file and parent dir", func(t *testing.T) {
		_ = WriteWorkspaceConfig("ws-del", "git:\n  repos: []\n")
		path, _ := WorkspaceConfigPath("ws-del")
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("setup failed: %v", err)
		}

		if err := DeleteWorkspaceConfig("ws-del"); err != nil {
			t.Fatalf("DeleteWorkspaceConfig: %v", err)
		}

		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Errorf("file still exists after delete")
		}
		// Parent dir should also be gone (it was empty).
		if _, err := os.Stat(filepath.Dir(path)); !os.IsNotExist(err) {
			t.Errorf("workspace dir still exists after delete")
		}
	})

	t.Run("missing file is not an error", func(t *testing.T) {
		if err := DeleteWorkspaceConfig("never-existed"); err != nil {
			t.Errorf("delete missing should be idempotent, got: %v", err)
		}
	})
}
