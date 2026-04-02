//go:build !integration

package router

import "testing"

// TestIsImperativeInput is a contract test for isImperativeInput.
// These cases define the boundary between the embedding fast path and the LLM stage.
// The fast path ONLY fires when isImperativeInput is true AND cosine ≥ ConfidentThreshold.
func TestIsImperativeInput(t *testing.T) {
	cases := []struct {
		input string
		want  bool
	}{
		// Commands — should return true
		{"run git-pulse", true},
		{"review my PR", true},
		{"check the status", true},
		{"deploy staging", true},
		{"generate a report", true},
		{"scan for vulnerabilities", true},
		{"show open issues", true},
		{"list recent commits", true},
		{"build the project", true},
		{"create a pipeline", true},
		{"fix the failing test", true},
		{"analyze the logs", true},
		{"start the job", true},
		{"launch the pipeline", true},
		{"rerun the digest", true},
		{"update dependencies", true},
		{"push my changes", true},

		// Questions ending with "?" — must return false
		{"looks like there are merge conflicts?", false},
		{"why is the build failing?", false},
		{"is the pipeline running?", false},
		{"can you see open PRs?", false},
		{"any open PRs?", false},
		{"what is the status?", false},
		{"how does this work?", false},
		{"are there any failures?", false},
		{"was the deploy successful?", false},
		{"were there any errors?", false},
		{"could this be a bug?", false},
		{"should I run this again?", false},
		{"would this trigger a pipeline?", false},
		{"will this run automatically?", false},
		{"did it finish?", false},
		{"does it still work?", false},
		{"do I need to push?", false},

		// Observations and hedged statements — must return false
		{"what seems to be wrong", false},
		{"it looks like the tests are failing", false},
		{"looks like there is a problem", false},
		{"seems like it crashed", false},
		{"i think something is wrong", false},
		{"i noticed the logs are empty", false},
		{"any idea why this broke", false},
		{"any thoughts on this", false},
		{"what's happening with the build", false},
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
