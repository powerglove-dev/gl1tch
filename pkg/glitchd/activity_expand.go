// activity_expand.go pulls thread context for docs selected in the
// activity sidebar's drill-in modal before they get handed to the
// analyzer.
//
// The problem this solves: when the user selects a single
// github.issue_comment, the analyzer only sees that comment's body.
// There's no parent issue, no sibling comments, no way for the
// model to know what the thread is actually about. The resulting
// analysis either hallucinates or produces generic talking points
// that don't ground on the real conversation.
//
// ExpandThreads takes the narrowed selection and, for each unique
// (repo, issue_number) or (repo, pr_number) pair it sees, fetches
// every doc in that thread from Elasticsearch and merges them in
// with the originals. Already-selected docs are preserved and
// deduped against the fetched set so the caller can still tell
// which rows the user actually picked.
//
// This is the first step in the "make glitch smarter about indexed
// content" initiative — see the daily-driver project memory. The
// next steps are cross-ref join (follow #NNNN references inside
// bodies) and vector-based retrieval via brainrag, both of which
// plug in on top of this expansion point.
package glitchd

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/8op-org/gl1tch/internal/capability"
)

// ExpansionResult is what ExpandThreads returns. The caller uses
// the EntryKeys set to tell which docs were the user's original
// picks — the analyzer prompt uses this to anchor the model on
// what the user actually cares about, treating the rest as
// supporting context. AllDocs is the merged, de-duplicated,
// chronologically-sorted slice ready to feed to the prompt.
type ExpansionResult struct {
	AllDocs   []RecentEvent
	EntryKeys map[string]bool
}

// ExpandThreads walks the selected docs, collects the unique
// GitHub issue/PR threads they belong to, and pulls every doc in
// those threads from ES so the analyzer can reason over the full
// conversation. Non-github docs pass through untouched — git
// commits, claude sessions, etc. have no "thread" concept here so
// there's nothing to expand.
//
// Best-effort: ES errors on individual thread fetches are logged
// and the function keeps going with whatever it could gather. The
// user is waiting on the analyzer and would rather have a partial
// expansion than a hard failure.
func ExpandThreads(ctx context.Context, workspaceID string, selected []RecentEvent) ExpansionResult {
	result := ExpansionResult{
		EntryKeys: make(map[string]bool, len(selected)),
	}
	if len(selected) == 0 {
		return result
	}

	// Collect unique threads to expand. We key by repo+kind+number
	// so an issue #4164 and a PR #4164 in the same repo don't
	// collide (they can reference entirely different conversations).
	type threadKey struct {
		repo   string
		kind   string // "issue" or "pr"
		number int
	}
	threads := make(map[threadKey]bool)

	// Index the selected docs into the result up front so the
	// expansion can dedupe against them. Entry-key set remembers
	// which rows the user picked so the analyzer prompt can mark
	// them distinctly from the supporting context.
	byKey := make(map[string]RecentEvent, len(selected))
	for _, d := range selected {
		k := dedupeKey(d)
		if _, seen := byKey[k]; !seen {
			byKey[k] = d
			result.EntryKeys[k] = true
		}
		// Only github docs with a ref number are thread-expandable.
		if d.Source != "github" || d.Repo == "" {
			continue
		}
		if d.IssueNumber > 0 {
			threads[threadKey{repo: d.Repo, kind: "issue", number: d.IssueNumber}] = true
		}
		if d.PRNumber > 0 {
			threads[threadKey{repo: d.Repo, kind: "pr", number: d.PRNumber}] = true
		}
	}

	// Fetch each thread and merge. We cap per-thread results at 100
	// docs — any thread bigger than that is pathological and would
	// blow the analyzer prompt anyway.
	for tk := range threads {
		fetchCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
		fetched, err := fetchGithubThread(fetchCtx, workspaceID, tk.repo, tk.kind, tk.number, 100)
		cancel()
		if err != nil {
			// Log via the caller's stderr pipeline would be nicer,
			// but this package doesn't have a slog handle for the
			// activity path. Silently skip — the analyzer will still
			// run on whatever context we managed to gather.
			continue
		}
		for _, d := range fetched {
			k := dedupeKey(d)
			if _, seen := byKey[k]; seen {
				continue
			}
			byKey[k] = d
		}
	}

	// Collect merged docs and sort chronologically so the analyzer
	// prompt reads as a conversation (oldest first). When timestamps
	// tie — rare but possible for bulk-imported rows — fall back to
	// SHA/URL for a stable order.
	all := make([]RecentEvent, 0, len(byKey))
	for _, d := range byKey {
		all = append(all, d)
	}
	sort.SliceStable(all, func(i, j int) bool {
		if all[i].TimestampMs != all[j].TimestampMs {
			return all[i].TimestampMs < all[j].TimestampMs
		}
		return dedupeKey(all[i]) < dedupeKey(all[j])
	})
	result.AllDocs = all
	return result
}

// dedupeKey produces a stable identity for a RecentEvent across
// the selected set and the ES fetch results. SHA is the primary
// key (git commits, github commit events); URL is the fallback
// (github issues, PRs, comments). Anything without either falls
// back to a composite of type+timestamp+message which is good
// enough to keep distinct rows distinct without collapsing
// conversation turns.
func dedupeKey(d RecentEvent) string {
	if d.SHA != "" {
		return "sha:" + d.SHA
	}
	if d.URL != "" {
		return "url:" + d.URL
	}
	return fmt.Sprintf("meta:%s:%d:%s", d.Type, d.TimestampMs, d.Message)
}

// fetchGithubThread pulls every doc belonging to a single github
// issue or PR thread. The query matches on repo + source=github
// and then a should clause over the two type/number combinations
// used by the github collector for that kind:
//
//	issue → (type=github.issue AND metadata.number=N)
//	       OR (type=github.issue_comment AND metadata.issue_number=N)
//	pr    → (type=github.pullrequest AND metadata.number=N)
//	       OR (type=github.pr_* AND metadata.pr_number=N)
//
// Workspace scoping matches QueryIndexedDocsForActivity so the
// expansion can't leak docs across workspaces.
func fetchGithubThread(
	ctx context.Context,
	workspaceID, repo, kind string,
	number, limit int,
) ([]RecentEvent, error) {
	if repo == "" || number <= 0 {
		return nil, fmt.Errorf("thread: repo/number required")
	}
	if limit <= 0 || limit > 500 {
		limit = 100
	}

	cfg, err := capability.LoadConfig()
	if err != nil {
		return nil, err
	}
	addr := cfg.Elasticsearch.Address
	if addr == "" {
		addr = "http://localhost:9200"
	}

	var typeShould string
	switch kind {
	case "issue":
		typeShould = fmt.Sprintf(`
			{ "bool": { "filter": [
				{ "term": { "type": "github.issue" } },
				{ "term": { "metadata.number": %d } }
			] } },
			{ "bool": { "filter": [
				{ "term": { "type": "github.issue_comment" } },
				{ "term": { "metadata.issue_number": %d } }
			] } }`, number, number)
	case "pr":
		typeShould = fmt.Sprintf(`
			{ "bool": { "filter": [
				{ "term": { "type": "github.pullrequest" } },
				{ "term": { "metadata.number": %d } }
			] } },
			{ "bool": { "filter": [
				{ "terms": { "type": ["github.pr_comment", "github.pr_review", "github.pr_check"] } },
				{ "term": { "metadata.pr_number": %d } }
			] } }`, number, number)
	default:
		return nil, fmt.Errorf("thread: unknown kind %q", kind)
	}

	var scopeFilter string
	if workspaceID == "" {
		scopeFilter = `{"match_all": {}}`
	} else {
		scopeFilter = fmt.Sprintf(`{
			"bool": {
				"should": [
					{ "terms": { "workspace_id": [%q, %q] } },
					{ "bool": { "must_not": { "exists": { "field": "workspace_id" } } } }
				],
				"minimum_should_match": 1
			}
		}`, workspaceID, capability.WorkspaceIDTools)
	}

	body := fmt.Sprintf(`{
		"size": %d,
		"sort": [{ "timestamp": "asc" }],
		"query": {
			"bool": {
				"filter": [
					{ "term": { "source": "github" } },
					{ "term": { "repo": %q } },
					%s,
					{ "bool": { "should": [ %s ], "minimum_should_match": 1 } }
				]
			}
		}
	}`, limit, repo, scopeFilter, typeShould)

	url := fmt.Sprintf("%s/glitch-events/_search", addr)
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader([]byte(body)))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	hc := &http.Client{Timeout: 5 * time.Second}
	resp, err := hc.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == 404 {
		return nil, nil
	}
	if resp.StatusCode >= 400 {
		raw, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("elasticsearch %d: %s", resp.StatusCode, strings.TrimSpace(string(raw)))
	}

	var parsed struct {
		Hits struct {
			Hits []struct {
				Source map[string]any `json:"_source"`
			} `json:"hits"`
		} `json:"hits"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return nil, err
	}

	out := make([]RecentEvent, 0, len(parsed.Hits.Hits))
	for _, h := range parsed.Hits.Hits {
		src := h.Source
		ev := RecentEvent{
			Type:    asString(src["type"]),
			Source:  asString(src["source"]),
			Repo:    asString(src["repo"]),
			Branch:  asString(src["branch"]),
			Author:  asString(src["author"]),
			SHA:     asString(src["sha"]),
			Message: asString(src["message"]),
			// Full body here — no truncation at fetch time. Thread
			// expansion is explicitly trying to give the model the
			// whole conversation; cap decisions belong to the
			// prompt formatter where the total token budget is
			// known.
			Body: asString(src["body"]),
		}
		if raw, ok := src["files_changed"].([]any); ok {
			for _, f := range raw {
				if s, ok := f.(string); ok && s != "" {
					ev.Files = append(ev.Files, s)
				}
			}
		}
		if md, ok := src["metadata"].(map[string]any); ok {
			if u, ok := md["url"].(string); ok {
				ev.URL = u
			}
		}
		if ts, ok := src["timestamp"].(string); ok {
			if t, err := time.Parse(time.RFC3339Nano, ts); err == nil {
				ev.TimestampMs = t.UnixMilli()
			}
		}
		populateRefNumbers(&ev, src)
		out = append(out, ev)
	}
	return out, nil
}
