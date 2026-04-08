package capability

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"
)

// IndexSecurity is the ES index holding gl1tch security events:
// ssh auth failures, sudo denials, new host key warnings, and
// anything else a local log forwarder decides to classify as
// security-relevant. Kept separate from glitch-events so a
// security-focused dashboard doesn't compete with the noisier
// collector stream.
const IndexSecurity = "glitch-security"

// SecurityAlertsCapability is an on-demand capability the assistant
// calls to answer "any security alerts?"-style questions. It queries
// the glitch-security ES index for high-severity events in a recent
// window and returns a human-readable summary.
//
// Kept intentionally narrow: no scoring, no dashboards, no
// correlation. The assistant is the presentation layer — this
// capability just turns "what happened recently" into text the model
// can reason about and relay to the user.
//
// Query behaviour:
//
//   - Window defaults to the last 24 hours; override per-call by
//     passing a duration string in Input.Stdin ("1h", "15m", "7d").
//   - Severity filter is >= "high". Informational log noise is
//     deliberately excluded so the assistant doesn't narrate every
//     successful login.
//   - Results are capped at 50 hits and sorted newest-first.
type SecurityAlertsCapability struct {
	ES Indexer // satisfied by *esearch.Client; nil = capability degrades to "ES unavailable"

	// Searcher is the search hook used by tests to avoid a real ES
	// server. Production leaves this nil and the capability falls
	// through to an *esearch.Client typed path via ESSearch.
	Searcher func(ctx context.Context, query map[string]any) ([]map[string]any, error)
}

// ESSearcher is the subset of esearch.Client the security capability
// actually needs at runtime. Having a narrow interface keeps the
// package from importing the full esearch client type here and lets
// tests inject a httptest-backed fake.
type ESSearcher interface {
	Search(ctx context.Context, indices []string, query map[string]any) (ESSearchResults, error)
}

// ESSearchResults is the flattened hit list the capability walks.
// Each element is the raw _source map of a matching document.
type ESSearchResults []map[string]any

func (s *SecurityAlertsCapability) Manifest() Manifest {
	return Manifest{
		Name: "security_alerts",
		Description: "List recent high-severity security events — ssh auth failures, sudo denials, " +
			"new host key warnings. Optional input: a duration like '1h' or '24h' " +
			"to override the default 24-hour window.",
		Category: "security",
		Trigger:  Trigger{Mode: TriggerOnDemand},
		Sink:     Sink{Stream: true},
	}
}

func (s *SecurityAlertsCapability) Invoke(ctx context.Context, in Input) (<-chan Event, error) {
	ch := make(chan Event, 4)
	go func() {
		defer close(ch)
		s.run(ctx, in, ch)
	}()
	return ch, nil
}

func (s *SecurityAlertsCapability) run(ctx context.Context, in Input, ch chan<- Event) {
	if s.Searcher == nil {
		ch <- Event{Kind: EventStream, Text: "security_alerts: no ES searcher configured — cannot query alerts"}
		return
	}

	window := 24 * time.Hour
	if trimmed := strings.TrimSpace(in.Stdin); trimmed != "" {
		if d, err := time.ParseDuration(trimmed); err == nil {
			window = d
		}
	}
	since := time.Now().Add(-window).UTC().Format(time.RFC3339)

	query := map[string]any{
		"size": 50,
		"sort": []any{
			map[string]any{"timestamp": map[string]any{"order": "desc"}},
		},
		"query": map[string]any{
			"bool": map[string]any{
				"filter": []any{
					map[string]any{
						"range": map[string]any{
							"timestamp": map[string]any{"gte": since},
						},
					},
					map[string]any{
						"terms": map[string]any{
							"severity": []string{"high", "critical"},
						},
					},
				},
			},
		},
	}

	hits, err := s.Searcher(ctx, query)
	if err != nil {
		ch <- Event{Kind: EventStream, Text: fmt.Sprintf("security_alerts: query failed: %v", err)}
		return
	}

	if len(hits) == 0 {
		ch <- Event{Kind: EventStream, Text: fmt.Sprintf("no security alerts in the last %s", window)}
		return
	}

	ch <- Event{Kind: EventStream, Text: formatSecurityAlerts(hits, window)}
}

// formatSecurityAlerts renders the hit list as a compact human
// summary. One line per alert, newest first, with severity and
// source-IP where available. The assistant takes this as a tool
// result and decides how to present it — we don't try to win on
// formatting here.
func formatSecurityAlerts(hits ESSearchResults, window time.Duration) string {
	type row struct {
		when     string
		severity string
		eventTyp string
		user     string
		sourceIP string
		message  string
	}

	rows := make([]row, 0, len(hits))
	for _, h := range hits {
		r := row{
			when:     stringField(h, "timestamp"),
			severity: stringField(h, "severity"),
			eventTyp: stringField(h, "event_type"),
			user:     stringField(h, "user"),
			sourceIP: stringField(h, "source_ip"),
			message:  stringField(h, "message"),
		}
		rows = append(rows, r)
	}
	// Defensive sort: ES should have honoured our desc sort but the
	// scripted test path feeds raw slices, so keep this local.
	sort.SliceStable(rows, func(i, j int) bool {
		return rows[i].when > rows[j].when
	})

	var sb strings.Builder
	fmt.Fprintf(&sb, "%d security alert(s) in the last %s:\n", len(rows), window)
	for _, r := range rows {
		line := fmt.Sprintf("- [%s] %s %s", r.severity, r.when, r.eventTyp)
		if r.user != "" {
			line += " user=" + r.user
		}
		if r.sourceIP != "" {
			line += " src=" + r.sourceIP
		}
		if r.message != "" {
			line += " — " + r.message
		}
		sb.WriteString(line)
		sb.WriteByte('\n')
	}
	return strings.TrimRight(sb.String(), "\n")
}

// stringField pulls a string out of an ES _source map, tolerating
// missing keys and non-string values. The ES response decoder hands
// us map[string]any so every lookup is a type assertion.
func stringField(doc map[string]any, key string) string {
	v, ok := doc[key]
	if !ok || v == nil {
		return ""
	}
	switch t := v.(type) {
	case string:
		return t
	case json.Number:
		return t.String()
	default:
		return fmt.Sprint(t)
	}
}
