// autodetect.go applies a "do what I mean" overlay to a loaded
// workspace config: any tooling we can detect from the workspace's
// directories (or from the user's home dir for user-level tools)
// gets enabled automatically without the user having to flip flags
// in collectors.yaml.
//
// Detection rules:
//
//   git    — any workspace dir that contains .git becomes a Git.Repos entry
//   github — any of those that has a github.com origin becomes a GitHub.Repos entry
//   claude — enable if ~/.claude/ exists OR any workspace dir contains .claude/
//   copilot— enable if ~/.copilot/ exists OR any workspace dir contains .copilot/
//
// The overlay is purely additive: it never removes anything the user
// explicitly set in collectors.yaml. If the user has manually disabled
// claude in their config and a .claude/ exists, the explicit disable
// loses (auto-detect re-enables) — which is intentional, because
// "disabled when the tool is clearly being used" is almost always a
// stale config rather than a deliberate opt-out. The user can opt out
// for real by removing the directory or by deleting the workspace.
//
// Future tools (.codex, .aider, etc.) plug in here once their
// collector implementations exist. For now we just enable the four
// collectors that ship in this binary.
package collector

import (
	"os"
	"path/filepath"
	"strings"
)

// AutoDetectFromWorkspace mutates cfg to enable any collectors that
// the workspace's directories or the user's home dir provide
// evidence for. Returns the same cfg pointer for fluent use.
//
// Safe to call with a nil cfg or empty dirs — both no-op.
func AutoDetectFromWorkspace(cfg *Config, dirs []string) *Config {
	if cfg == nil {
		return cfg
	}

	home, _ := os.UserHomeDir()

	// ── git + github ────────────────────────────────────────────────
	// Walk every workspace dir; any .git presence enables git, and
	// a github origin URL further enables github. We dedupe against
	// the existing config so manual entries aren't duplicated.
	existingGit := stringSet(cfg.Git.Repos)
	existingGitHub := stringSet(cfg.GitHub.Repos)
	for _, d := range dirs {
		if d == "" {
			continue
		}
		if !IsGitRepo(d) {
			continue
		}
		if !existingGit[d] {
			cfg.Git.Repos = append(cfg.Git.Repos, d)
			existingGit[d] = true
		}
		if slug := GitHubRepoSlug(d); slug != "" && !existingGitHub[slug] {
			cfg.GitHub.Repos = append(cfg.GitHub.Repos, slug)
			existingGitHub[slug] = true
		}
	}

	// ── claude ──────────────────────────────────────────────────────
	// User-level: ~/.claude/ exists.
	// Project-level: any workspace dir contains a .claude/ subdir.
	if hasToolingDir(home, dirs, ".claude") {
		cfg.Claude.Enabled = true
	}

	// ── copilot ─────────────────────────────────────────────────────
	if hasToolingDir(home, dirs, ".copilot") {
		cfg.Copilot.Enabled = true
	}

	return cfg
}

// hasToolingDir returns true when home/<name> exists OR when any of
// the given dirs contains a <name>/ subdirectory. Used as the
// detection signal for user-level tools that ship a config dir.
//
// We check both layers because:
//
//   - Some tools live entirely under $HOME (claude history, copilot
//     log dir) and a project might have no per-dir marker
//   - Some tools have per-project markers but no global state (e.g.
//     .claude/skills/ inside a project, no global ~/.claude needed)
//
// Either signal is enough to enable the corresponding collector —
// the false-positive cost is "we run an empty collector poll" which
// is harmless.
func hasToolingDir(home string, dirs []string, name string) bool {
	if home != "" {
		if info, err := os.Stat(filepath.Join(home, name)); err == nil && info.IsDir() {
			return true
		}
	}
	for _, d := range dirs {
		if d == "" {
			continue
		}
		if info, err := os.Stat(filepath.Join(d, name)); err == nil && info.IsDir() {
			return true
		}
	}
	return false
}

// stringSet is a tiny helper to dedupe slices. Used by the auto-
// detect overlay so manually configured repos don't get duplicated
// when auto-detection re-discovers them.
func stringSet(items []string) map[string]bool {
	out := make(map[string]bool, len(items))
	for _, s := range items {
		out[s] = true
	}
	return out
}

// IsGitRepo reports whether path contains a .git directory or file
// (the latter for worktrees and submodules), or is itself a bare
// repo (HEAD file at the root). Loose by design — the git collector
// can handle any of these layouts.
//
// Exported for the pkg/glitchd brain helpers and the pod manager's
// auto-detect overlay; both want the same answer for "is this a
// git checkout?".
func IsGitRepo(path string) bool {
	if path == "" {
		return false
	}
	if _, err := os.Stat(filepath.Join(path, ".git")); err == nil {
		return true
	}
	if info, err := os.Stat(filepath.Join(path, "HEAD")); err == nil && !info.IsDir() {
		return true // bare repo
	}
	return false
}

// GitHubRepoSlug extracts "owner/repo" from the git remote origin
// of the given directory. Returns "" if the dir isn't a git repo,
// has no origin, or the origin isn't on github.com.
//
// Parses .git/config directly so it works in sandboxed builds
// without git on PATH.
func GitHubRepoSlug(dir string) string {
	gitDir := filepath.Join(dir, ".git")
	// .git can be a regular dir (normal repo) or a file pointing at
	// the real gitdir (worktrees / submodules).
	if info, err := os.Stat(gitDir); err == nil && !info.IsDir() {
		b, err := os.ReadFile(gitDir)
		if err == nil {
			line := strings.TrimSpace(string(b))
			if strings.HasPrefix(line, "gitdir:") {
				gitDir = strings.TrimSpace(strings.TrimPrefix(line, "gitdir:"))
				if !filepath.IsAbs(gitDir) {
					gitDir = filepath.Join(dir, gitDir)
				}
			}
		}
	}
	cfgPath := filepath.Join(gitDir, "config")
	raw, err := os.ReadFile(cfgPath)
	if err != nil {
		return ""
	}

	// Extremely targeted parse: find [remote "origin"] section and
	// grab its url = line. Good enough for 99% of configs without
	// pulling in a real ini parser.
	lines := strings.Split(string(raw), "\n")
	inOrigin := false
	var originURL string
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "[remote ") {
			inOrigin = strings.Contains(trimmed, `"origin"`)
			continue
		}
		if strings.HasPrefix(trimmed, "[") {
			inOrigin = false
			continue
		}
		if inOrigin && strings.HasPrefix(trimmed, "url") {
			if eq := strings.Index(trimmed, "="); eq >= 0 {
				originURL = strings.TrimSpace(trimmed[eq+1:])
				break
			}
		}
	}
	if originURL == "" {
		return ""
	}

	// Accept https://github.com/owner/repo(.git)? and
	// git@github.com:owner/repo(.git)?.
	var slug string
	switch {
	case strings.Contains(originURL, "github.com/"):
		i := strings.Index(originURL, "github.com/")
		slug = originURL[i+len("github.com/"):]
	case strings.Contains(originURL, "github.com:"):
		i := strings.Index(originURL, "github.com:")
		slug = originURL[i+len("github.com:"):]
	default:
		return ""
	}
	slug = strings.TrimSuffix(slug, ".git")
	slug = strings.TrimSuffix(slug, "/")
	if strings.Count(slug, "/") != 1 || strings.ContainsAny(slug, " \t") {
		return ""
	}
	return slug
}
