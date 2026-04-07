//go:build integration

package esearch

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

// TestEventsMappingFreshFromCode wipes glitch-events and asks
// EnsureIndices to recreate it from the mapping in mappings.go, then
// asserts that workspace_id and source are stored as keyword fields
// (not text). This is the integration check for the bug discovered
// after the desktop's collector double-up was fixed: the live cluster
// had `workspace_id: text` because the index had been auto-created
// by an early write before EnsureIndices ran, and ES dynamic mapping
// guesses text for unknown string fields.
//
// If this test fails, every workspace-scoped aggregation in the GUI
// breaks with "Fielddata is disabled on [workspace_id]".
func TestEventsMappingFreshFromCode(t *testing.T) {
	es := openLiveES(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Wipe only this index — we don't want to clobber unrelated
	// indices the cluster might be hosting.
	delReq, _ := http.NewRequestWithContext(ctx, "DELETE",
		"http://localhost:9200/"+IndexEvents, nil)
	delRes, err := http.DefaultClient.Do(delReq)
	if err == nil {
		delRes.Body.Close()
	}

	// EnsureIndices should now create glitch-events fresh, applying
	// the strict mapping in mappings.go.
	if err := es.EnsureIndices(ctx); err != nil {
		t.Fatalf("ensure indices: %v", err)
	}

	// Read back the mapping and assert the critical fields.
	getReq, _ := http.NewRequestWithContext(ctx, "GET",
		"http://localhost:9200/"+IndexEvents+"/_mapping", nil)
	getRes, err := http.DefaultClient.Do(getReq)
	if err != nil {
		t.Fatalf("read mapping: %v", err)
	}
	defer getRes.Body.Close()
	body, _ := io.ReadAll(getRes.Body)

	for _, field := range []string{
		`"workspace_id":{"type":"keyword"}`,
		`"source":{"type":"keyword"}`,
		`"type":{"type":"keyword"}`,
	} {
		if !strings.Contains(string(body), field) {
			t.Errorf("expected mapping fragment %s in %s", field, string(body))
		}
	}
}

// TestKibanaEnsureDataViews verifies the Kibana bootstrap path that
// runs from observer.Start: connect to Kibana, POST every canonical
// data view, then read them back via the saved-objects API and assert
// each one is present. Skipped when Kibana isn't reachable so the
// test still runs in ES-only environments.
func TestKibanaEnsureDataViews(t *testing.T) {
	kb := NewKibana("")
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	if err := kb.Ping(ctx); err != nil {
		t.Skipf("kibana not available on localhost:5601: %v", err)
	}

	if err := kb.EnsureDataViews(ctx); err != nil {
		t.Fatalf("ensure data views: %v", err)
	}

	// Read back the data views via the saved-objects API and assert
	// each canonical id is present. We hit the same /api/data_views
	// endpoint the verify-glitch-stack workflow uses, so this test
	// also acts as a smoke test for that workflow's probe.
	req, err := http.NewRequestWithContext(ctx, "GET",
		kb.addr+"/api/data_views", nil)
	if err != nil {
		t.Fatalf("build req: %v", err)
	}
	req.Header.Set("kbn-xsrf", "glitch")
	res, err := kb.hc.Do(req)
	if err != nil {
		t.Fatalf("list data views: %v", err)
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		t.Fatalf("list data views: status %d", res.StatusCode)
	}
	body, _ := io.ReadAll(res.Body)
	got := string(body)

	for _, want := range []string{
		"glitch-brain-decisions",
		"glitch-pipelines",
		"glitch-events",
		"glitch-summaries",
		"glitch-insights",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("data view %q missing from list", want)
		}
	}

	// Idempotency check: a second EnsureDataViews must succeed (the
	// 409 → success branch). Without this guard a Kibana minor that
	// changes the conflict status code would silently break daily
	// glitchd boots.
	if err := kb.EnsureDataViews(ctx); err != nil {
		t.Fatalf("ensure data views (second pass): %v", err)
	}
}

// openLiveES dials the local Elasticsearch on localhost:9200 (the
// docker-compose default) and skips the test if nothing is listening.
// We deliberately do NOT spin up a testcontainer here — gl1tch's whole
// dev story is "docker compose up", so the test should validate the
// real path the user takes, not a synthetic one.
func openLiveES(t *testing.T) *Client {
	t.Helper()
	es, err := New("")
	if err != nil {
		t.Skipf("esearch: %v", err)
	}
	if err := es.Ping(context.Background()); err != nil {
		t.Skipf("elasticsearch not available on localhost:9200: %v", err)
	}
	if err := es.EnsureIndices(context.Background()); err != nil {
		t.Fatalf("ensure indices: %v", err)
	}
	return es
}

// TestBrainDecisionsRoundTrip is the end-to-end smoke test for the
// brain-decisions index: it writes one document with every field set,
// reads it back via search, and asserts that the round-trip preserves
// types correctly. If this test passes, the Kibana dashboards built on
// the same field names will work too.
//
// Run with: go test -tags=integration ./internal/esearch/...
func TestBrainDecisionsRoundTrip(t *testing.T) {
	es := openLiveES(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Use a sentinel workspace id so the test doc is easy to find and
	// to delete afterwards without disturbing real decisions the user
	// has accumulated by running glitchd locally.
	const sentinelWorkspace = "__test_brain_decisions_roundtrip__"

	doc := BrainDecision{
		WorkspaceID:    sentinelWorkspace,
		Question:       "is the round-trip lossless?",
		ChosenProvider: "ollama",
		ChosenModel:    "llama3.2",
		AllProviders:   []string{"ollama", "claude"},
		AllModels:      []string{"llama3.2", "claude-sonnet-4-6"},
		Escalated:      true,
		Confidence:     0.42,
		Resolved:       false,
		Status:         "success",
		StepCount:      2,
		DurationMs:     1234,
		CostUSD:        0.0017,
		Timestamp:      time.Now().UTC(),
	}

	if err := es.Index(ctx, IndexBrainDecisions, "", doc); err != nil {
		t.Fatalf("index: %v", err)
	}

	// Refresh so the search sees the doc immediately. The shared
	// Index helper uses refresh:false to keep production writes fast,
	// so we have to force a refresh ourselves in the test.
	if _, err := es.es.Indices.Refresh(es.es.Indices.Refresh.WithIndex(IndexBrainDecisions)); err != nil {
		t.Fatalf("refresh: %v", err)
	}

	// Cleanup the sentinel before assertions so a failed assertion
	// doesn't leave junk in the local index between runs.
	t.Cleanup(func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_, _ = es.DeleteByQuery(
			cleanupCtx,
			[]string{IndexBrainDecisions},
			map[string]any{
				"term": map[string]any{"workspace_id": sentinelWorkspace},
			},
		)
	})

	resp, err := es.Search(ctx, []string{IndexBrainDecisions}, map[string]any{
		"size": 5,
		"query": map[string]any{
			"term": map[string]any{"workspace_id": sentinelWorkspace},
		},
	})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if resp.Total < 1 {
		t.Fatalf("expected at least 1 hit, got %d", resp.Total)
	}

	// Decode the first hit and assert every field that Kibana
	// dashboards depend on. If any of these fail, dashboards built
	// against the mapping will silently render empty.
	var got BrainDecision
	if err := json.Unmarshal(resp.Results[0].Source, &got); err != nil {
		t.Fatalf("decode hit: %v", err)
	}

	if got.ChosenProvider != "ollama" {
		t.Errorf("chosen_provider: got %q want ollama", got.ChosenProvider)
	}
	if got.ChosenModel != "llama3.2" {
		t.Errorf("chosen_model: got %q want llama3.2", got.ChosenModel)
	}
	if !got.Escalated {
		t.Errorf("escalated: got false want true")
	}
	if got.Status != "success" {
		t.Errorf("status: got %q want success", got.Status)
	}
	if got.StepCount != 2 {
		t.Errorf("step_count: got %d want 2", got.StepCount)
	}
	if got.DurationMs != 1234 {
		t.Errorf("duration_ms: got %d want 1234", got.DurationMs)
	}
	if got.Confidence < 0.41 || got.Confidence > 0.43 {
		t.Errorf("confidence: got %v want ~0.42", got.Confidence)
	}
	if len(got.AllProviders) != 2 {
		t.Errorf("all_providers: got %v want 2 entries", got.AllProviders)
	}
}

