// workspace_collector.go is the single per-workspace data collector.
//
// It replaces the old split where directories, git, and github were
// three independently-scheduled collectors that the user had to wire
// up via separate config sections. The split was a leaky abstraction:
// every workspace dir is at most one entity (a checkout), and the
// answer to "what should we collect for this checkout" — local files,
// commit history, GitHub PR/issue activity — is always the union of
// the three. Forcing the user to enable each layer separately produced
// the disconnected popover state that motivated this rewrite (a
// directory could exist with neither git nor github watching it, or
// vice versa).
//
// Behavior:
//
//   - Owns one ticker, one heartbeat ("workspace"), one tick span. The
//     popover row goes from "you have 3 separate things to reason about"
//     to a single status line per workspace.
//   - Iterates the workspace's enabled directories on every tick. For
//     each dir it always runs the directory scan, runs git polling if
//     the dir is a git repo, and runs github polling if the repo has a
//     github.com origin. There is no per-collector toggle anymore;
//     enable/disable is per-directory.
//   - Maintains per-directory cursors for git (last indexed SHA) and
//     github (last poll timestamp) so subsequent ticks are incremental.
//   - Indexed events keep their existing source field values
//     ("git" / "github" / "directory") so existing ES data and
//     downstream queries continue to work. The desktop popover sums
//     those three source buckets into the single "workspace" row.
package collector

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/8op-org/gl1tch/internal/esearch"
)

// WorkspaceCollector is the unified per-workspace collector. One
// instance is constructed per workspace pod with the workspace's
// enabled directory list; it stays alive for the lifetime of the pod.
type WorkspaceCollector struct {
	// Dirs is the list of *enabled* workspace directories. Disabled
	// dirs are filtered out at the store layer (see
	// Store.getWorkspaceDirectories) before they reach this slice, so
	// the collector never has to know about the toggle column.
	Dirs []string
	// Interval is how often the unified tick fires. Defaults to 60s
	// — fast enough that new git commits show up promptly, slow
	// enough that github polling doesn't run hot. The github subsystem
	// is incremental via per-dir lastPoll, so a tighter cadence here
	// doesn't do extra work.
	Interval time.Duration
	// WorkspaceID is stamped on every doc the collector indexes so
	// brain queries can scope to one workspace.
	WorkspaceID string

	// per-directory state, allocated on first use inside Start.
	gitCursors     map[string]string
	githubLastPoll map[string]time.Time
}

func (w *WorkspaceCollector) Name() string { return "workspace" }

func (w *WorkspaceCollector) Start(ctx context.Context, es *esearch.Client) error {
	if w.Interval == 0 {
		w.Interval = 60 * time.Second
	}
	w.gitCursors = make(map[string]string, len(w.Dirs))
	w.githubLastPoll = make(map[string]time.Time, len(w.Dirs))

	slog.Info("workspace collector: started",
		"workspace", w.WorkspaceID,
		"dirs", len(w.Dirs),
		"interval", w.Interval)
	for _, d := range w.Dirs {
		slog.Debug("workspace collector: watching dir",
			"workspace", w.WorkspaceID, "dir", d)
	}

	// Same nil-ES guard pattern as the old separate collectors:
	// surface the failure via RecordRun so the brain popover row
	// turns red with a real reason instead of staying gray-dotted.
	if es == nil {
		err := fmt.Errorf("workspace collector: elasticsearch client is nil — check ES connectivity")
		RecordRun("workspace", time.Now(), 0, err)
		return err
	}

	// Run one cycle immediately so the first wave of artifacts lands
	// in ES on startup instead of waiting Interval (default 60s) for
	// the first ticker tick. The brain popover otherwise reads
	// "0 indexed" for the first minute of every desktop session.
	w.runOnce(ctx, es)

	ticker := time.NewTicker(w.Interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			w.runOnce(ctx, es)
		}
	}
}

// runOnce executes a single collection cycle across all enabled
// workspace directories. Per-tick metrics roll up into the single
// "workspace" heartbeat — collectors that previously emitted three
// separate heartbeats (git, github, directories) now contribute to one.
func (w *WorkspaceCollector) runOnce(ctx context.Context, es *esearch.Client) {
	slog.Debug("workspace collector: tick", "workspace", w.WorkspaceID, "dirs", len(w.Dirs))
	tickCtx, tickDone := startTickSpan(ctx, "workspace", w.WorkspaceID)
	start := time.Now()
	var lastErr error
	indexed := 0

	for _, dir := range w.Dirs {
		if dir == "" {
			continue
		}
		n, err := w.collectOneDir(tickCtx, es, dir)
		indexed += n
		if err != nil {
			lastErr = err
			slog.Warn("workspace collector: dir error",
				"workspace", w.WorkspaceID, "dir", dir, "err", err)
		}
	}

	slog.Debug("workspace collector: tick done",
		"workspace", w.WorkspaceID, "indexed", indexed, "dur", time.Since(start))
	tickDone(indexed, lastErr)
	RecordRun("workspace", start, indexed, lastErr)
}

// collectOneDir runs every applicable subsystem against a single
// workspace directory and returns the total number of docs indexed.
//
// Subsystems run sequentially (not in parallel goroutines) because
// they all hit the same on-disk state and the gh CLI is the slowest by
// far — parallelizing the cheap subsystems behind it would only save
// a few milliseconds per dir while complicating error reporting.
func (w *WorkspaceCollector) collectOneDir(ctx context.Context, es *esearch.Client, dir string) (int, error) {
	indexed := 0
	var firstErr error

	// 1. Directory scan — always runs. Picks up skills, agents,
	//    project structure, CLAUDE.md, copilot config, etc.
	if n, err := w.scanDirectory(ctx, es, dir); err != nil {
		firstErr = err
	} else {
		indexed += n
	}

	// 2. Git poll — runs only if the dir is a git checkout. Indexes
	//    new commits since the last cursor.
	if IsGitRepo(dir) {
		if n, err := w.pollGit(ctx, es, dir); err != nil {
			if firstErr == nil {
				firstErr = err
			}
		} else {
			indexed += n
		}

		// 3. GitHub poll — only if the git repo has a github.com
		//    origin. Derives the slug from .git/config; no separate
		//    config knob needed.
		if slug := GitHubRepoSlug(dir); slug != "" {
			if n, err := w.pollGitHub(ctx, es, dir, slug); err != nil {
				if firstErr == nil {
					firstErr = err
				}
			} else {
				indexed += n
			}
		}
	}

	return indexed, firstErr
}

// ── git subsystem ──────────────────────────────────────────────────────────

func (w *WorkspaceCollector) pollGit(ctx context.Context, es *esearch.Client, repo string) (int, error) {
	lastSHA := w.gitCursors[repo]

	var rangeArg string
	if lastSHA != "" {
		rangeArg = lastSHA + "..HEAD"
	} else {
		// First poll — backfill the most recent 50 commits per repo
		// so users open gl1tch and immediately see existing history.
		// The previous split-collector implementation made the same
		// choice for the same reason; preserved here verbatim.
		rangeArg = "-50"
	}

	commits, err := gitLog(repo, rangeArg)
	if err != nil {
		return 0, err
	}
	if len(commits) == 0 {
		return 0, nil
	}

	repoName := shortRepoName(repo)
	branch := gitCurrentBranch(repo)

	docs := make([]any, 0, len(commits))
	for _, c := range commits {
		docs = append(docs, esearch.Event{
			Type:         "git.commit",
			Source:       "git",
			WorkspaceID:  w.WorkspaceID,
			Repo:         repoName,
			Branch:       branch,
			Author:       c.author,
			SHA:          c.sha,
			Message:      c.message,
			Body:         c.body,
			FilesChanged: c.files,
			Timestamp:    c.timestamp,
		})
	}

	slog.Info("workspace collector: new commits",
		"workspace", w.WorkspaceID, "repo", repoName, "count", len(commits))

	if err := es.BulkIndex(ctx, esearch.IndexEvents, StampWorkspaceID(w.WorkspaceID, docs)); err != nil {
		return 0, fmt.Errorf("git bulk index: %w", err)
	}

	w.gitCursors[repo] = commits[0].sha
	return len(commits), nil
}

// ── github subsystem ───────────────────────────────────────────────────────

func (w *WorkspaceCollector) pollGitHub(ctx context.Context, es *esearch.Client, dir, slug string) (int, error) {
	// First-poll backfill window: 24h. Same value the old GitHubCollector
	// used; tightened by lastPoll on every subsequent tick.
	since, ok := w.githubLastPoll[dir]
	if !ok || since.IsZero() {
		since = time.Now().Add(-24 * time.Hour)
	}

	if !ghAvailable() {
		// Surface the missing-CLI failure on the unified heartbeat
		// the same way the old GitHubCollector surfaced it on the
		// "github" row. The error string is the same so existing user
		// muscle memory ("gh CLI not found") still applies.
		return 0, fmt.Errorf("github: gh CLI not found on PATH — install gh and re-authenticate")
	}

	docs, err := ghCollectAll(ctx, slug, since)
	if err != nil {
		return 0, err
	}
	if len(docs) == 0 {
		w.githubLastPoll[dir] = time.Now()
		return 0, nil
	}

	slog.Info("workspace collector: new github activity",
		"workspace", w.WorkspaceID, "repo", slug, "events", len(docs))

	if err := es.BulkIndex(ctx, esearch.IndexEvents, StampWorkspaceID(w.WorkspaceID, docs)); err != nil {
		return 0, fmt.Errorf("github bulk index: %w", err)
	}

	w.githubLastPoll[dir] = time.Now()
	return len(docs), nil
}

// ── directory scan subsystem ───────────────────────────────────────────────

// scanDirectory runs the filesystem-artifact scan over a single
// directory and indexes whatever it finds. Returns the doc count for
// the unified heartbeat.
func (w *WorkspaceCollector) scanDirectory(ctx context.Context, es *esearch.Client, dir string) (int, error) {
	docs, err := scanWorkspaceDirArtifacts(dir)
	if err != nil {
		return 0, err
	}
	if len(docs) == 0 {
		return 0, nil
	}
	if err := es.BulkIndex(ctx, esearch.IndexEvents, StampWorkspaceID(w.WorkspaceID, docs)); err != nil {
		return 0, fmt.Errorf("directory bulk index: %w", err)
	}
	return len(docs), nil
}

// shortRepoName returns the last path segment of a directory path.
// Pulled out as a tiny helper so the git subsystem doesn't need to
// import filepath at the call site.
func shortRepoName(dir string) string {
	for i := len(dir) - 1; i >= 0; i-- {
		if dir[i] == '/' {
			return dir[i+1:]
		}
	}
	return dir
}

// ghMu serializes gh CLI invocations across the entire process. The
// gh binary is happy to run concurrent commands but every invocation
// loads the user's keychain entry, which on macOS pops a system
// security prompt if the keychain item's ACL hasn't been granted yet
// — running ten parallel commands surfaces ten prompts. Serializing
// here keeps the prompt count to one per session.
var ghMu sync.Mutex
