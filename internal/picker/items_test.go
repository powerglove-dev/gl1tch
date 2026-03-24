package picker_test

import (
	"testing"

	"github.com/adam-stokes/orcai/internal/picker"
)

func TestApplyFuzzy_EmptyQuery(t *testing.T) {
	items := []picker.PickerItem{
		{Kind: "skill", Name: "golang-patterns", Description: "Idiomatic Go"},
		{Kind: "skill", Name: "golang-testing", Description: "Go testing"},
	}
	got := picker.ApplyFuzzy("", items)
	if len(got) != 2 {
		t.Fatalf("empty query: want 2 items, got %d", len(got))
	}
	for _, item := range got {
		if len(item.MatchIndexes()) != 0 {
			t.Errorf("empty query: expected no match indexes, got %v", item.MatchIndexes())
		}
	}
}

func TestApplyFuzzy_FiltersMatches(t *testing.T) {
	items := []picker.PickerItem{
		{Kind: "skill", Name: "golang-patterns", Description: "Idiomatic Go"},
		{Kind: "agent", Name: "beast-mode", Description: "coding agent"},
		{Kind: "pipeline", Name: "research-pipeline", Description: "research"},
	}
	got := picker.ApplyFuzzy("beast", items)
	if len(got) != 1 {
		t.Fatalf("want 1 match for 'beast', got %d", len(got))
	}
	if got[0].Name != "beast-mode" {
		t.Errorf("want beast-mode, got %q", got[0].Name)
	}
}

func TestApplyFuzzy_MatchIndexesSet(t *testing.T) {
	items := []picker.PickerItem{
		{Kind: "skill", Name: "golang-patterns", Description: ""},
	}
	got := picker.ApplyFuzzy("go", items)
	if len(got) == 0 {
		t.Fatal("expected match for 'go'")
	}
	if len(got[0].MatchIndexes()) == 0 {
		t.Error("expected match indexes to be set after fuzzy match")
	}
}

func TestApplyFuzzy_NoMatch(t *testing.T) {
	items := []picker.PickerItem{
		{Kind: "skill", Name: "golang-patterns", Description: ""},
	}
	got := picker.ApplyFuzzy("zzzzz", items)
	if len(got) != 0 {
		t.Errorf("expected no matches, got %d", len(got))
	}
}

func TestBuildPickerItems_HasProviders(t *testing.T) {
	providers := []picker.ProviderDef{
		{ID: "claude", Label: "Claude", Models: []picker.ModelOption{{ID: "claude-sonnet-4-6", Label: "Sonnet"}}},
		{ID: "shell", Label: "Shell"},
	}
	items := picker.BuildPickerItems(nil, providers, "/tmp", "/tmp")
	var found int
	for _, item := range items {
		if item.Kind == "provider" {
			found++
		}
	}
	if found != 2 {
		t.Errorf("want 2 provider items, got %d", found)
	}
}

func TestBuildPickerItems_SessionsFirst(t *testing.T) {
	sessions := []picker.WindowEntry{{Index: "1", Name: "claude-1"}}
	items := picker.BuildPickerItems(sessions, nil, "/tmp", "/tmp")
	if len(items) == 0 {
		t.Fatal("expected items")
	}
	if items[0].Kind != "session" {
		t.Errorf("first item should be session, got %q", items[0].Kind)
	}
}

func TestBuildPickerItems_ProvidersLast(t *testing.T) {
	providers := []picker.ProviderDef{{ID: "shell", Label: "Shell"}}
	items := picker.BuildPickerItems(nil, providers, "/tmp", "/tmp")
	if len(items) == 0 {
		t.Fatal("expected items")
	}
	last := items[len(items)-1]
	if last.Kind != "provider" {
		t.Errorf("last item group should be provider, got %q", last.Kind)
	}
}

func TestPickerItem_FilterString(t *testing.T) {
	item := picker.PickerItem{Kind: "skill", Name: "beast-mode", Description: "top-notch coding agent"}
	got := item.Filter()
	want := "beast-mode top-notch coding agent"
	if got != want {
		t.Errorf("Filter() = %q, want %q", got, want)
	}
}
