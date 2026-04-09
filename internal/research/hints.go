package research

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"sync"
	"time"
)

// hints.go is the read side of the brain event log. The loop already
// writes research_attempt + research_score events to
// ~/.glitch/research_events.jsonl on every iteration. Until now nothing
// read them back. This file closes that loop:
//
//   1. HintsProvider is the seam the loop calls before every plan
//      stage to get a one-paragraph hint string.
//   2. FileEventHintsProvider implements it by scanning the JSONL file,
//      finding events whose questions overlap the current one (token
//      jaccard), and ranking past picks by composite confidence.
//   3. The hint goes into the planner template via {{.Hints}}, so the
//      planner sees "for past questions like 'PR updated', the picks
//      that landed were github-prs (avg 0.91, n=4)" and biases its
//      pick accordingly — without anyone touching Go.
//
// This is the first instance of glitch reading its own brain. The
// "AI-first, nothing hardcoded" rule says routing decisions must come
// from the model + data, not Go switch statements. Past composite
// scores are data; the planner reading them is the model. No router
// table, no keyword list — just empirical evidence feeding the next
// prompt.

// HintsProvider returns a short hint string the planner template
// embeds in the {{.Hints}} slot. Implementations must be cheap
// (sub-100ms is the budget) because the hint runs before every plan
// call and the user is waiting on an interactive thread.
//
// An empty return is valid and means "no hints available" — the
// planner template's {{if .Hints}} block then renders nothing and the
// loop behaves exactly as it does without a provider.
type HintsProvider interface {
	Hints(ctx context.Context, question string) string
}

// nopHintsProvider is the default the loop uses when no caller has
// wired one up. Returns empty so the planner template hides its
// {{.Hints}} block. Defined unexported so callers who want "no
// hints" pass nil to WithHintsProvider rather than a sentinel.
type nopHintsProvider struct{}

func (nopHintsProvider) Hints(context.Context, string) string { return "" }

// FileEventHintsProvider reads JSONL research events from disk and
// builds a hint by ranking past picks for similar questions. Path
// defaults to ~/.glitch/research_events.jsonl (same default as
// FileEventSink so write and read line up automatically).
//
// The provider is intentionally pure-functional given a file: every
// Hints call re-reads the file and rebuilds the index. The cost is
// trivial (<<10ms for thousands of events) and the freshness gain is
// real — a research call can immediately benefit from the events the
// PREVIOUS research call just wrote, with no in-memory cache to
// invalidate.
type FileEventHintsProvider struct {
	Path string
	// MinSimilarity is the token-jaccard floor a past question must
	// clear to count as "similar enough". Defaults to 0.20 — low
	// enough that "did this PR get updated" matches "what PRs are
	// open" via the shared "pr" token, high enough that completely
	// unrelated questions don't pollute the hint.
	MinSimilarity float64
	// MinSamples is the minimum number of past observations a pick
	// combination must have before it appears in the hint. Defaults
	// to 1 — even a single past success is signal until enough
	// data accumulates to bump this up.
	MinSamples int
	// MaxAge is the lookback window. Older events are ignored.
	// Defaults to 30 days. Set to 0 for "look at everything".
	MaxAge time.Duration

	mu sync.Mutex
}

// NewFileEventHintsProvider constructs a provider with the conventional
// defaults. Pass an empty path to use ~/.glitch/research_events.jsonl.
func NewFileEventHintsProvider(path string) *FileEventHintsProvider {
	if path == "" {
		path = defaultEventPath()
	}
	return &FileEventHintsProvider{
		Path:          path,
		MinSimilarity: 0.20,
		MinSamples:    1,
		MaxAge:        30 * 24 * time.Hour,
	}
}

// Hints scans the event log and returns a hint string for the given
// question. Errors (file missing, parse failure, etc.) collapse to
// empty so the loop never fails over a missing brain — the planner
// just falls back to its default behavior.
func (f *FileEventHintsProvider) Hints(_ context.Context, question string) string {
	f.mu.Lock()
	defer f.mu.Unlock()

	data, err := os.ReadFile(f.Path)
	if err != nil {
		return ""
	}
	if len(data) == 0 {
		return ""
	}

	queryTokens := tokenise(question)
	if len(queryTokens) == 0 {
		return ""
	}

	cutoff := time.Time{}
	if f.MaxAge > 0 {
		cutoff = time.Now().Add(-f.MaxAge)
	}

	type sample struct {
		similarity float64
		composite  float64
		picks      []string
	}
	var samples []sample

	for _, line := range splitLines(data) {
		if len(line) == 0 {
			continue
		}
		var ev Event
		if err := json.Unmarshal(line, &ev); err != nil {
			continue
		}
		// We need attempt events because they carry the bundle,
		// and the bundle's Sources() is what gives us the picks.
		// Score events are bundle-free for size reasons.
		if ev.Type != EventTypeAttempt {
			continue
		}
		if ev.Bundle == nil || ev.Bundle.Len() == 0 {
			continue
		}
		if !cutoff.IsZero() {
			t, err := time.Parse(time.RFC3339, ev.Timestamp)
			if err == nil && t.Before(cutoff) {
				continue
			}
		}

		past := tokenise(ev.Question)
		if len(past) == 0 {
			continue
		}
		sim := jaccard(queryTokens, past)
		if sim < f.MinSimilarity {
			continue
		}

		samples = append(samples, sample{
			similarity: sim,
			composite:  ev.Score.Composite,
			picks:      ev.Bundle.Sources(),
		})
	}

	if len(samples) == 0 {
		return ""
	}

	// Group by sorted-pick tuple. The hint shows the top 2 groups by
	// average composite weighted by similarity, broken by min-samples.
	type key string
	type group struct {
		picks         []string
		count         int
		weightedScore float64
		weightSum     float64
	}
	groups := make(map[key]*group)
	for _, s := range samples {
		picks := append([]string(nil), s.picks...)
		sort.Strings(picks)
		k := key(strings.Join(picks, "+"))
		g, ok := groups[k]
		if !ok {
			g = &group{picks: picks}
			groups[k] = g
		}
		g.count++
		g.weightedScore += s.composite * s.similarity
		g.weightSum += s.similarity
	}

	type ranked struct {
		picks []string
		count int
		avg   float64
	}
	var ranks []ranked
	for _, g := range groups {
		if g.count < f.MinSamples {
			continue
		}
		avg := 0.0
		if g.weightSum > 0 {
			avg = g.weightedScore / g.weightSum
		}
		ranks = append(ranks, ranked{picks: g.picks, count: g.count, avg: avg})
	}
	if len(ranks) == 0 {
		return ""
	}
	sort.Slice(ranks, func(i, j int) bool {
		if ranks[i].avg != ranks[j].avg {
			return ranks[i].avg > ranks[j].avg
		}
		return ranks[i].count > ranks[j].count
	})

	// Render the top 2 groups as a compact bullet list. Keeping it
	// short avoids burying the planner's actual menu under a wall of
	// historical noise.
	const maxRows = 2
	if len(ranks) > maxRows {
		ranks = ranks[:maxRows]
	}
	var b strings.Builder
	for _, r := range ranks {
		fmt.Fprintf(&b, "- picks=[%s] avg_composite=%.2f n=%d\n",
			strings.Join(r.picks, ", "), r.avg, r.count)
	}
	return strings.TrimRight(b.String(), "\n")
}

// tokenise lowercases s and splits on every non-alphanumeric byte,
// then drops a tiny stopword list and any 1-character token. The
// stopword list is intentionally small — the goal is "let 'pr' and
// 'commits' carry signal", not to build a search engine.
func tokenise(s string) []string {
	var tokens []string
	var cur strings.Builder
	flush := func() {
		if cur.Len() == 0 {
			return
		}
		tok := strings.ToLower(cur.String())
		cur.Reset()
		if len(tok) <= 1 {
			return
		}
		if hintStopwords[tok] {
			return
		}
		tokens = append(tokens, tok)
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		isAlnum := (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9')
		if isAlnum {
			cur.WriteByte(c)
			continue
		}
		flush()
	}
	flush()
	// De-dup while preserving order so jaccard sees a set, not a bag.
	seen := make(map[string]struct{}, len(tokens))
	uniq := tokens[:0]
	for _, t := range tokens {
		if _, ok := seen[t]; ok {
			continue
		}
		seen[t] = struct{}{}
		uniq = append(uniq, t)
	}
	return uniq
}

// hintStopwords is the small list we drop because the words carry no
// routing signal. Adding more words risks losing real signal — we'd
// rather have a noisier hint than a wrong-but-confident one.
var hintStopwords = map[string]bool{
	"the": true, "is": true, "are": true, "was": true, "were": true,
	"this": true, "that": true, "what": true, "who": true, "when": true,
	"where": true, "why": true, "how": true, "do": true, "does": true,
	"did": true, "you": true, "me": true, "my": true, "of": true,
	"to": true, "in": true, "on": true, "at": true, "and": true,
	"or": true, "for": true, "from": true, "as": true, "by": true,
	"with": true, "an": true, "be": true, "have": true, "has": true,
	"had": true, "it": true, "its": true, "any": true, "some": true,
	"all": true, "no": true, "yes": true, "yet": true,
}

// jaccard returns the size of the intersection over the size of the
// union of two token sets. Both inputs must be deduplicated (tokenise
// guarantees that). Range [0, 1].
func jaccard(a, b []string) float64 {
	if len(a) == 0 || len(b) == 0 {
		return 0
	}
	set := make(map[string]struct{}, len(a))
	for _, t := range a {
		set[t] = struct{}{}
	}
	intersect := 0
	for _, t := range b {
		if _, ok := set[t]; ok {
			intersect++
		}
	}
	union := len(a) + len(b) - intersect
	if union == 0 {
		return 0
	}
	return float64(intersect) / float64(union)
}

// splitLines splits the event-log byte slice on newlines, returning
// the raw lines without copies. Used by Hints instead of bufio so the
// scanner doesn't allocate per-line.
func splitLines(data []byte) [][]byte {
	var out [][]byte
	start := 0
	for i := 0; i < len(data); i++ {
		if data[i] == '\n' {
			out = append(out, data[start:i])
			start = i + 1
		}
	}
	if start < len(data) {
		out = append(out, data[start:])
	}
	return out
}
