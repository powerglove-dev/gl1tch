package collector

import (
	"os"
	"path/filepath"
	"testing"
)

// TestAutoDetectFromWorkspace locks the do-what-I-mean overlay's
// contract: each detection rule fires when its evidence exists,
// existing manual config is preserved (additive only), and the
// helper degrades to a no-op when fed nothing.
func TestAutoDetectFromWorkspace(t *testing.T) {
	t.Run("nil cfg is a no-op", func(t *testing.T) {
		// Should not panic.
		_ = AutoDetectFromWorkspace(nil, []string{"/tmp"})
	})

	t.Run("empty dirs and empty home leave config alone", func(t *testing.T) {
		// Sandbox HOME so user-level checks can't trip on the real
		// developer machine running the tests.
		t.Setenv("HOME", t.TempDir())
		cfg := &Config{}
		AutoDetectFromWorkspace(cfg, nil)
		if cfg.Claude.Enabled {
			t.Error("claude should stay disabled with no evidence")
		}
		if cfg.Copilot.Enabled {
			t.Error("copilot should stay disabled with no evidence")
		}
		if len(cfg.Git.Repos) != 0 {
			t.Errorf("git repos should stay empty, got %v", cfg.Git.Repos)
		}
	})

	t.Run("git presence enables git collector", func(t *testing.T) {
		t.Setenv("HOME", t.TempDir())
		repo := mkGitRepo(t, "")
		cfg := &Config{}
		AutoDetectFromWorkspace(cfg, []string{repo})
		if len(cfg.Git.Repos) != 1 || cfg.Git.Repos[0] != repo {
			t.Errorf("git repos = %v, want [%s]", cfg.Git.Repos, repo)
		}
	})

	t.Run("github origin enables github collector", func(t *testing.T) {
		t.Setenv("HOME", t.TempDir())
		repo := mkGitRepo(t, "https://github.com/elastic/ensemble.git")
		cfg := &Config{}
		AutoDetectFromWorkspace(cfg, []string{repo})
		if len(cfg.GitHub.Repos) != 1 || cfg.GitHub.Repos[0] != "elastic/ensemble" {
			t.Errorf("github repos = %v, want [elastic/ensemble]", cfg.GitHub.Repos)
		}
	})

	t.Run("github ssh origin parses correctly", func(t *testing.T) {
		t.Setenv("HOME", t.TempDir())
		repo := mkGitRepo(t, "git@github.com:8op-org/gl1tch.git")
		cfg := &Config{}
		AutoDetectFromWorkspace(cfg, []string{repo})
		if len(cfg.GitHub.Repos) != 1 || cfg.GitHub.Repos[0] != "8op-org/gl1tch" {
			t.Errorf("github repos = %v, want [8op-org/gl1tch]", cfg.GitHub.Repos)
		}
	})

	t.Run("non-github origin is ignored", func(t *testing.T) {
		t.Setenv("HOME", t.TempDir())
		repo := mkGitRepo(t, "https://gitlab.com/foo/bar.git")
		cfg := &Config{}
		AutoDetectFromWorkspace(cfg, []string{repo})
		if len(cfg.Git.Repos) != 1 {
			t.Errorf("git should still be detected, got %v", cfg.Git.Repos)
		}
		if len(cfg.GitHub.Repos) != 0 {
			t.Errorf("github should not be detected for gitlab origin, got %v", cfg.GitHub.Repos)
		}
	})

	t.Run("home .claude enables claude", func(t *testing.T) {
		home := t.TempDir()
		t.Setenv("HOME", home)
		_ = os.MkdirAll(filepath.Join(home, ".claude"), 0o755)
		cfg := &Config{}
		AutoDetectFromWorkspace(cfg, nil)
		if !cfg.Claude.Enabled {
			t.Error("claude should be enabled when ~/.claude exists")
		}
	})

	t.Run("project .claude enables claude even without home", func(t *testing.T) {
		t.Setenv("HOME", t.TempDir())
		project := t.TempDir()
		_ = os.MkdirAll(filepath.Join(project, ".claude"), 0o755)
		cfg := &Config{}
		AutoDetectFromWorkspace(cfg, []string{project})
		if !cfg.Claude.Enabled {
			t.Error("claude should be enabled when project has .claude")
		}
	})

	t.Run("home .copilot enables copilot", func(t *testing.T) {
		home := t.TempDir()
		t.Setenv("HOME", home)
		_ = os.MkdirAll(filepath.Join(home, ".copilot"), 0o755)
		cfg := &Config{}
		AutoDetectFromWorkspace(cfg, nil)
		if !cfg.Copilot.Enabled {
			t.Error("copilot should be enabled when ~/.copilot exists")
		}
	})

	t.Run("does not duplicate manually configured repos", func(t *testing.T) {
		t.Setenv("HOME", t.TempDir())
		repo := mkGitRepo(t, "https://github.com/elastic/ensemble.git")
		cfg := &Config{}
		// User manually wrote the same repo in their config.
		cfg.Git.Repos = []string{repo}
		cfg.GitHub.Repos = []string{"elastic/ensemble"}
		AutoDetectFromWorkspace(cfg, []string{repo})
		if len(cfg.Git.Repos) != 1 {
			t.Errorf("git repos duplicated: %v", cfg.Git.Repos)
		}
		if len(cfg.GitHub.Repos) != 1 {
			t.Errorf("github repos duplicated: %v", cfg.GitHub.Repos)
		}
	})

	t.Run("multiple workspace dirs are all detected", func(t *testing.T) {
		t.Setenv("HOME", t.TempDir())
		a := mkGitRepo(t, "https://github.com/o/a.git")
		b := mkGitRepo(t, "https://github.com/o/b.git")
		notRepo := t.TempDir()
		cfg := &Config{}
		AutoDetectFromWorkspace(cfg, []string{a, b, notRepo})
		if len(cfg.Git.Repos) != 2 {
			t.Errorf("expected 2 git repos, got %v", cfg.Git.Repos)
		}
		if len(cfg.GitHub.Repos) != 2 {
			t.Errorf("expected 2 github repos, got %v", cfg.GitHub.Repos)
		}
	})

	t.Run("non-git directories are silently skipped", func(t *testing.T) {
		t.Setenv("HOME", t.TempDir())
		notRepo := t.TempDir()
		cfg := &Config{}
		AutoDetectFromWorkspace(cfg, []string{notRepo})
		if len(cfg.Git.Repos) != 0 {
			t.Errorf("non-git dir was detected as git: %v", cfg.Git.Repos)
		}
	})
}

// mkGitRepo creates a temp directory with a fake .git layout that
// IsGitRepo recognizes. When origin is non-empty, .git/config is
// populated with that URL so GitHubRepoSlug can extract a slug.
func mkGitRepo(t *testing.T, origin string) string {
	t.Helper()
	dir := t.TempDir()
	gitDir := filepath.Join(dir, ".git")
	if err := os.MkdirAll(gitDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if origin != "" {
		cfg := `[core]
	repositoryformatversion = 0
[remote "origin"]
	url = ` + origin + `
	fetch = +refs/heads/*:refs/remotes/origin/*
`
		if err := os.WriteFile(filepath.Join(gitDir, "config"), []byte(cfg), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}
