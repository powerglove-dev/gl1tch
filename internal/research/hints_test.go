package research

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"
)

// TestTokenise covers the tiny lowercase + non-alnum + stopword
// pipeline. The hint quality depends on this — a stopword leak
// would inflate jaccard for unrelated questions and pollute the
// planner with noise.
func TestTokenise(t *testing.T) {
	cases := map[string]struct {
		in   string
		want []string
	}{
		"basic":         {"What PRs are open?", []string{"prs", "open"}},
		"hyphenated":    {"git-log shows recent commits", []string{"git", "log", "shows", "recent", "commits"}},
		"dedup":         {"PR pr Pr open OPEN", []string{"pr", "open"}},
		"empty":         {"", nil},
		"all stopwords": {"is the of and", nil},
		"single chars":  {"a b c hi", []string{"hi"}},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			got := tokenise(tc.in)
			if !equalSlice(got, tc.want) {
				t.Errorf("tokenise(%q) = %v, want %v", tc.in, got, tc.want)
			}
		})
	}
}

// TestJaccard covers the set-similarity primitive. Identical sets
// score 1.0, disjoint score 0.0, partial overlap is intersection /
// union with no surprises.
func TestJaccard(t *testing.T) {
	cases := map[string]struct {
		a, b []string
		want float64
	}{
		"identical":     {[]string{"pr", "open"}, []string{"pr", "open"}, 1.0},
		"disjoint":      {[]string{"pr", "open"}, []string{"commit", "log"}, 0.0},
		"half overlap":  {[]string{"pr", "open"}, []string{"pr", "log"}, 1.0 / 3.0},
		"empty a":       {nil, []string{"pr"}, 0.0},
		"empty b":       {[]string{"pr"}, nil, 0.0},
		"both empty":    {nil, nil, 0.0},
		"single shared": {[]string{"pr", "open", "yet"}, []string{"pr", "log", "diff"}, 1.0 / 5.0},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			got := jaccard(tc.a, tc.b)
			if abs(got-tc.want) > 1e-9 {
				t.Errorf("jaccard(%v, %v) = %v, want %v", tc.a, tc.b, got, tc.want)
			}
		})
	}
}

// TestFileEventHintsProvider_RanksByComposite is the end-to-end
// happy path: write three synthetic past events, two for "pr"
// questions and one for "commit", call Hints with a new "pr"
// question, assert the github-prs pick is in the returned hint and
// the git-log-only pick is not.
func TestFileEventHintsProvider_RanksByComposite(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "events.jsonl")

	now := time.Now()
	events := []Event{
		// Two past attempts on PR-shaped questions, one with a
		// strong github-prs pick and one with a weaker git-log
		// pick. The provider should rank github-prs higher.
		{
			Type:      EventTypeAttempt,
			Timestamp: now.Format(time.RFC3339),
			Question:  "what prs are open right now",
			Score:     Score{Composite: 0.92},
			Bundle:    bundleFromSources("github-prs"),
		},
		{
			Type:      EventTypeAttempt,
			Timestamp: now.Format(time.RFC3339),
			Question:  "did this pr get updated yet",
			Score:     Score{Composite: 0.88},
			Bundle:    bundleFromSources("github-prs"),
		},
		{
			Type:      EventTypeAttempt,
			Timestamp: now.Format(time.RFC3339),
			Question:  "show me recent commits",
			Score:     Score{Composite: 0.40},
			Bundle:    bundleFromSources("git-log"),
		},
	}
	writeEvents(t, path, events)

	provider := NewFileEventHintsProvider(path)
	hint := provider.Hints(context.Background(), "is this pr updated")
	if hint == "" {
		t.Fatalf("expected non-empty hint, got empty")
	}
	if !strings.Contains(hint, "github-prs") {
		t.Errorf("hint should mention github-prs, got: %s", hint)
	}
	// The git-log event's question shares no PR-related tokens with
	// "is this pr updated", so jaccard should drop it below the
	// MinSimilarity floor and the hint should not mention it.
	if strings.Contains(hint, "git-log") {
		t.Errorf("hint should not mention git-log for a PR question, got: %s", hint)
	}
}

// TestFileEventHintsProvider_EmptyOnNoFile covers the "no brain yet"
// path: a fresh install has no events file, and Hints must return
// empty without erroring so the loop falls through to the
// no-hint default.
func TestFileEventHintsProvider_EmptyOnNoFile(t *testing.T) {
	provider := NewFileEventHintsProvider(filepath.Join(t.TempDir(), "missing.jsonl"))
	hint := provider.Hints(context.Background(), "anything")
	if hint != "" {
		t.Errorf("hint on missing file should be empty, got: %q", hint)
	}
}

// TestFileEventHintsProvider_HonoursMaxAge writes one old event and
// one fresh event, asserts only the fresh one shows up in the hint.
func TestFileEventHintsProvider_HonoursMaxAge(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "events.jsonl")

	old := time.Now().Add(-90 * 24 * time.Hour).Format(time.RFC3339)
	fresh := time.Now().Format(time.RFC3339)

	events := []Event{
		{
			Type:      EventTypeAttempt,
			Timestamp: old,
			Question:  "what prs are open",
			Score:     Score{Composite: 0.99},
			Bundle:    bundleFromSources("github-prs-OLD"),
		},
		{
			Type:      EventTypeAttempt,
			Timestamp: fresh,
			Question:  "what prs are open",
			Score:     Score{Composite: 0.50},
			Bundle:    bundleFromSources("github-prs-FRESH"),
		},
	}
	writeEvents(t, path, events)

	provider := NewFileEventHintsProvider(path) // default 30d window
	hint := provider.Hints(context.Background(), "what prs are open")
	if !strings.Contains(hint, "github-prs-FRESH") {
		t.Errorf("hint should mention fresh source, got: %s", hint)
	}
	if strings.Contains(hint, "github-prs-OLD") {
		t.Errorf("hint should NOT mention old source past MaxAge, got: %s", hint)
	}
}

// TestFileEventHintsProvider_GroupsByPickCombination verifies the
// grouping logic: two events with the same pick set average their
// composites; events with different pick sets land in separate groups.
func TestFileEventHintsProvider_GroupsByPickCombination(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "events.jsonl")

	now := time.Now().Format(time.RFC3339)
	events := []Event{
		{Type: EventTypeAttempt, Timestamp: now, Question: "open prs", Score: Score{Composite: 0.9}, Bundle: bundleFromSources("github-prs")},
		{Type: EventTypeAttempt, Timestamp: now, Question: "any open prs", Score: Score{Composite: 0.8}, Bundle: bundleFromSources("github-prs")},
		{Type: EventTypeAttempt, Timestamp: now, Question: "show open prs and commits", Score: Score{Composite: 0.7}, Bundle: bundleFromSources("github-prs", "git-log")},
	}
	writeEvents(t, path, events)

	provider := NewFileEventHintsProvider(path)
	hint := provider.Hints(context.Background(), "are there open prs?")
	if !strings.Contains(hint, "n=2") {
		t.Errorf("github-prs alone group should report n=2 from two past events, got: %s", hint)
	}
}

// ── helpers ─────────────────────────────────────────────────────────────────

func bundleFromSources(sources ...string) *EvidenceBundle {
	b := &EvidenceBundle{}
	for _, s := range sources {
		b.Add(Evidence{Source: s, Title: s, Body: s + " body"})
	}
	return b
}

func writeEvents(t *testing.T, path string, events []Event) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("create events file: %v", err)
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	for _, e := range events {
		if err := enc.Encode(e); err != nil {
			t.Fatalf("encode event: %v", err)
		}
	}
}

func equalSlice(a, b []string) bool {
	if len(a) == 0 && len(b) == 0 {
		return true
	}
	ac := append([]string(nil), a...)
	bc := append([]string(nil), b...)
	sort.Strings(ac)
	sort.Strings(bc)
	return reflect.DeepEqual(ac, bc)
}

func abs(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}
