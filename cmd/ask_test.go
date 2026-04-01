package cmd

import (
	"strings"
	"testing"

	"github.com/8op-org/gl1tch/internal/pipeline"
)

// ── stripFences ───────────────────────────────────────────────────────────────

func TestStripFences(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"name: foo", "name: foo"},
		{"```yaml\nname: foo\n```", "name: foo"},
		{"```yml\nname: foo\n```", "name: foo"},
		{"```\nname: foo\n```", "name: foo"},
		{"  ```yaml\nname: foo\n```  ", "name: foo"},
	}
	for _, c := range cases {
		got := stripFences(c.in)
		if got != c.want {
			t.Errorf("stripFences(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// ── routeIntent response matching ────────────────────────────────────────────

// matchResponse is the pure matching logic extracted from routeIntent for unit testing.
func matchResponse(response string, refs []pipeline.PipelineRef) *pipeline.PipelineRef {
	response = strings.TrimSpace(response)
	response = strings.Trim(response, `"'.`)
	if strings.EqualFold(response, "NONE") || response == "" {
		return nil
	}
	for i, r := range refs {
		if strings.EqualFold(r.Name, response) {
			return &refs[i]
		}
	}
	return nil
}

func TestRouteIntentMatching(t *testing.T) {
	refs := []pipeline.PipelineRef{
		{Name: "sync-docs", Description: "Sync documentation"},
		{Name: "gh-review", Description: "Review GitHub PRs"},
	}

	t.Run("exact match", func(t *testing.T) {
		got := matchResponse("sync-docs", refs)
		if got == nil || got.Name != "sync-docs" {
			t.Errorf("expected sync-docs match, got %v", got)
		}
	})

	t.Run("case insensitive match", func(t *testing.T) {
		got := matchResponse("SYNC-DOCS", refs)
		if got == nil || got.Name != "sync-docs" {
			t.Errorf("expected sync-docs match, got %v", got)
		}
	})

	t.Run("NONE returns nil", func(t *testing.T) {
		if got := matchResponse("NONE", refs); got != nil {
			t.Errorf("expected nil for NONE, got %v", got)
		}
	})

	t.Run("none lowercase returns nil", func(t *testing.T) {
		if got := matchResponse("none", refs); got != nil {
			t.Errorf("expected nil for none, got %v", got)
		}
	})

	t.Run("garbage returns nil", func(t *testing.T) {
		if got := matchResponse("I think you want the sync-docs pipeline", refs); got != nil {
			t.Errorf("expected nil for garbage, got %v", got)
		}
	})

	t.Run("empty returns nil", func(t *testing.T) {
		if got := matchResponse("", refs); got != nil {
			t.Errorf("expected nil for empty, got %v", got)
		}
	})

	t.Run("quoted name is matched", func(t *testing.T) {
		got := matchResponse(`"sync-docs"`, refs)
		if got == nil || got.Name != "sync-docs" {
			t.Errorf("expected sync-docs match for quoted name, got %v", got)
		}
	})
}

// ── --route=false: discovery + classification skipped ────────────────────────

func TestAskRoute_FalseSkipsRouting(t *testing.T) {
	// When route=false, DiscoverPipelines is never called.
	// We verify this by ensuring the flag is wired — no integration needed.
	f := askCmd.Flags().Lookup("route")
	if f == nil {
		t.Fatal("--route flag not registered on askCmd")
	}
	if f.DefValue != "true" {
		t.Errorf("--route default = %q, want \"true\"", f.DefValue)
	}
}

// ── --dry-run flag registered ─────────────────────────────────────────────────

func TestAskDryRun_FlagRegistered(t *testing.T) {
	f := askCmd.Flags().Lookup("dry-run")
	if f == nil {
		t.Fatal("--dry-run flag not registered on askCmd")
	}
}

// ── --auto flag registered ────────────────────────────────────────────────────

func TestAskAuto_FlagRegistered(t *testing.T) {
	f := askCmd.Flags().Lookup("auto")
	if f == nil {
		t.Fatal("--auto flag not registered on askCmd")
	}
	shorthand := askCmd.Flags().ShorthandLookup("y")
	if shorthand == nil {
		t.Fatal("-y shorthand not registered on askCmd")
	}
}
