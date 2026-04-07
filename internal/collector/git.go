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

	// Self-diagnostic startup log so the user can grep the dev
	// console to confirm the collector goroutine actually launched
	// for their workspace, with the right repo set. The brain
	// popover gives no other signal between "configured" and "ran
	// at least once" — without this line a misconfigured pod looks
	// identical to a healthy one that hasn't ticked yet.
	slog.Info("git collector: started",
		"workspace", g.WorkspaceID,
		"repos", len(g.Repos),
		"interval", g.Interval)
	for _, repo := range g.Repos {
		slog.Debug("git collector: watching repo",
			"workspace", g.WorkspaceID,
			"repo", repo)
	}

	// Nil-ES guard. The pod manager constructs collectors with
	// whatever ES client InitPodManager managed to build, and that
	// can be nil if esearch.New itself failed. Without this check
	// the very first BulkIndex call inside poll() nil-derefs and
	// the goroutine dies before reaching RecordRun, leaving the
	// brain popover stuck on a gray dot with no error message
	// forever. Surfacing it as a RecordRun error paints the row
	// red and tells the user exactly what to fix.
	if es == nil {
		err := fmt.Errorf("git collector: elasticsearch client is nil — check ES connectivity")
		RecordRun("git", time.Now(), 0, err)
		return err
	}

	// Track the last indexed SHA per repo so we only index new commits.
	// Cursors start empty so the first poll cycle hits the
	// empty-cursor branch in poll() and backfills the most recent 50
	// commits per repo. An earlier version of this loop pre-seeded the
	// cursor to HEAD on startup as a "don't backfill the entire
	// history" optimization, but that overshot — it backfilled NOTHING
	// because the very first poll's range was HEAD..HEAD, and the
	// brain popover stayed stuck at "git: 0 indexed" for any user
	// whose existing commits predated the desktop launch. The user's
	// expectation is "I open gl1tch and my recent commits are already
	// in the brain", so we backfill the last 50 instead.
	cursors := make(map[string]string)

	ticker := time.NewTicker(g.Interval)
	defer ticker.Stop()

	// Run one cycle immediately so the first 50 commits land in ES on
	// startup instead of waiting g.Interval (default 60s) for the
	// first ticker tick. The brain popover otherwise reads "0 indexed"
	// for the first minute of every desktop session, which is the
	// exact symptom that masked the cursor seed bug for as long as it
	// did. RecordRun fires from the same path so the heartbeat lands
	// on the row immediately too.
	{
		slog.Debug("git collector: initial poll", "workspace", g.WorkspaceID, "repos", len(g.Repos))
		start := time.Now()
		var lastErr error
		indexed := 0
		for _, repo := range g.Repos {
			n, err := g.poll(ctx, es, repo, cursors)
			indexed += n
			if err != nil {
				lastErr = err
				slog.Warn("git collector: initial poll error", "workspace", g.WorkspaceID, "repo", filepath.Base(repo), "err", err)
			}
		}
		slog.Debug("git collector: initial poll done", "workspace", g.WorkspaceID, "indexed", indexed, "dur", time.Since(start))
		RecordRun("git", start, indexed, lastErr)
	}

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			slog.Debug("git collector: tick", "workspace", g.WorkspaceID, "repos", len(g.Repos))
			tickCtx, tickDone := startTickSpan(ctx, "git", g.WorkspaceID)
			start := time.Now()
			var lastErr error
			indexed := 0
			for _, repo := range g.Repos {
				n, err := g.poll(tickCtx, es, repo, cursors)
				indexed += n
				if err != nil {
					lastErr = err
					slog.Warn("git collector: poll error", "workspace", g.WorkspaceID, "repo", filepath.Base(repo), "err", err)
				}
			}
			slog.Debug("git collector: tick done", "workspace", g.WorkspaceID, "indexed", indexed, "dur", time.Since(start))
			tickDone(indexed, lastErr)
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
	slog.Info("git collector: new commits",
		"workspace", g.WorkspaceID,
		"repo", repoName,
		"count", len(commits))

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

func gitCurrentBranch(repo string) string {
	out, err := exec.Command("git", "-C", repo, "rev-parse", "--abbrev-ref", "HEAD").Output()
	if err != nil {
		return "unknown"
	}
	return strings.TrimSpace(string(out))
}
