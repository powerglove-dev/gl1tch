package glitchd

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestParseGitHubHandleFromEmail(t *testing.T) {
	cases := map[string]string{
		"51892+adam-stokes@users.noreply.github.com": "adam-stokes",
		"1234+octocat@users.noreply.github.com":      "octocat",
		"octocat@users.noreply.github.com":           "octocat", // old format
		"  adam@users.noreply.github.com  ":          "adam",
		"adam@example.com":                           "", // not a noreply
		"":                                           "",
		"garbage":                                    "",
		"+no-id@users.noreply.github.com":            "no-id",
	}
	for in, want := range cases {
		if got := parseGitHubHandleFromEmail(in); got != want {
			t.Errorf("parseGitHubHandleFromEmail(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestNormalizeAttentionLevel(t *testing.T) {
	cases := map[string]AttentionLevel{
		"high":      AttentionHigh,
		"HIGH":      AttentionHigh,
		" high  ":   AttentionHigh,
		"normal":    AttentionNormal,
		"low":       AttentionLow,
		"":          AttentionNormal,
		"urgent":    AttentionNormal, // unknown → safe middle
		"critical":  AttentionNormal,
	}
	for in, want := range cases {
		if got := normalizeAttentionLevel(in); got != want {
			t.Errorf("normalizeAttentionLevel(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestParseClassifierResponse_HappyPath(t *testing.T) {
	raw := `{
		"verdicts": [
			{"index": 0, "level": "high",   "reason": "review on my PR"},
			{"index": 1, "level": "normal", "reason": "unrelated commit"},
			{"index": 2, "level": "low",    "reason": "dependabot bump"}
		]
	}`
	got, err := parseClassifierResponse(raw, 3)
	if err != nil {
		t.Fatalf("parseClassifierResponse: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("want 3 verdicts, got %d", len(got))
	}
	if got[0].Level != "high" || got[0].Index != 0 {
		t.Errorf("verdict[0]: %+v", got[0])
	}
	if got[2].Level != "low" {
		t.Errorf("verdict[2] level: %q", got[2].Level)
	}
}

func TestParseClassifierResponse_DropsOutOfRangeIndices(t *testing.T) {
	raw := `{
		"verdicts": [
			{"index": 0,  "level": "high",   "reason": "ok"},
			{"index": 99, "level": "high",   "reason": "out of range"},
			{"index": -1, "level": "low",    "reason": "negative"}
		]
	}`
	got, err := parseClassifierResponse(raw, 2)
	if err != nil {
		t.Fatalf("parseClassifierResponse: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("want 1 in-range verdict, got %d", len(got))
	}
	if got[0].Index != 0 {
		t.Errorf("surviving verdict should be index 0, got %d", got[0].Index)
	}
}

func TestParseClassifierResponse_EmptyErrors(t *testing.T) {
	if _, err := parseClassifierResponse("", 3); err == nil {
		t.Errorf("empty response should error")
	}
	if _, err := parseClassifierResponse("   ", 3); err == nil {
		t.Errorf("whitespace response should error")
	}
}

func TestParseClassifierResponse_MalformedJSONErrors(t *testing.T) {
	if _, err := parseClassifierResponse("{not json", 3); err == nil {
		t.Errorf("malformed json should error")
	}
}

func TestMarshalEventsForClassifier_TruncatesBodies(t *testing.T) {
	longBody := strings.Repeat("x", 600)
	batch := []AnalyzableEvent{
		{Source: "github", Type: "github.pr", Title: "t", Body: longBody},
	}
	out, err := marshalEventsForClassifier(batch)
	if err != nil {
		t.Fatalf("marshalEventsForClassifier: %v", err)
	}
	// 400 char cap + truncation ellipsis.
	if !strings.Contains(out, "…") {
		t.Errorf("long body should be truncated with ellipsis")
	}
	// The full 600-char body must not appear.
	if strings.Contains(out, longBody) {
		t.Errorf("full 600-char body leaked into classifier input")
	}
}

// TestClassifyAttention_OllamaUnreachableDegradesToNormal verifies
// the "never block the sink" contract: when Ollama is down the
// classifier returns a verdict per input event, all normal, with no
// error bubbled up.
func TestClassifyAttention_OllamaUnreachableDegradesToNormal(t *testing.T) {
	_, _ = withIsolatedPromptEnv(t)

	// Redirect to an intentionally-dead port so the HTTP call
	// fails fast. 127.0.0.1:1 is the canonical "nothing listens
	// here" address and rejects connections immediately.
	t.Setenv("GLITCH_OLLAMA_URL", "http://127.0.0.1:1")

	events := []AnalyzableEvent{
		{Source: "github", Type: "github.pr", Title: "PR one"},
		{Source: "git", Type: "git.commit", Title: "commit two"},
	}

	verdicts, err := ClassifyAttention(t.Context(), events, "ws-test")
	if err != nil {
		t.Fatalf("ClassifyAttention should not error on unreachable ollama: %v", err)
	}
	if len(verdicts) != len(events) {
		t.Fatalf("want %d verdicts, got %d", len(events), len(verdicts))
	}
	for i, v := range verdicts {
		if v.Level != AttentionNormal {
			t.Errorf("verdict[%d] should default to normal, got %q", i, v.Level)
		}
		if v.Index != i {
			t.Errorf("verdict[%d] index should equal %d, got %d", i, i, v.Index)
		}
	}
}

func TestClassifyAttention_EmptyInputReturnsNil(t *testing.T) {
	_, _ = withIsolatedPromptEnv(t)
	verdicts, err := ClassifyAttention(t.Context(), nil, "ws-test")
	if err != nil || verdicts != nil {
		t.Errorf("empty input: got (%v, %v), want (nil, nil)", verdicts, err)
	}
}

// TestClassifyAttention_MockedOllamaFlagsHigh exercises the full
// happy path by standing up a fake ollama HTTP server and pointing
// the classifier at it via GLITCH_OLLAMA_URL. Drives the full
// marshal → http → parse → normalize pipeline with a representative
// JSON payload.
//
// Batch size was reduced to 1 per attention.go's attentionMaxBatch
// constant so every event is its own HTTP call. The mock here
// tracks call count and returns a different verdict per call so
// the test can distinguish "first event classified high" from
// "second event classified low".
func TestClassifyAttention_MockedOllamaFlagsHigh(t *testing.T) {
	_, _ = withIsolatedPromptEnv(t)

	var (
		callCount int
		gotPath   string
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		// With attentionMaxBatch=1 each classified event gets its
		// own POST. Alternate the canned response per call so we
		// can check that ClassifyAttention re-bases verdicts onto
		// the right global index.
		var responseBody string
		if callCount == 0 {
			responseBody = `{"verdicts":[{"index":0,"level":"high","reason":"review on your PR"}]}`
		} else {
			responseBody = `{"verdicts":[{"index":0,"level":"low","reason":"dependabot"}]}`
		}
		callCount++
		resp := map[string]any{"response": responseBody}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()
	t.Setenv("GLITCH_OLLAMA_URL", srv.URL)

	events := []AnalyzableEvent{
		{Source: "github", Type: "github.pr", Title: "#1246 feat"},
		{Source: "github", Type: "github.pr", Author: "dependabot[bot]",
			Title: "bump x from 1.0 to 1.1"},
	}

	verdicts, err := ClassifyAttention(t.Context(), events, "ws-test")
	if err != nil {
		t.Fatalf("ClassifyAttention: %v", err)
	}
	if gotPath != "/api/generate" {
		t.Errorf("expected POST to /api/generate, got %q", gotPath)
	}
	if callCount != 2 {
		t.Errorf("expected 2 HTTP calls (one per event at batch=1), got %d", callCount)
	}
	if len(verdicts) != 2 {
		t.Fatalf("want 2 verdicts, got %d", len(verdicts))
	}
	if verdicts[0].Level != AttentionHigh {
		t.Errorf("verdict[0] level: want high, got %q", verdicts[0].Level)
	}
	if verdicts[1].Level != AttentionLow {
		t.Errorf("verdict[1] level: want low, got %q", verdicts[1].Level)
	}
	if !strings.Contains(verdicts[0].Reason, "review on your PR") {
		t.Errorf("verdict[0] reason: %q", verdicts[0].Reason)
	}
}
