package observer

import (
	"reflect"
	"testing"
)

// TestInjectScopeFilters locks the wire shape produced by the
// scope-filter helper. The shape is load-bearing — wrong filter
// JSON either rejects valid queries (false negatives) or worse,
// returns docs from other workspaces (false positives that
// silently leak data across workspace boundaries).
func TestInjectScopeFilters(t *testing.T) {
	t.Run("no filters is a no-op", func(t *testing.T) {
		original := map[string]any{
			"query": map[string]any{"match_all": map[string]any{}},
		}
		clone := map[string]any{
			"query": map[string]any{"match_all": map[string]any{}},
		}
		injectScopeFilters(clone, nil, "")
		if !reflect.DeepEqual(clone, original) {
			t.Errorf("query mutated despite no filters: %+v", clone)
		}
	})

	t.Run("repo only filter", func(t *testing.T) {
		query := map[string]any{
			"query": map[string]any{"match_all": map[string]any{}},
		}
		injectScopeFilters(query, []string{"gl1tch", "ensemble"}, "")

		bool_, ok := query["query"].(map[string]any)["bool"].(map[string]any)
		if !ok {
			t.Fatalf("expected bool wrapper, got %+v", query["query"])
		}
		filters := bool_["filter"].([]any)
		if len(filters) != 1 {
			t.Fatalf("filters = %d, want 1", len(filters))
		}
		terms := filters[0].(map[string]any)["terms"].(map[string]any)
		repos := terms["repo"].([]string)
		if len(repos) != 2 || repos[0] != "gl1tch" || repos[1] != "ensemble" {
			t.Errorf("repo terms = %v", repos)
		}
	})

	t.Run("workspace only filter", func(t *testing.T) {
		query := map[string]any{
			"query": map[string]any{"match_all": map[string]any{}},
		}
		injectScopeFilters(query, nil, "ws-123")

		bool_ := query["query"].(map[string]any)["bool"].(map[string]any)
		filters := bool_["filter"].([]any)
		if len(filters) != 1 {
			t.Fatalf("filters = %d, want 1", len(filters))
		}
		term := filters[0].(map[string]any)["term"].(map[string]any)
		if term["workspace_id"] != "ws-123" {
			t.Errorf("workspace_id term = %v", term)
		}
	})

	t.Run("both filters AND'd together", func(t *testing.T) {
		query := map[string]any{
			"query": map[string]any{"match_all": map[string]any{}},
		}
		injectScopeFilters(query, []string{"gl1tch"}, "ws-1")

		bool_ := query["query"].(map[string]any)["bool"].(map[string]any)
		filters := bool_["filter"].([]any)
		if len(filters) != 2 {
			t.Fatalf("filters = %d, want 2 (repo + workspace)", len(filters))
		}
		// Order is repo first, workspace second per the helper.
		if _, ok := filters[0].(map[string]any)["terms"]; !ok {
			t.Errorf("first filter should be repo terms: %+v", filters[0])
		}
		if _, ok := filters[1].(map[string]any)["term"]; !ok {
			t.Errorf("second filter should be workspace term: %+v", filters[1])
		}
	})

	t.Run("must clause preserves original query", func(t *testing.T) {
		original := map[string]any{"match_all": map[string]any{}}
		query := map[string]any{"query": original}
		injectScopeFilters(query, nil, "ws-1")

		bool_ := query["query"].(map[string]any)["bool"].(map[string]any)
		must := bool_["must"].([]any)
		if len(must) != 1 || !reflect.DeepEqual(must[0], original) {
			t.Errorf("must lost the original query: %+v", must)
		}
	})
}
