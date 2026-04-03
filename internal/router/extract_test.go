//go:build !integration

package router

import "testing"

// Tests for extractInput and extractCronPhrase — the fast-path extraction helpers
// that populate RouteResult.Input and RouteResult.CronExpr when the embedding
// stage bypasses the LLM.

func TestExtractInput_URL(t *testing.T) {
	cases := []struct {
		prompt string
		want   string
	}{
		{
			"run pr-review on https://github.com/org/repo/pull/42",
			"https://github.com/org/repo/pull/42",
		},
		{
			"execute support-digest for http://internal.example.com/queue",
			"http://internal.example.com/queue",
		},
		{
			// URL at end with trailing period — must strip it
			"run pr-review on https://github.com/org/repo/pull/1.",
			"https://github.com/org/repo/pull/1",
		},
		{
			// No URL — should not match
			"run git-pulse",
			"",
		},
	}
	for _, tc := range cases {
		t.Run(tc.prompt, func(t *testing.T) {
			got := extractInput(tc.prompt)
			if got != tc.want {
				t.Errorf("extractInput(%q) = %q, want %q", tc.prompt, got, tc.want)
			}
		})
	}
}

func TestExtractInput_OnFor(t *testing.T) {
	cases := []struct {
		prompt string
		want   string
	}{
		{
			"run docs-improve on executor package",
			"executor package",
		},
		{
			"launch support-digest for acme corp",
			"acme corp",
		},
		{
			// "on" with trailing "every" — topic ends before schedule
			"run docs-improve on executor package every 2 hours",
			"executor package",
		},
		{
			// No "on"/"for" — no match
			"run git-pulse",
			"",
		},
	}
	for _, tc := range cases {
		t.Run(tc.prompt, func(t *testing.T) {
			got := extractInput(tc.prompt)
			if got != tc.want {
				t.Errorf("extractInput(%q) = %q, want %q", tc.prompt, got, tc.want)
			}
		})
	}
}

func TestExtractCronPhrase(t *testing.T) {
	cases := []struct {
		label  string
		prompt string
		want   string
	}{
		{"every N hours", "run docs-improve every 2 hours", "0 */2 * * *"},
		{"every 1 hour (singular)", "run git-pulse every 1 hour", "0 */1 * * *"},
		{"every N minutes", "run git-pulse every 30 minutes", "*/30 * * * *"},
		{"every hour", "run docs-improve every hour", "0 * * * *"},
		{"daily", "run support-digest daily", "0 0 * * *"},
		{"every day", "run git-pulse every day", "0 0 * * *"},
		{"every morning", "run git-pulse every morning", "0 9 * * *"},
		{"every weekday", "run docs-improve every weekday", "0 9 * * 1-5"},
		{"every weekdays", "run docs-improve every weekdays", "0 9 * * 1-5"},
		{"every monday", "run support-digest every monday", "0 9 * * 1"},
		{"every friday", "run git-pulse every friday", "0 9 * * 5"},
		{"every sunday", "run cleanup every sunday", "0 9 * * 0"},
		{"no schedule", "run git-pulse", ""},
		{"unrecognized", "run git-pulse every other day", ""},
	}
	for _, tc := range cases {
		t.Run(tc.label, func(t *testing.T) {
			got := extractCronPhrase(tc.prompt)
			if got != tc.want {
				t.Errorf("extractCronPhrase(%q) = %q, want %q", tc.prompt, got, tc.want)
			}
		})
	}
}

func TestExtractInput_URLTakesPriorityOverOnFor(t *testing.T) {
	// When both URL and "on <topic>" are present, URL wins.
	prompt := "run pr-review on https://github.com/org/repo/pull/42 every day"
	got := extractInput(prompt)
	want := "https://github.com/org/repo/pull/42"
	if got != want {
		t.Errorf("extractInput(%q) = %q, want %q (URL should take priority)", prompt, got, want)
	}
}
