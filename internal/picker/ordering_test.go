package picker

import (
	"sort"
	"testing"
)

// TestProviderPriority_Order verifies the canonical ranking of known providers.
func TestProviderPriority_Order(t *testing.T) {
	rank := make(map[string]int, len(providerPriority))
	for i, name := range providerPriority {
		rank[name] = i
	}

	cases := []struct {
		a, b string // a must rank before b
	}{
		{"claude", "copilot"},
		{"copilot", "codex"},
		{"codex", "gemini"},
		{"gemini", "opencode"},
		{"opencode", "ollama"},
		{"ollama", "shell"},
	}
	for _, tc := range cases {
		ra, aOk := rank[tc.a]
		rb, bOk := rank[tc.b]
		if !aOk {
			t.Errorf("%q not found in providerPriority", tc.a)
			continue
		}
		if !bOk {
			t.Errorf("%q not found in providerPriority", tc.b)
			continue
		}
		if ra >= rb {
			t.Errorf("expected %q (rank %d) before %q (rank %d)", tc.a, ra, tc.b, rb)
		}
	}
}

// sortByPriority runs the same sort used in buildProviders on a []ProviderDef.
func sortByPriority(out []ProviderDef) {
	priorityRank := make(map[string]int, len(providerPriority))
	for i, name := range providerPriority {
		priorityRank[name] = i
	}
	sort.SliceStable(out, func(i, j int) bool {
		ri, iOk := priorityRank[out[i].ID]
		rj, jOk := priorityRank[out[j].ID]
		if iOk && jOk {
			return ri < rj
		}
		if iOk {
			return true
		}
		if jOk {
			return false
		}
		return false
	})
}

// TestOutSort_OllamaAndShellSortAfterPriorityProviders verifies that the static
// providers (ollama, shell) end up after claude/copilot/codex in the final list.
func TestOutSort_OllamaAndShellSortAfterPriorityProviders(t *testing.T) {
	// Simulate the order in which buildProviders assembles out:
	// static providers first (ollama, shell), then discovered extras.
	out := []ProviderDef{
		{ID: "ollama"},
		{ID: "shell"},
		{ID: "gemini"},
		{ID: "codex"},
		{ID: "claude"},
	}
	sortByPriority(out)

	wantOrder := []string{"claude", "codex", "gemini", "ollama", "shell"}
	for i, want := range wantOrder {
		if out[i].ID != want {
			t.Errorf("position %d: got %q, want %q", i, out[i].ID, want)
		}
	}
}

// TestOutSort_UnknownPreservesRelativeOrder verifies that unknown providers
// appear after all priority providers, in their original relative order.
func TestOutSort_UnknownPreservesRelativeOrder(t *testing.T) {
	out := []ProviderDef{
		{ID: "zebra-agent"},
		{ID: "alpha-agent"},
		{ID: "ollama"},
		{ID: "claude"},
	}
	sortByPriority(out)

	// claude → ollama → zebra-agent → alpha-agent (original order for unknowns)
	if out[0].ID != "claude" {
		t.Errorf("position 0: got %q, want claude", out[0].ID)
	}
	if out[1].ID != "ollama" {
		t.Errorf("position 1: got %q, want ollama", out[1].ID)
	}
	if out[2].ID != "zebra-agent" {
		t.Errorf("position 2: got %q, want zebra-agent", out[2].ID)
	}
	if out[3].ID != "alpha-agent" {
		t.Errorf("position 3: got %q, want alpha-agent", out[3].ID)
	}
}
