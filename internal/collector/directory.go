package collector

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/8op-org/gl1tch/internal/esearch"
)

// DirectoryCollector scans configured directories for agents, skills, slash
// commands, provider metadata (CLAUDE.md, .claude/ sessions), and project
// structure. All discovered artifacts are indexed into Elasticsearch so
// gl1tch can reference them when answering questions.
//
// Runs non-blocking: initial scan is immediate, then re-scans periodically
// to pick up changes.
type DirectoryCollector struct {
	// Dirs is the list of absolute directory paths to scan.
	Dirs []string
	// Interval between re-scans. Defaults to 120s.
	Interval time.Duration
	// WorkspaceID is stamped on every indexed event so brain queries
	// can scope to one workspace's discovered skills/agents/etc.
	WorkspaceID string
}

func (d *DirectoryCollector) Name() string { return "directory" }

func (d *DirectoryCollector) Start(ctx context.Context, es *esearch.Client) error {
	if d.Interval == 0 {
		d.Interval = 120 * time.Second
	}

	slog.Info("directory collector: started",
		"workspace", d.WorkspaceID,
		"dirs", len(d.Dirs),
		"interval", d.Interval)
	for _, dir := range d.Dirs {
		slog.Debug("directory collector: watching dir",
			"workspace", d.WorkspaceID,
			"dir", dir)
	}

	if es == nil {
		err := fmt.Errorf("directory collector: elasticsearch client is nil — check ES connectivity")
		RecordRun("directories", time.Now(), 0, err)
		return err
	}

	// Track which directories we've already scanned in this run so we
	// can spot newly-added paths and pick them up without a restart.
	// The desktop's "Add directory" button writes the workspace
	// config and triggers a pod restart, so in practice known is
	// seeded fresh on every pod start; the runtime-discovery path is
	// defensive for the case where a workspace's collectors.yaml is
	// edited in place.
	known := make(map[string]bool, len(d.Dirs))

	// scanAll runs scanDirectory across the current workspace dirs.
	// Returns the total directories scanned and the last error seen.
	scanAll := func() (int, error) {
		dirs := d.currentDirs()
		var lastErr error
		newOnes := 0
		for _, dir := range dirs {
			if !known[dir] {
				known[dir] = true
				newOnes++
				slog.Info("directory collector: new directory picked up",
					"workspace", d.WorkspaceID, "dir", dir)
			}
			slog.Debug("directory collector: scanning",
				"workspace", d.WorkspaceID, "dir", dir)
			if err := d.scanDirectory(ctx, es, dir); err != nil {
				lastErr = err
				slog.Warn("directory collector: scan error",
					"workspace", d.WorkspaceID, "dir", dir, "err", err)
			}
		}
		if newOnes > 0 {
			slog.Info("directory collector: discovered new dirs",
				"workspace", d.WorkspaceID, "count", newOnes)
		}
		return len(dirs), lastErr
	}

	// Initial scan on startup so we don't wait for the first tick.
	slog.Debug("directory collector: initial scan", "workspace", d.WorkspaceID)
	if _, err := scanAll(); err != nil {
		slog.Warn("directory collector: initial scan error",
			"workspace", d.WorkspaceID, "err", err)
	}

	ticker := time.NewTicker(d.Interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			slog.Debug("directory collector: tick", "workspace", d.WorkspaceID)
			_, tickDone := startTickSpan(ctx, "directory", d.WorkspaceID)
			start := time.Now()
			n, lastErr := scanAll()
			slog.Debug("directory collector: tick done",
				"workspace", d.WorkspaceID,
				"dirs_scanned", n,
				"dur", time.Since(start))
			// Use n as the "indexed" count for the span attribute —
			// it's the number of directories scanned, which is the
			// closest per-tick volume metric the directory
			// collector produces. Real doc counts go through the
			// brain UI's ES aggregation.
			tickDone(n, lastErr)
			// Heartbeat: indexed count is 0 here because scanDirectory
			// doesn't return one. The brain UI uses ES doc counts for
			// totals; this entry just proves the collector ran.
			RecordRun("directories", start, 0, lastErr)
		}
	}
}

// currentDirs returns the directories the collector was constructed
// with. Workspace directories are now the source of truth and are
// passed in via d.Dirs at pod start time; the desktop's
// AddWorkspaceDirectory / WriteWorkspaceCollectorConfigJSON paths
// restart the pod so a fresh d.Dirs reflects any changes immediately.
//
// Historically this method also merged the global observer.yaml
// directories.paths list, which leaked dirs from one workspace into
// every other workspace's collector. That fallback was dropped along
// with the workspace-scoped collector split.
func (d *DirectoryCollector) currentDirs() []string {
	seen := make(map[string]bool, len(d.Dirs))
	out := make([]string, 0, len(d.Dirs))
	for _, p := range d.Dirs {
		if p == "" || seen[p] {
			continue
		}
		seen[p] = true
		out = append(out, p)
	}
	return out
}

func (d *DirectoryCollector) scanDirectory(ctx context.Context, es *esearch.Client, dir string) error {
	if _, err := os.Stat(dir); err != nil {
		return fmt.Errorf("directory not accessible: %w", err)
	}

	repoName := filepath.Base(dir)

	var docs []any

	// 1. Scan for skills
	docs = append(docs, d.scanSkills(dir, repoName)...)

	// 2. Scan for agents (commands)
	docs = append(docs, d.scanAgents(dir, repoName)...)

	// 3. Scan for CLAUDE.md / project instructions
	docs = append(docs, d.scanProviderMeta(dir, repoName)...)

	// 4. Scan for Claude Code project sessions
	docs = append(docs, d.scanClaudeSessions(dir, repoName)...)

	// 5. Scan project structure
	docs = append(docs, d.scanProjectStructure(dir, repoName)...)

	// 6. Detect GitHub remote and scan for repo metadata
	docs = append(docs, d.scanGitRemote(dir, repoName)...)

	if len(docs) > 0 {
		slog.Info("directory collector: indexed artifacts",
			"workspace", d.WorkspaceID,
			"dir", repoName,
			"count", len(docs))
		if err := es.BulkIndex(ctx, esearch.IndexEvents, StampWorkspaceID(d.WorkspaceID, docs)); err != nil {
			return fmt.Errorf("bulk index: %w", err)
		}
	} else {
		slog.Debug("directory collector: no artifacts",
			"workspace", d.WorkspaceID, "dir", repoName)
	}

	return nil
}

// scanSkills finds SKILL.md files in well-known skill directories.
func (d *DirectoryCollector) scanSkills(dir, repoName string) []any {
	var docs []any

	skillDirs := []string{
		filepath.Join(dir, ".claude", "skills"),
		filepath.Join(dir, "skills"),
	}

	for _, sd := range skillDirs {
		entries, err := os.ReadDir(sd)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			skillMD := filepath.Join(sd, e.Name(), "SKILL.md")
			content, err := os.ReadFile(skillMD)
			if err != nil {
				continue
			}

			desc := extractFirstDescription(string(content))
			docs = append(docs, esearch.Event{
				Type:    "directory.skill",
				Source:  "directory",
				Repo:    repoName,
				Author:  "scanner",
				Message: fmt.Sprintf("skill: %s — %s", e.Name(), desc),
				Body:    truncateStr(string(content), 4000),
				Metadata: map[string]any{
					"skill_name": e.Name(),
					"skill_path": skillMD,
					"invoke":     "/" + e.Name(),
				},
				Timestamp: time.Now(),
			})
		}
	}

	return docs
}

// scanAgents finds .md command files and AGENTS.md.
func (d *DirectoryCollector) scanAgents(dir, repoName string) []any {
	var docs []any

	agentDirs := []string{
		filepath.Join(dir, ".claude", "commands"),
		filepath.Join(dir, ".copilot", "agents"),
	}

	for _, ad := range agentDirs {
		entries, err := os.ReadDir(ad)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			ext := filepath.Ext(e.Name())
			if ext != ".md" && ext != ".yaml" {
				continue
			}

			path := filepath.Join(ad, e.Name())
			content, err := os.ReadFile(path)
			if err != nil {
				continue
			}

			name := strings.TrimSuffix(e.Name(), ext)
			desc := extractFirstDescription(string(content))

			docs = append(docs, esearch.Event{
				Type:    "directory.agent",
				Source:  "directory",
				Repo:    repoName,
				Author:  "scanner",
				Message: fmt.Sprintf("agent: %s — %s", name, desc),
				Body:    truncateStr(string(content), 4000),
				Metadata: map[string]any{
					"agent_name": name,
					"agent_path": path,
					"invoke":     "@" + name,
				},
				Timestamp: time.Now(),
			})
		}
	}

	// AGENTS.md at repo root
	agentsPath := filepath.Join(dir, "AGENTS.md")
	if content, err := os.ReadFile(agentsPath); err == nil {
		docs = append(docs, esearch.Event{
			Type:    "directory.agent",
			Source:  "directory",
			Repo:    repoName,
			Author:  "scanner",
			Message: "AGENTS.md — project agent definitions",
			Body:    truncateStr(string(content), 4000),
			Metadata: map[string]any{
				"agent_name": "agents",
				"agent_path": agentsPath,
			},
			Timestamp: time.Now(),
		})
	}

	return docs
}

// scanProviderMeta reads CLAUDE.md, .claude/settings.json, and other
// provider-specific project configuration.
func (d *DirectoryCollector) scanProviderMeta(dir, repoName string) []any {
	var docs []any

	// CLAUDE.md — project instructions for Claude Code
	claudeMDPaths := []string{
		filepath.Join(dir, "CLAUDE.md"),
		filepath.Join(dir, ".claude", "CLAUDE.md"),
	}
	for _, p := range claudeMDPaths {
		content, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		docs = append(docs, esearch.Event{
			Type:    "directory.provider_config",
			Source:  "directory",
			Repo:    repoName,
			Author:  "scanner",
			Message: fmt.Sprintf("CLAUDE.md project instructions for %s", repoName),
			Body:    truncateStr(string(content), 8000),
			Metadata: map[string]any{
				"provider":  "claude",
				"file_path": p,
				"config_type": "instructions",
			},
			Timestamp: time.Now(),
		})
	}

	// .claude/settings.json — Claude Code project settings
	settingsPath := filepath.Join(dir, ".claude", "settings.json")
	if content, err := os.ReadFile(settingsPath); err == nil {
		docs = append(docs, esearch.Event{
			Type:    "directory.provider_config",
			Source:  "directory",
			Repo:    repoName,
			Author:  "scanner",
			Message: fmt.Sprintf("Claude Code settings for %s", repoName),
			Body:    string(content),
			Metadata: map[string]any{
				"provider":    "claude",
				"file_path":   settingsPath,
				"config_type": "settings",
			},
			Timestamp: time.Now(),
		})
	}

	// .copilot/config.yml
	copilotPaths := []string{
		filepath.Join(dir, ".github", "copilot-instructions.md"),
		filepath.Join(dir, ".copilot", "config.yml"),
	}
	for _, p := range copilotPaths {
		content, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		docs = append(docs, esearch.Event{
			Type:    "directory.provider_config",
			Source:  "directory",
			Repo:    repoName,
			Author:  "scanner",
			Message: fmt.Sprintf("Copilot config for %s", repoName),
			Body:    truncateStr(string(content), 4000),
			Metadata: map[string]any{
				"provider":    "copilot",
				"file_path":   p,
				"config_type": "instructions",
			},
			Timestamp: time.Now(),
		})
	}

	return docs
}

// scanClaudeSessions reads Claude Code project session data from ~/.claude/projects/.
func (d *DirectoryCollector) scanClaudeSessions(dir, repoName string) []any {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}

	// Claude encodes project paths as hyphen-separated: /Users/stokes/Projects/gl1tch → -Users-stokes-Projects-gl1tch
	absDir, err := filepath.Abs(dir)
	if err != nil {
		return nil
	}
	encoded := strings.ReplaceAll(absDir, "/", "-")
	if strings.HasPrefix(encoded, "-") {
		encoded = encoded[0:] // keep leading dash
	}

	sessDir := filepath.Join(home, ".claude", "projects", encoded)
	if _, err := os.Stat(sessDir); err != nil {
		return nil
	}

	// Read the CLAUDE.md memory file if it exists in the project dir
	var docs []any
	memoryPath := filepath.Join(sessDir, "CLAUDE.md")
	if content, err := os.ReadFile(memoryPath); err == nil {
		docs = append(docs, esearch.Event{
			Type:    "directory.provider_memory",
			Source:  "directory",
			Repo:    repoName,
			Author:  "claude-code",
			Message: fmt.Sprintf("Claude Code memory/context for %s", repoName),
			Body:    truncateStr(string(content), 8000),
			Metadata: map[string]any{
				"provider":    "claude",
				"file_path":   memoryPath,
				"config_type": "memory",
			},
			Timestamp: time.Now(),
		})
	}

	return docs
}

// scanProjectStructure indexes high-level project structure (package files,
// README, Makefile, etc.) so glitch knows what the project is.
func (d *DirectoryCollector) scanProjectStructure(dir, repoName string) []any {
	var docs []any

	// Key project files that describe what the project is
	structureFiles := []string{
		"README.md",
		"go.mod",
		"package.json",
		"Cargo.toml",
		"pyproject.toml",
		"Makefile",
		"Dockerfile",
		"docker-compose.yml",
		"docker-compose.yaml",
		".github/workflows/ci.yml",
		".github/workflows/ci.yaml",
	}

	for _, f := range structureFiles {
		path := filepath.Join(dir, f)
		content, err := os.ReadFile(path)
		if err != nil {
			continue
		}

		docs = append(docs, esearch.Event{
			Type:    "directory.structure",
			Source:  "directory",
			Repo:    repoName,
			Author:  "scanner",
			Message: fmt.Sprintf("%s — project structure file", f),
			Body:    truncateStr(string(content), 4000),
			Metadata: map[string]any{
				"file_name": f,
				"file_path": path,
			},
			Timestamp: time.Now(),
		})
	}

	return docs
}

// scanGitRemote detects the GitHub remote URL and indexes it.
func (d *DirectoryCollector) scanGitRemote(dir, repoName string) []any {
	var docs []any

	// Read .git/config for remote URL
	gitConfig := filepath.Join(dir, ".git", "config")
	content, err := os.ReadFile(gitConfig)
	if err != nil {
		return nil
	}

	// Parse remote origin URL
	lines := strings.Split(string(content), "\n")
	inRemoteOrigin := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == `[remote "origin"]` {
			inRemoteOrigin = true
			continue
		}
		if strings.HasPrefix(trimmed, "[") {
			inRemoteOrigin = false
			continue
		}
		if inRemoteOrigin && strings.HasPrefix(trimmed, "url = ") {
			url := strings.TrimPrefix(trimmed, "url = ")

			// Extract owner/repo from GitHub URL
			ownerRepo := extractGitHubRepo(url)
			if ownerRepo != "" {
				docs = append(docs, esearch.Event{
					Type:    "directory.remote",
					Source:  "directory",
					Repo:    repoName,
					Author:  "scanner",
					Message: fmt.Sprintf("GitHub remote: %s", ownerRepo),
					Body:    url,
					Metadata: map[string]any{
						"remote_url":  url,
						"github_repo": ownerRepo,
					},
					Timestamp: time.Now(),
				})
			}
			break
		}
	}

	return docs
}

// extractGitHubRepo parses owner/repo from various GitHub URL formats.
func extractGitHubRepo(url string) string {
	// git@github.com:owner/repo.git
	if strings.HasPrefix(url, "git@github.com:") {
		repo := strings.TrimPrefix(url, "git@github.com:")
		repo = strings.TrimSuffix(repo, ".git")
		return repo
	}
	// https://github.com/owner/repo.git
	if strings.Contains(url, "github.com/") {
		idx := strings.Index(url, "github.com/")
		repo := url[idx+len("github.com/"):]
		repo = strings.TrimSuffix(repo, ".git")
		return repo
	}
	return ""
}

// extractFirstDescription pulls a description from markdown content.
func extractFirstDescription(content string) string {
	lines := strings.Split(content, "\n")
	inFrontmatter := false
	frontmatterCount := 0

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "---" {
			frontmatterCount++
			if frontmatterCount == 1 {
				inFrontmatter = true
				continue
			}
			if frontmatterCount == 2 {
				inFrontmatter = false
				continue
			}
		}
		if inFrontmatter {
			if k, v, ok := strings.Cut(trimmed, ":"); ok {
				if strings.TrimSpace(k) == "description" {
					desc := strings.TrimSpace(v)
					if desc != "" {
						return truncateStr(desc, 200)
					}
				}
			}
			continue
		}
	}

	// Fallback: first non-empty, non-heading line
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") || trimmed == "---" {
			continue
		}
		return truncateStr(trimmed, 200)
	}
	return ""
}

func truncateStr(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
