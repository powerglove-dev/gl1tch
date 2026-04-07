package glitchd

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/8op-org/gl1tch/internal/busd"
	"github.com/8op-org/gl1tch/internal/collector"
)

// PublishBusEvent publishes an event onto the gl1tch bus. Used by the
// desktop app to notify glitch-notify (the macOS systray) when the
// brain raises an alert. Degrades silently if busd is not running so
// the desktop UI never blocks on the bus.
func PublishBusEvent(topic string, payload any) error {
	sock := busdSocketPath()
	if sock == "" {
		return nil
	}
	return busd.PublishEvent(sock, topic, payload)
}

// CollectorActivity is one collector's recent indexing stats. Used by
// the brain popover to show real per-collector deltas instead of the
// derived "next in" countdowns. Counts are sourced from Elasticsearch
// using the `source` field on glitch-events documents.
type CollectorActivity struct {
	Source       string `json:"source"`
	TotalDocs    int64  `json:"total_docs"`
	LastSeenMs   int64  `json:"last_seen_ms,omitempty"`
	NewSinceLast int64  `json:"new_since_last,omitempty"`
}

// QueryCollectorActivity asks Elasticsearch how many docs each
// collector source has, and the timestamp of its most recent doc.
// The brain loop calls this periodically and computes deltas between
// polls so the UI can show "got 12 new commits in the last 30s".
//
// Uses the raw HTTP API (no esearch.Client) so the desktop binary
// doesn't pull the heavy ES client into its bundle. observer.yaml's
// elasticsearch.address is honored.
func QueryCollectorActivity(ctx context.Context) ([]CollectorActivity, error) {
	cfg, err := collector.LoadConfig()
	if err != nil {
		return nil, err
	}
	addr := cfg.Elasticsearch.Address
	if addr == "" {
		addr = "http://localhost:9200"
	}

	// Aggregation: group by source.keyword, get count + max(@timestamp).
	// We use the .keyword subfield because `source` is mapped as text in
	// the events index. Falls back gracefully if the index is missing.
	body := `{
		"size": 0,
		"aggs": {
			"by_source": {
				"terms": { "field": "source.keyword", "size": 50 },
				"aggs": {
					"last_seen": { "max": { "field": "timestamp" } }
				}
			}
		}
	}`

	url := fmt.Sprintf("%s/glitch-events/_search", addr)
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader([]byte(body)))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	hc := &http.Client{Timeout: 3 * time.Second}
	resp, err := hc.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == 404 {
		// Index doesn't exist yet — collectors haven't indexed anything.
		return []CollectorActivity{}, nil
	}
	if resp.StatusCode >= 400 {
		raw, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("elasticsearch %d: %s", resp.StatusCode, string(raw))
	}

	var parsed struct {
		Aggregations struct {
			BySource struct {
				Buckets []struct {
					Key      string `json:"key"`
					DocCount int64  `json:"doc_count"`
					LastSeen struct {
						Value float64 `json:"value"`
					} `json:"last_seen"`
				} `json:"buckets"`
			} `json:"by_source"`
		} `json:"aggregations"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return nil, err
	}

	out := make([]CollectorActivity, 0, len(parsed.Aggregations.BySource.Buckets))
	for _, b := range parsed.Aggregations.BySource.Buckets {
		out = append(out, CollectorActivity{
			Source:     b.Key,
			TotalDocs:  b.DocCount,
			LastSeenMs: int64(b.LastSeen.Value),
		})
	}
	return out, nil
}

// CollectorConfigPath returns the absolute path to observer.yaml. The
// desktop "Edit collectors" modal shows this so the user knows where
// the file lives.
func CollectorConfigPath() (string, error) {
	return collector.DefaultConfigPath()
}

// EnsureCollectorConfig writes the default observer.yaml if it doesn't
// already exist. Called before "Read" so users always see the fully
// commented starter file instead of a missing-file error.
func EnsureCollectorConfig() error {
	return collector.EnsureDefaultConfig()
}

// ReadCollectorConfig returns the raw observer.yaml contents. If the
// file doesn't exist yet, it's created from defaults first so the
// in-app editor always opens with a real, useful starting point.
func ReadCollectorConfig() (string, error) {
	if err := EnsureCollectorConfig(); err != nil {
		return "", err
	}
	path, err := CollectorConfigPath()
	if err != nil {
		return "", err
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// AddCollectorDirectory adds path to the workspace-scoped collector
// set. It's the single entry point users hit when they "add a
// directory" — the helper handles three things at once so the user
// doesn't have to think about git vs github vs filesystem scanning:
//
//  1. Appends path to observer.yaml's directories.paths so the
//     directory collector scans it for agents/skills/CLAUDE.md.
//  2. If path contains a .git directory, also appends it to
//     observer.yaml's git.repos so the git collector indexes its
//     commit history.
//  3. If path's git remote origin is a github.com URL, parses the
//     owner/repo and appends it to observer.yaml's github.repos so
//     the github collector indexes PRs/issues/reviews.
//
// The collectors re-read observer.yaml on each tick (or are always
// running and pick up the new config at startup), so all three
// sources start flowing without a restart.
func AddCollectorDirectory(path string) error {
	if path == "" {
		return fmt.Errorf("path is required")
	}
	if err := EnsureCollectorConfig(); err != nil {
		return err
	}
	cfg, err := collector.LoadConfig()
	if err != nil {
		return err
	}

	dirty := false

	// 1. directories.paths — the filesystem scanner
	if !containsString(cfg.Directories.Paths, path) {
		cfg.Directories.Paths = append(cfg.Directories.Paths, path)
		dirty = true
	}

	// 2. git.repos — if this is a git checkout
	if isGitRepo(path) {
		if !containsString(cfg.Git.Repos, path) {
			cfg.Git.Repos = append(cfg.Git.Repos, path)
			dirty = true
		}

		// 3. github.repos — if origin is on github
		if slug := githubRepoSlug(path); slug != "" {
			if !containsString(cfg.GitHub.Repos, slug) {
				cfg.GitHub.Repos = append(cfg.GitHub.Repos, slug)
				dirty = true
			}
		}
	}

	if !dirty {
		return nil
	}
	return writeCollectorConfigStruct(cfg)
}

// RemoveCollectorDirectory removes path and any derived entries from
// observer.yaml. Symmetric with AddCollectorDirectory — if adding the
// directory pulled in git + github, removing it pulls all three back
// out.
func RemoveCollectorDirectory(path string) error {
	if path == "" {
		return nil
	}
	cfg, err := collector.LoadConfig()
	if err != nil {
		return err
	}

	dirty := false

	if before := len(cfg.Directories.Paths); before > 0 {
		cfg.Directories.Paths = removeString(cfg.Directories.Paths, path)
		if len(cfg.Directories.Paths) != before {
			dirty = true
		}
	}

	// Git repos are stored by absolute path, so we can remove by the
	// same path that was added.
	if before := len(cfg.Git.Repos); before > 0 {
		cfg.Git.Repos = removeString(cfg.Git.Repos, path)
		if len(cfg.Git.Repos) != before {
			dirty = true
		}
	}

	// GitHub repos are stored as "owner/repo" slugs. We need to
	// re-resolve the slug from the path *before* it's removed from
	// disk (if the user is just unlinking the workspace entry the
	// repo is still there; if they've deleted the dir it isn't and
	// slug resolution quietly no-ops).
	if slug := githubRepoSlug(path); slug != "" {
		if before := len(cfg.GitHub.Repos); before > 0 {
			cfg.GitHub.Repos = removeString(cfg.GitHub.Repos, slug)
			if len(cfg.GitHub.Repos) != before {
				dirty = true
			}
		}
	}

	if !dirty {
		return nil
	}
	return writeCollectorConfigStruct(cfg)
}

// containsString is a tiny helper so Add/Remove stay readable.
func containsString(haystack []string, needle string) bool {
	for _, s := range haystack {
		if s == needle {
			return true
		}
	}
	return false
}

// removeString returns haystack with all occurrences of needle stripped.
func removeString(haystack []string, needle string) []string {
	out := haystack[:0]
	for _, s := range haystack {
		if s != needle {
			out = append(out, s)
		}
	}
	return out
}

// isGitRepo reports whether path contains a .git directory (or is
// itself one — for bare repos). We're deliberately loose here so
// worktrees and submodule dirs still count: the git collector can
// handle any of them.
func isGitRepo(path string) bool {
	if path == "" {
		return false
	}
	if info, err := os.Stat(filepath.Join(path, ".git")); err == nil {
		// .git can be a directory (normal repo) or a file (worktree).
		_ = info
		return true
	}
	if info, err := os.Stat(filepath.Join(path, "HEAD")); err == nil && !info.IsDir() {
		return true // bare repo
	}
	return false
}

// githubRepoSlug extracts "owner/repo" from the git remote origin of
// the given directory. Returns "" if the dir isn't a git repo, has no
// origin, or the origin isn't on github.com. We parse .git/config
// directly rather than shelling out to `git remote -v` so this works
// in sandboxed builds without git on PATH.
func githubRepoSlug(dir string) string {
	// Try both layouts: .git as a directory (normal repo) or .git as a
	// file that points to the real gitdir (worktrees / submodules).
	gitDir := filepath.Join(dir, ".git")
	if info, err := os.Stat(gitDir); err == nil && !info.IsDir() {
		// .git file format: "gitdir: /abs/path"
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

	// Accept both shapes:
	//   https://github.com/owner/repo(.git)?
	//   git@github.com:owner/repo(.git)?
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
	// Sanity: must look like "owner/repo" (exactly one slash, no spaces).
	if strings.Count(slug, "/") != 1 || strings.ContainsAny(slug, " \t") {
		return ""
	}
	return slug
}

// writeCollectorConfigStruct re-marshals a Config and writes it to
// observer.yaml. Used by Add/RemoveCollectorDirectory.
//
// Trade-off: this loses any user comments / formatting in the file
// because yaml.Marshal doesn't preserve them. We accept that because
// the alternative (mutating the YAML AST in place) is significantly
// more complex and the file is small enough that re-rendering it is
// readable. If a user has heavy custom comments they should edit
// the file directly via the in-app editor instead.
func writeCollectorConfigStruct(cfg *collector.Config) error {
	out, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}
	path, err := CollectorConfigPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, out, 0o644)
}

// WriteCollectorConfig validates and writes new observer.yaml content.
// Validation parses the YAML into the same Config struct collectors
// load at runtime; if parsing fails the file is *not* written so the
// user's running config can't get corrupted from a typo in the editor.
//
// Returns nil on success. On parse failure returns the underlying
// yaml error so the modal can surface it to the user.
func WriteCollectorConfig(content string) error {
	var probe collector.Config
	if err := yaml.Unmarshal([]byte(content), &probe); err != nil {
		return fmt.Errorf("invalid yaml: %w", err)
	}
	path, err := CollectorConfigPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(content), 0o644)
}

// BrainAlertTopic is the busd topic glitch-notify subscribes to for
// brain alerts. Kept here so the desktop and the systray plugin agree
// on the wire name without an import cycle.
const BrainAlertTopic = "brain.alert.raised"
