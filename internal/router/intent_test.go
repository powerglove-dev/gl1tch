//go:build !integration

package router

import "testing"

// TestIsImperativeInput is a contract test for isImperativeInput.
// The fast path ONLY fires for explicit pipeline-invocation verbs
// (run/execute/launch/rerun/start/trigger). Generic task requests — even
// strongly imperative ones ("review my PR") — must return false so the LLM
// classifier handles them and can return NONE instead of auto-routing.
func TestIsImperativeInput(t *testing.T) {
	cases := []struct {
		input string
		want  bool
	}{
		// Explicit pipeline invocations — fast path eligible
		{"run git-pulse", true},
		{"run the pr-review pipeline", true},
		{"run pr-review on https://github.com/org/repo/pull/42", true},
		{"execute support-digest", true},
		{"launch the docs-improve pipeline", true},
		{"rerun the digest", true},
		{"re-run support-digest", true},
		{"start git-pulse", true},
		{"trigger the pr-review pipeline", true},
		{"kick off support-digest", true},
		{"kick-off the pipeline", true},

		// Generic task requests — must return false (AI handles, not pipeline)
		{"review my PR", false},
		{"please review this PR https://github.com/org/repo/pull/1", false},
		{"can you review this pr?", false},
		{"improve the docs", false},
		{"check the status", false},
		{"fix the failing test", false},
		{"analyze the logs", false},
		{"generate a report", false},
		{"scan for vulnerabilities", false},
		{"show open issues", false},
		{"list recent commits", false},
		{"build the project", false},
		{"create a pipeline", false},
		{"deploy staging", false},
		{"update dependencies", false},
		{"push my changes", false},

		// Questions — must return false
		{"looks like there are merge conflicts?", false},
		{"why is the build failing?", false},
		{"is the pipeline running?", false},
		{"can you see open PRs?", false},
		{"any open PRs?", false},
		{"what is the status?", false},
		{"how does this work?", false},
		{"are there any failures?", false},
		{"was the deploy successful?", false},

		// Observations — must return false
		{"it looks like the tests are failing", false},
		{"looks like there is a problem", false},
		{"seems like it crashed", false},
		{"i think something is wrong", false},
		{"i noticed the logs are empty", false},
		{"any idea why this broke", false},
	}

	for _, tc := range cases {
		t.Run(tc.input, func(t *testing.T) {
			got := isImperativeInput(tc.input)
			if got != tc.want {
				t.Errorf("isImperativeInput(%q) = %v, want %v", tc.input, got, tc.want)
			}
		})
	}
}
