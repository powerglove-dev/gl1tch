// directory.go holds the filesystem-artifact scan helpers used by the
// unified WorkspaceCollector. The standalone DirectoryCollector type
// was retired when the unified workspace collector replaced the split
// directories/git/github trio — these helpers live on as package-level
// functions because the scanning logic itself is still useful, just
// not as its own goroutine + ticker.
package capability

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/8op-org/gl1tch/internal/esearch"
)

// scanWorkspaceDirArtifacts walks one workspace directory and returns
// every artifact event the scanner can extract from it: skills,
// agents, provider config (CLAUDE.md / .claude / .copilot), Claude
// session memory, project structure files, and the git remote URL.
//
// Returns (nil, error) if the directory itself isn't accessible.
// Returns (events, nil) — possibly empty — when the dir is fine but
// has nothing scan-worthy under it.
//
// The caller (WorkspaceCollector.scanDirectory) is responsible for
// stamping events with workspace_id and bulk-indexing.
func scanWorkspaceDirArtifacts(dir string) ([]any, error) {
	if _, err := os.Stat(dir); err != nil {
		return nil, fmt.Errorf("directory not accessible: %w", err)
	}

	repoName := filepath.Base(dir)

	var docs []any
	docs = append(docs, scanDirSkills(dir, repoName)...)
	docs = append(docs, scanDirAgents(dir, repoName)...)
	docs = append(docs, scanDirProviderMeta(dir, repoName)...)
	docs = append(docs, scanDirClaudeSessions(dir, repoName)...)
	docs = append(docs, scanDirProjectStructure(dir, repoName)...)
	docs = append(docs, scanDirGitRemote(dir, repoName)...)
	return docs, nil
}

// scanDirSkills finds SKILL.md files in well-known skill directories.
func scanDirSkills(dir, repoName string) []any {
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

// scanDirAgents finds .md command files and AGENTS.md.
func scanDirAgents(dir, repoName string) []any {
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

// scanDirProviderMeta reads CLAUDE.md, .claude/settings.json, and
// other provider-specific project configuration.
func scanDirProviderMeta(dir, repoName string) []any {
	var docs []any

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
				"provider":    "claude",
				"file_path":   p,
				"config_type": "instructions",
			},
			Timestamp: time.Now(),
		})
	}

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

// scanDirClaudeSessions reads Claude Code project session memory data
// from ~/.claude/projects/.
func scanDirClaudeSessions(dir, repoName string) []any {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}

	absDir, err := filepath.Abs(dir)
	if err != nil {
		return nil
	}
	encoded := strings.ReplaceAll(absDir, "/", "-")

	sessDir := filepath.Join(home, ".claude", "projects", encoded)
	if _, err := os.Stat(sessDir); err != nil {
		return nil
	}

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

// scanDirProjectStructure indexes high-level project structure
// (package files, README, Makefile, etc.) so glitch knows what the
// project is.
func scanDirProjectStructure(dir, repoName string) []any {
	var docs []any

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

// scanDirGitRemote detects the GitHub remote URL and indexes it.
func scanDirGitRemote(dir, repoName string) []any {
	var docs []any

	gitConfig := filepath.Join(dir, ".git", "config")
	content, err := os.ReadFile(gitConfig)
	if err != nil {
		return nil
	}

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
	if strings.HasPrefix(url, "git@github.com:") {
		repo := strings.TrimPrefix(url, "git@github.com:")
		repo = strings.TrimSuffix(repo, ".git")
		return repo
	}
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
