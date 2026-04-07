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
// Equivalent to QueryCollectorActivityScoped(ctx, "") — global, no
// workspace filter. Kept as a convenience wrapper for callers that
// don't yet pass a workspace id.
//
// Uses the raw HTTP API (no esearch.Client) so the desktop binary
// doesn't pull the heavy ES client into its bundle. observer.yaml's
// elasticsearch.address is honored.
func QueryCollectorActivity(ctx context.Context) ([]CollectorActivity, error) {
	return QueryCollectorActivityScoped(ctx, "")
}

// QueryCollectorActivityScoped is the workspace-aware variant. When
// workspaceID is non-empty, the aggregation is filtered to events
// with that workspace_id, so the brain popover for workspace `robots`
// shows only `robots`-attributed activity instead of the global sum.
//
// The "tool pod" bucket (collector.WorkspaceIDTools) is OR-included
// alongside the active workspace so global tool collectors (copilot,
// mattermost) still surface in the popover. Their numbers are
// identical across every workspace because the underlying data
// genuinely is shared — that's a true reflection of the source, not
// a re-indexing bug.
//
// An empty workspaceID returns the global view (every event in
// glitch-events) — used during startup before any workspace is
// active and as the legacy entry-point for the headless `glitch
// serve` path.
func QueryCollectorActivityScoped(ctx context.Context, workspaceID string) ([]CollectorActivity, error) {
	cfg, err := collector.LoadConfig()
	if err != nil {
		return nil, err
	}
	addr := cfg.Elasticsearch.Address
	if addr == "" {
		addr = "http://localhost:9200"
	}

	// Aggregation: group by source, get count + max(timestamp).
	//
	// `source` is mapped directly as a keyword field in eventsMapping
	// (internal/esearch/mappings.go), NOT as text-with-.keyword-subfield.
	// An earlier version of this query used `source.keyword` based on
	// a stale assumption about the mapping; that returned an empty
	// bucket list silently and made the brain popover render
	// "TOTAL INDEXED 0" even though the events index had hundreds of
	// thousands of docs across multiple collectors. Verified against
	// the live cluster: aggregating on `source` returns real buckets,
	// `source.keyword` returns nothing.
	//
	// Scoped query shape (workspaceID != ""): a bool-should that
	// matches THREE buckets, OR'd together:
	//
	//   1. workspace_id == active workspace
	//   2. workspace_id == collector.WorkspaceIDTools (the global
	//      tool pod's sentinel for copilot/mattermost — shared
	//      across every workspace by design)
	//   3. workspace_id field is MISSING ENTIRELY (legacy /
	//      unattributed docs)
	//
	// Bucket #3 is the bug fix that put this comment here. The
	// Event struct serializes WorkspaceID with `omitempty`, so any
	// doc indexed before the workspace_id stamping was added — and
	// every doc indexed via the cmd/observe.go IngestAll one-shot
	// path which calls BulkIndex directly without StampWorkspaceID
	// — lands in ES with NO `workspace_id` field at all. The old
	// terms-only filter couldn't match a missing field, so those
	// docs were invisible to the popover even though the unscoped
	// activity log saw them just fine. Symptom: copilot row showed
	// "0 indexed" while the activity log right below it logged
	// "indexed 3697 new doc(s) since last poll · 3697 total". The
	// scoped query was hiding the existing data, not failing to
	// find it.
	//
	// We treat missing-field as "global / unattributed, visible to
	// every workspace" rather than backfilling a workspace_id we
	// can't reliably guess. New docs from properly-stamped
	// collectors keep landing under their real workspace and the
	// merge happens at query time.
	//
	// ES short-circuits all three branches so this is still
	// essentially free even on hundreds of thousands of docs.
	var body string
	if workspaceID == "" {
		body = `{
			"size": 0,
			"aggs": {
				"by_source": {
					"terms": { "field": "source", "size": 50 },
					"aggs": {
						"last_seen": { "max": { "field": "timestamp" } }
					}
				}
			}
		}`
	} else {
		body = fmt.Sprintf(`{
			"size": 0,
			"query": {
				"bool": {
					"should": [
						{ "terms": { "workspace_id": [%q, %q] } },
						{ "bool": { "must_not": { "exists": { "field": "workspace_id" } } } }
					],
					"minimum_should_match": 1
				}
			},
			"aggs": {
				"by_source": {
					"terms": { "field": "source", "size": 50 },
					"aggs": {
						"last_seen": { "max": { "field": "timestamp" } }
					}
				}
			}
		}`, workspaceID, collector.WorkspaceIDTools)
	}

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

// QueryCodeIndexActivityScoped counts the chunks in glitch-vectors
// that belong to any of the given workspace directories. Each dir is
// translated into a brainrag scope of the form "cwd:<abs path>", and
// the query OR-includes all of them in a single ES request.
//
// Returns a single CollectorActivity-shaped row (Source = "code-index")
// so the brain popover's existing merge path can stamp it onto the
// code-index collector row without any popover-side schema changes.
//
// Empty dirs returns a zero-valued snapshot. Code-index data lives in
// a separate index from glitch-events, so the existing source
// aggregation in QueryCollectorActivityScoped can never find it —
// this helper exists specifically to bridge that gap for the popover.
func QueryCodeIndexActivityScoped(ctx context.Context, dirs []string) (CollectorActivity, error) {
	out := CollectorActivity{Source: "code-index"}
	if len(dirs) == 0 {
		return out, nil
	}

	cfg, err := collector.LoadConfig()
	if err != nil {
		return out, err
	}
	addr := cfg.Elasticsearch.Address
	if addr == "" {
		addr = "http://localhost:9200"
	}

	// Build the list of "cwd:<abs>" scopes brainrag uses for code
	// chunks. Same prefix as brainrag.NewRAGStoreForCWD so the
	// scopes match what the collector wrote.
	scopes := make([]string, 0, len(dirs))
	for _, d := range dirs {
		if d == "" {
			continue
		}
		scopes = append(scopes, "cwd:"+d)
	}
	if len(scopes) == 0 {
		return out, nil
	}

	scopesJSON, _ := json.Marshal(scopes)
	body := fmt.Sprintf(`{
		"size": 0,
		"query": {
			"terms": { "scope": %s }
		},
		"aggs": {
			"last_seen": { "max": { "field": "indexed_at" } }
		}
	}`, scopesJSON)

	url := fmt.Sprintf("%s/glitch-vectors/_search", addr)
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader([]byte(body)))
	if err != nil {
		return out, err
	}
	req.Header.Set("Content-Type", "application/json")

	hc := &http.Client{Timeout: 3 * time.Second}
	resp, err := hc.Do(req)
	if err != nil {
		return out, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == 404 {
		// Index doesn't exist yet — collector hasn't run.
		return out, nil
	}
	if resp.StatusCode >= 400 {
		raw, _ := io.ReadAll(resp.Body)
		return out, fmt.Errorf("elasticsearch %d: %s", resp.StatusCode, string(raw))
	}

	var parsed struct {
		Hits struct {
			Total struct {
				Value int64 `json:"value"`
			} `json:"total"`
		} `json:"hits"`
		Aggregations struct {
			LastSeen struct {
				Value float64 `json:"value"`
			} `json:"last_seen"`
		} `json:"aggregations"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return out, err
	}

	out.TotalDocs = parsed.Hits.Total.Value
	out.LastSeenMs = int64(parsed.Aggregations.LastSeen.Value)
	return out, nil
}

// BrainDecisionsActivity is a one-shot snapshot of the per-workspace
// brain decisions log. Powers the DECISIONS section of the brain
// popover so the user can see at a glance how often the brain is
// staying local vs. escalating to a paid model.
//
// All counts are scoped to a single workspace when WorkspaceID is
// non-empty; an empty WorkspaceID returns the global view (used at
// startup before any workspace is active).
type BrainDecisionsActivity struct {
	Total           int64  `json:"total"`
	Escalated       int64  `json:"escalated"`
	LastDecisionMs  int64  `json:"last_decision_ms,omitempty"`
	LastProvider    string `json:"last_provider,omitempty"`
	LastEscalated   bool   `json:"last_escalated,omitempty"`
}

// QueryBrainDecisionsActivity returns the global decisions snapshot.
// Equivalent to QueryBrainDecisionsActivityScoped(ctx, "").
func QueryBrainDecisionsActivity(ctx context.Context) (*BrainDecisionsActivity, error) {
	return QueryBrainDecisionsActivityScoped(ctx, "")
}

// QueryBrainDecisionsActivityScoped runs a single ES aggregation
// against glitch-brain-decisions: total count, escalated count, and
// the most recent decision's provider/escalated state. Cheap enough
// to call on every popover open without caching — one query, two
// terms aggs, no scripting.
//
// Returns a zero-valued snapshot (not an error) when the index doesn't
// exist yet so the popover can render "0 decisions" instead of a
// scary error toast on a fresh install.
func QueryBrainDecisionsActivityScoped(ctx context.Context, workspaceID string) (*BrainDecisionsActivity, error) {
	cfg, err := collector.LoadConfig()
	if err != nil {
		return nil, err
	}
	addr := cfg.Elasticsearch.Address
	if addr == "" {
		addr = "http://localhost:9200"
	}

	// Query: optional workspace filter, then aggregate. We use a
	// top_hits sub-agg to grab the most-recent doc's provider and
	// escalated flag in the same round-trip — saves a second query
	// for the "last decision" field on the popover row.
	var query string
	if workspaceID == "" {
		query = `"match_all": {}`
	} else {
		query = fmt.Sprintf(`"term": { "workspace_id": %q }`, workspaceID)
	}
	body := fmt.Sprintf(`{
		"size": 0,
		"query": { %s },
		"aggs": {
			"escalated_count": {
				"filter": { "term": { "escalated": true } }
			},
			"latest": {
				"top_hits": {
					"size": 1,
					"sort": [{ "timestamp": { "order": "desc" } }],
					"_source": [ "chosen_provider", "escalated", "timestamp" ]
				}
			}
		}
	}`, query)

	url := fmt.Sprintf("%s/glitch-brain-decisions/_search", addr)
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
		// Index doesn't exist yet — no decisions logged. Return an
		// empty snapshot so the popover renders "0 decisions" cleanly.
		return &BrainDecisionsActivity{}, nil
	}
	if resp.StatusCode >= 400 {
		raw, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("elasticsearch %d: %s", resp.StatusCode, string(raw))
	}

	var parsed struct {
		Hits struct {
			Total struct {
				Value int64 `json:"value"`
			} `json:"total"`
		} `json:"hits"`
		Aggregations struct {
			EscalatedCount struct {
				DocCount int64 `json:"doc_count"`
			} `json:"escalated_count"`
			Latest struct {
				Hits struct {
					Hits []struct {
						Source struct {
							ChosenProvider string `json:"chosen_provider"`
							Escalated      bool   `json:"escalated"`
							Timestamp      string `json:"timestamp"`
						} `json:"_source"`
					} `json:"hits"`
				} `json:"hits"`
			} `json:"latest"`
		} `json:"aggregations"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return nil, err
	}

	out := &BrainDecisionsActivity{
		Total:     parsed.Hits.Total.Value,
		Escalated: parsed.Aggregations.EscalatedCount.DocCount,
	}
	if len(parsed.Aggregations.Latest.Hits.Hits) > 0 {
		latest := parsed.Aggregations.Latest.Hits.Hits[0].Source
		out.LastProvider = latest.ChosenProvider
		out.LastEscalated = latest.Escalated
		// timestamp is RFC3339; parse to ms epoch.
		if latest.Timestamp != "" {
			if t, terr := time.Parse(time.RFC3339Nano, latest.Timestamp); terr == nil {
				out.LastDecisionMs = t.UnixMilli()
			}
		}
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

// AddCollectorDirectory and RemoveCollectorDirectory used to write to
// the global observer.yaml when the desktop's "add directory" button
// was clicked. That leaked directories from one workspace into every
// other workspace's directory collector — the workspace-scoped
// collector split made directories per-workspace SQLite state, so
// these helpers were dropped. The desktop's AddWorkspaceDirectory /
// RemoveWorkspaceDirectory paths now write directly to the workspace
// store and restart the affected pod.

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
