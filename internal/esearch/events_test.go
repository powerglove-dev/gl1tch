package esearch

import "testing"

// TestIsLocalProvider locks down the policy that drives the
// `escalated` flag in BrainDecision. If this test changes, every
// historical Kibana chart filtered on `escalated:true` shifts meaning
// — so update with intent.
func TestIsLocalProvider(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want bool
	}{
		{"ollama", "ollama", true},
		{"ollama uppercase", "Ollama", true},
		{"ollama whitespace", "  ollama  ", true},
		{"claude", "claude", false},
		{"openai", "openai", false},
		{"voyage", "voyage", false},
		{"empty is not local", "", false},
		{"unknown", "some-future-provider", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := IsLocalProvider(tc.in); got != tc.want {
				t.Errorf("IsLocalProvider(%q) = %v, want %v", tc.in, got, tc.want)
			}
		})
	}
}
