package collector

import (
	"context"
	"fmt"
	"log/slog"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/8op-org/gl1tch/internal/esearch"
)

// GitCollector watches configured git repositories and indexes new commits.
type GitCollector struct {
	// Repos is the list of absolute paths to git repositories to watch.
	Repos []string
	// Interval is how often to poll for new commits. Defaults to 60s.
	Interval time.Duration
	// WorkspaceID is stamped on every indexed event so brain queries
	// can scope to one workspace's commits. Empty when the collector
	// runs outside any workspace pod (legacy / always-on global).
	WorkspaceID string
}

func (g *GitCollector) Name() string { return "git" }

func (g *GitCollector) Start(ctx context.Context, es *esearch.Client) error {
	if g.Interval == 0 {
		g.Interval = 60 * time.Second
	}

	// Track the last indexed SHA per repo so we only index new commits.
	cursors := make(map[string]string)

	// Seed cursors with the latest commit so we don't backfill the entire history
	// on first run. Only new commits from this point forward get indexed.
	for _, repo := range g.Repos {
		sha, _ := gitLatestSHA(repo)
		if sha != "" {
			cursors[repo] = sha
			slog.Info("git collector: seeded cursor", "repo", filepath.Base(repo), "sha", sha[:8])
		}
	}

	ticker := time.NewTicker(g.Interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			start := time.Now()
			var lastErr error
			indexed := 0
			for _, repo := range g.Repos {
				n, err := g.poll(ctx, es, repo, cursors)
				indexed += n
				if err != nil {
					lastErr = err
					slog.Warn("git collector: poll error", "repo", filepath.Base(repo), "err", err)
				}
			}
			// Heartbeat for the brain UI: tells the desktop "git ran
			// at T, took D, found N new commits, err=…". The brain
			// popover shows last-run timestamps so users can see a
			// collector is alive even when it has nothing to index.
			RecordRun("git", start, indexed, lastErr)
		}
	}
}

// poll runs one collection cycle for a single repo and returns the
// number of commits indexed (for the run heartbeat) plus any error.
func (g *GitCollector) poll(ctx context.Context, es *esearch.Client, repo string, cursors map[string]string) (int, error) {
	lastSHA := cursors[repo]

	// Build git log range.
	var rangeArg string
	if lastSHA != "" {
		rangeArg = lastSHA + "..HEAD"
	} else {
		// First poll — get last 50 commits.
		rangeArg = "-50"
	}

	commits, err := gitLog(repo, rangeArg)
	if err != nil {
		return 0, err
	}
	if len(commits) == 0 {
		return 0, nil
	}

	repoName := filepath.Base(repo)
	slog.Info("git collector: new commits", "repo", repoName, "count", len(commits))

	// Index each commit as an event.
	var docs []any
	for _, c := range commits {
		docs = append(docs, esearch.Event{
			Type:         "git.commit",
			Source:       "git",
			WorkspaceID:  g.WorkspaceID,
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

	if err := es.BulkIndex(ctx, esearch.IndexEvents, StampWorkspaceID(g.WorkspaceID, docs)); err != nil {
		return 0, fmt.Errorf("bulk index: %w", err)
	}

	// Update cursor to newest commit.
	cursors[repo] = commits[0].sha
	return len(commits), nil
}

type gitCommit struct {
	sha       string
	author    string
	message   string
	body      string
	files     []string
	timestamp time.Time
}

// gitLog returns commits in reverse chronological order.
func gitLog(repo, rangeArg string) ([]gitCommit, error) {
	// Format: SHA\x1fauthor\x1ftimestamp\x1fsubject\x1fbody\x1e
	format := "%H%x1f%an%x1f%aI%x1f%s%x1f%b%x1e"
	args := []string{"-C", repo, "log", "--format=" + format, "--name-only"}

	if strings.HasPrefix(rangeArg, "-") {
		args = append(args, rangeArg)
	} else {
		args = append(args, rangeArg)
	}

	out, err := exec.Command("git", args...).Output()
	if err != nil {
		return nil, fmt.Errorf("git log: %w", err)
	}

	raw := strings.TrimSpace(string(out))
	if raw == "" {
		return nil, nil
	}

	// Split on record separator.
	records := strings.Split(raw, "\x1e")
	var commits []gitCommit

	for _, rec := range records {
		rec = strings.TrimSpace(rec)
		if rec == "" {
			continue
		}

		// The record has the formatted line, then a blank line, then file names.
		lines := strings.SplitN(rec, "\n", 2)
		fields := strings.SplitN(lines[0], "\x1f", 5)
		if len(fields) < 4 {
			continue
		}

		ts, _ := time.Parse(time.RFC3339, fields[2])

		c := gitCommit{
			sha:       fields[0],
			author:    fields[1],
			timestamp: ts,
			message:   fields[3],
		}
		if len(fields) >= 5 {
			c.body = strings.TrimSpace(fields[4])
		}

		// Parse file names (everything after the formatted header).
		if len(lines) > 1 {
			for _, f := range strings.Split(strings.TrimSpace(lines[1]), "\n") {
				f = strings.TrimSpace(f)
				if f != "" {
					c.files = append(c.files, f)
				}
			}
		}

		commits = append(commits, c)
	}

	return commits, nil
}

func gitLatestSHA(repo string) (string, error) {
	out, err := exec.Command("git", "-C", repo, "rev-parse", "HEAD").Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func gitCurrentBranch(repo string) string {
	out, err := exec.Command("git", "-C", repo, "rev-parse", "--abbrev-ref", "HEAD").Output()
	if err != nil {
		return "unknown"
	}
	return strings.TrimSpace(string(out))
}
