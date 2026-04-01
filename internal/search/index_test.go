package search_test

import (
	"testing"

	"github.com/8op-org/gl1tch/internal/chatui"
	"github.com/8op-org/gl1tch/internal/search"
)

func TestQueryFindsExactName(t *testing.T) {
	entries := []chatui.IndexEntry{
		{Name: "standup", Kind: "skill", Description: "Generate daily standup", Inject: "/standup"},
		{Name: "pr-reviewer", Kind: "agent", Description: "Review pull requests", Inject: "@pr-reviewer "},
	}
	results := search.Query(entries, "standup")
	if len(results) == 0 {
		t.Fatal("expected at least one result for 'standup'")
	}
	if results[0].Name != "standup" {
		t.Fatalf("expected standup, got %s", results[0].Name)
	}
}

func TestQueryFindsPartialDescription(t *testing.T) {
	entries := []chatui.IndexEntry{
		{Name: "standup", Kind: "skill", Description: "Generate daily standup summary from yesterday"},
		{Name: "pr-reviewer", Kind: "agent", Description: "Review pull requests for security"},
	}
	results := search.Query(entries, "security")
	if len(results) == 0 {
		t.Fatal("expected result for 'security'")
	}
	if results[0].Name != "pr-reviewer" {
		t.Fatalf("expected pr-reviewer, got %s", results[0].Name)
	}
}

func TestQueryEmptyReturnsAll(t *testing.T) {
	entries := []chatui.IndexEntry{
		{Name: "a", Kind: "skill"},
		{Name: "b", Kind: "skill"},
	}
	results := search.Query(entries, "")
	if len(results) != 2 {
		t.Fatalf("expected 2 results for empty query, got %d", len(results))
	}
}

func TestQueryNoMatchReturnsEmpty(t *testing.T) {
	entries := []chatui.IndexEntry{
		{Name: "standup", Kind: "skill", Description: "daily standup"},
	}
	results := search.Query(entries, "xyzzy")
	if len(results) != 0 {
		t.Fatalf("expected 0 results for nonsense query, got %d", len(results))
	}
}
