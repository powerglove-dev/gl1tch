package capability

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"
)

// WorkspaceCapability is the unified per-workspace data source. One instance
// is constructed per workspace pod with the workspace's enabled directory
// list; it stays alive for the lifetime of the pod and the runner invokes it
// once per Interval tick.
//
// It iterates every enabled workspace directory on every tick. For each dir
// it always runs the directory scan, runs git polling if the dir is a git
// repo, and runs github polling if the repo has a github.com origin. There
// is no per-subsystem toggle — enable/disable is per-directory.
//
// Per-directory cursors (git last-indexed SHA, github last-poll timestamp)
// live on the struct so subsequent ticks are incremental.
//
// Emitted docs keep the legacy source field values ("git", "github",
// "directory") so existing ES data and downstream queries work unchanged.
// The brain popover sums those three source buckets into a single
// "workspace" row via the collector.Runs heartbeat.
type WorkspaceCapability struct {
	Dirs        []string
	Interval    time.Duration
	WorkspaceID string

	mu             sync.Mutex
	gitCursors     map[string]string
	githubLastPoll map[string]time.Time
	seeded         bool
}

func (w *WorkspaceCapability) Manifest() Manifest {
	every := w.Interval
	if every == 0 {
		every = 60 * time.Second
	}
	return Manifest{
		Name:        "workspace",
		Description: "Unified per-workspace data source. Walks each enabled directory, runs the filesystem artifact scan, polls git for new commits, and polls GitHub for PR/issue activity when the repo has a github.com origin. Emits one document per commit / artifact / github event.",
		Category:    "workspace.unified",
		Trigger:     Trigger{Mode: TriggerInterval, Every: every},
		Sink:        Sink{Index: true},
	}
}

func (w *WorkspaceCapability) Invoke(ctx context.Context, _ Input) (<-chan Event, error) {
	ch := make(chan Event, 256)
	go func() {
		defer close(ch)
		w.tick(ctx, ch)
	}()
	return ch, nil
}

func (w *WorkspaceCapability) tick(ctx context.Context, ch chan<- Event) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if !w.seeded {
		w.gitCursors = make(map[string]string, len(w.Dirs))
		w.githubLastPoll = make(map[string]time.Time, len(w.Dirs))
		w.seeded = true
	}

	for _, dir := range w.Dirs {
		if ctx.Err() != nil {
			return
		}
		if dir == "" {
			continue
		}
		w.collectOneDir(ctx, dir, ch)
	}
}

// collectOneDir runs every applicable subsystem against a single workspace
// directory and fans the results out as Doc events.
//
// Subsystems run sequentially (not in parallel) because they all hit the
// same on-disk state and the gh CLI is the slowest by far — parallelising
// the cheap subsystems behind it would only save milliseconds per dir while
// complicating error reporting.
func (w *WorkspaceCapability) collectOneDir(ctx context.Context, dir string, ch chan<- Event) {
	// 1. Directory scan — always runs. Picks up skills, agents, project
	//    structure, CLAUDE.md, copilot config, etc.
	if docs, err := scanWorkspaceDirArtifacts(dir); err != nil {
		ch <- Event{Kind: EventError, Err: fmt.Errorf("directory scan %s: %w", dir, err)}
	} else {
		for _, d := range stampWorkspaceIDSlice(w.WorkspaceID, docs) {
			ch <- Event{Kind: EventDoc, Doc: d}
		}
	}

	// 2. Git poll — runs only if the dir is a git checkout. Indexes new
	//    commits since the last cursor.
	if !IsGitRepo(dir) {
		return
	}
	w.emitGitCommits(dir, ch)

	// 3. GitHub poll — only if the git repo has a github.com origin.
	if slug := GitHubRepoSlug(dir); slug != "" {
		w.emitGitHubActivity(ctx, dir, slug, ch)
	}
}

func (w *WorkspaceCapability) emitGitCommits(repo string, ch chan<- Event) {
	lastSHA := w.gitCursors[repo]
	var rangeArg string
	if lastSHA != "" {
		rangeArg = lastSHA + "..HEAD"
	} else {
		// First poll: backfill the most recent 50 commits so users see
		// existing history on startup.
		rangeArg = "-50"
	}

	commits, err := gitLog(repo, rangeArg)
	if err != nil {
		ch <- Event{Kind: EventError, Err: fmt.Errorf("git log %s: %w", repo, err)}
		return
	}
	if len(commits) == 0 {
		return
	}

	repoName := shortRepoName(repo)
	branch := gitCurrentBranch(repo)

	for _, c := range commits {
		ch <- Event{Kind: EventDoc, Doc: map[string]any{
			"type":          "git.commit",
			"source":        "git",
			"workspace_id":  w.WorkspaceID,
			"repo":          repoName,
			"branch":        branch,
			"author":        c.author,
			"sha":           c.sha,
			"message":       c.message,
			"body":          c.body,
			"files_changed": c.files,
			"timestamp":     c.timestamp,
		}}
	}

	slog.Info("workspace capability: new commits",
		"workspace", w.WorkspaceID, "repo", repoName, "count", len(commits))

	w.gitCursors[repo] = commits[0].sha
}

func (w *WorkspaceCapability) emitGitHubActivity(ctx context.Context, dir, slug string, ch chan<- Event) {
	since, ok := w.githubLastPoll[dir]
	if !ok || since.IsZero() {
		since = time.Now().Add(-24 * time.Hour)
	}
	if !ghAvailable() {
		ch <- Event{Kind: EventError, Err: fmt.Errorf("github: gh CLI not found on PATH — install gh and re-authenticate")}
		return
	}

	docs, err := ghCollectAll(ctx, slug, since)
	if err != nil {
		ch <- Event{Kind: EventError, Err: fmt.Errorf("github poll %s: %w", slug, err)}
		return
	}
	w.githubLastPoll[dir] = time.Now()
	if len(docs) == 0 {
		return
	}
	slog.Info("workspace capability: new github activity",
		"workspace", w.WorkspaceID, "repo", slug, "events", len(docs))
	for _, d := range stampWorkspaceIDSlice(w.WorkspaceID, docs) {
		ch <- Event{Kind: EventDoc, Doc: d}
	}
}

// shortRepoName returns the last path segment of a directory path. Pulled
// out as a tiny helper so the git subsystem doesn't need to import filepath
// at the call site.
func shortRepoName(dir string) string {
	for i := len(dir) - 1; i >= 0; i-- {
		if dir[i] == '/' {
			return dir[i+1:]
		}
	}
	return dir
}

// stampWorkspaceIDSlice applies StampWorkspaceID over a slice and returns a
// new slice. Thin convenience wrapper so the capability's emit loops can
// stay readable.
func stampWorkspaceIDSlice(workspaceID string, docs []any) []any {
	if workspaceID == "" || len(docs) == 0 {
		return docs
	}
	return StampWorkspaceID(workspaceID, docs)
}
