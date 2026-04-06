package collector

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/8op-org/gl1tch/internal/esearch"
)

// IngestAll runs a one-shot backfill of all configured collectors.
// Unlike the polling collectors, this reads everything from the beginning
// and indexes it all into ES.
func IngestAll(ctx context.Context, es *esearch.Client, cfg *Config) error {
	var totalDocs int

	// Claude Code history.
	if cfg.Claude.Enabled {
		n, err := ingestClaudeHistory(ctx, es)
		if err != nil {
			slog.Warn("ingest: claude history", "err", err)
		} else {
			totalDocs += n
		}
		n, err = ingestClaudeProjects(ctx, es)
		if err != nil {
			slog.Warn("ingest: claude projects", "err", err)
		} else {
			totalDocs += n
		}
	}

	// Copilot CLI.
	if cfg.Copilot.Enabled {
		n, err := ingestCopilot(ctx, es)
		if err != nil {
			slog.Warn("ingest: copilot", "err", err)
		} else {
			totalDocs += n
		}
	}

	// Mattermost channels.
	if cfg.Mattermost.URL != "" && cfg.Mattermost.Token != "" {
		n, err := IngestMattermost(ctx, es, cfg)
		if err != nil {
			slog.Warn("ingest: mattermost", "err", err)
		} else {
			totalDocs += n
		}
	}

	// Git repos.
	for _, repo := range cfg.Git.Repos {
		n, err := ingestGitRepo(ctx, es, repo)
		if err != nil {
			slog.Warn("ingest: git", "repo", filepath.Base(repo), "err", err)
		} else {
			totalDocs += n
		}
	}

	slog.Info("ingest: complete", "total_docs", totalDocs)
	return nil
}

func ingestClaudeHistory(ctx context.Context, es *esearch.Client) (int, error) {
	home, _ := os.UserHomeDir()
	path := filepath.Join(home, ".claude", "history.jsonl")

	f, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 1024*256), 1024*256)

	var batch []any
	total := 0

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var entry claudeHistoryEntry
		if err := json.Unmarshal(line, &entry); err != nil {
			continue
		}
		if entry.Display == "" {
			continue
		}

		ts := time.UnixMilli(entry.Timestamp)
		project := filepath.Base(entry.Project)
		if project == "." || project == "" {
			project = "unknown"
		}

		batch = append(batch, esearch.Event{
			Type:    "claude.prompt",
			Source:  "claude",
			Repo:    project,
			Author:  "user",
			Message: truncate(entry.Display, 500),
			Body:    entry.Display,
			Metadata: map[string]any{
				"session_id":   entry.SessionID,
				"project_path": entry.Project,
			},
			Timestamp: ts,
		})

		if len(batch) >= 200 {
			if err := es.BulkIndex(ctx, esearch.IndexEvents, batch); err != nil {
				return total, err
			}
			total += len(batch)
			batch = batch[:0]
		}
	}

	if len(batch) > 0 {
		if err := es.BulkIndex(ctx, esearch.IndexEvents, batch); err != nil {
			return total, err
		}
		total += len(batch)
	}

	slog.Info("ingest: claude history", "docs", total)
	return total, nil
}

func ingestClaudeProjects(ctx context.Context, es *esearch.Client) (int, error) {
	home, _ := os.UserHomeDir()
	projectsDir := filepath.Join(home, ".claude", "projects")

	entries, err := os.ReadDir(projectsDir)
	if err != nil {
		return 0, err
	}

	c := &ClaudeProjectCollector{}
	total := 0

	for _, projDir := range entries {
		if !projDir.IsDir() {
			continue
		}
		projectName := decodeClaudeProjectName(projDir.Name())
		projPath := filepath.Join(projectsDir, projDir.Name())

		files, err := filepath.Glob(filepath.Join(projPath, "*.jsonl"))
		if err != nil {
			continue
		}

		for _, f := range files {
			docs := c.parseSessionFile(f, projectName)
			if len(docs) > 0 {
				if err := es.BulkIndex(ctx, esearch.IndexEvents, docs); err != nil {
					slog.Warn("ingest: claude project file", "err", err)
					continue
				}
				total += len(docs)
			}
		}
	}

	slog.Info("ingest: claude projects", "docs", total)
	return total, nil
}

func ingestCopilot(ctx context.Context, es *esearch.Client) (int, error) {
	home, _ := os.UserHomeDir()
	copilotDir := filepath.Join(home, ".copilot")

	total := 0

	// Command history.
	data, err := os.ReadFile(filepath.Join(copilotDir, "command-history-state.json"))
	if err == nil {
		var state struct {
			CommandHistory []string `json:"commandHistory"`
		}
		if json.Unmarshal(data, &state) == nil {
			now := time.Now()
			var batch []any
			for i, cmd := range state.CommandHistory {
				cmd = fmt.Sprintf("%s", cmd) // ensure string
				if cmd == "" || cmd == "/quit" || cmd == "/clear" {
					continue
				}
				batch = append(batch, esearch.Event{
					Type:    "copilot.command",
					Source:  "copilot",
					Author:  "user",
					Message: cmd,
					Metadata: map[string]any{
						"index": i,
					},
					Timestamp: now,
				})
			}
			if len(batch) > 0 {
				if err := es.BulkIndex(ctx, esearch.IndexEvents, batch); err == nil {
					total += len(batch)
				}
			}
		}
	}

	// Log files.
	c := &CopilotCollector{}
	files, _ := filepath.Glob(filepath.Join(copilotDir, "logs", "process-*.log"))
	for _, path := range files {
		docs := c.parseLogFile(path)
		if len(docs) > 0 {
			if err := es.BulkIndex(ctx, esearch.IndexEvents, docs); err == nil {
				total += len(docs)
			}
		}
	}

	slog.Info("ingest: copilot", "docs", total)
	return total, nil
}

func ingestGitRepo(ctx context.Context, es *esearch.Client, repo string) (int, error) {
	commits, err := gitLog(repo, "-200") // last 200 commits
	if err != nil {
		return 0, err
	}

	repoName := filepath.Base(repo)
	var batch []any
	for _, c := range commits {
		batch = append(batch, esearch.Event{
			Type:         "git.commit",
			Source:       "git",
			Repo:         repoName,
			Branch:       gitCurrentBranch(repo),
			Author:       c.author,
			SHA:          c.sha,
			Message:      c.message,
			Body:         c.body,
			FilesChanged: c.files,
			Timestamp:    c.timestamp,
		})
	}

	if len(batch) > 0 {
		if err := es.BulkIndex(ctx, esearch.IndexEvents, batch); err != nil {
			return 0, err
		}
	}

	slog.Info("ingest: git", "repo", repoName, "commits", len(batch))
	return len(batch), nil
}
