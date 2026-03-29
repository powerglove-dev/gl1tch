package switchboard

import (
	"encoding/json"
	"testing"
)

func TestLooksLikeMarkdown(t *testing.T) {
	cases := []struct {
		input string
		want  bool
	}{
		{"# Heading", true},
		{"**bold**", true},
		{"```go\ncode\n```", true},
		{"plain text output", false},
		{"", false},
		{"some output with # in the middle", true},
	}
	for _, tc := range cases {
		got := looksLikeMarkdown(tc.input)
		if got != tc.want {
			t.Errorf("looksLikeMarkdown(%q) = %v, want %v", tc.input, got, tc.want)
		}
	}
}

func TestRunMetadataJSON(t *testing.T) {
	cases := []struct {
		pipelineFile string
		cwd          string
		wantEmpty    bool
		wantKeys     map[string]string
	}{
		{"", "", true, nil},
		{"/a/b.yaml", "", false, map[string]string{"pipeline_file": "/a/b.yaml"}},
		{"", "/home/user", false, map[string]string{"cwd": "/home/user"}},
		{"/a/b.yaml", "/home/user", false, map[string]string{"pipeline_file": "/a/b.yaml", "cwd": "/home/user"}},
		{`path with "quotes"`, "", false, map[string]string{"pipeline_file": `path with "quotes"`}},
	}
	for _, tc := range cases {
		got := runMetadataJSON(tc.pipelineFile, tc.cwd, "")
		if tc.wantEmpty {
			if got != "" {
				t.Errorf("runMetadataJSON(%q, %q) = %q, want empty", tc.pipelineFile, tc.cwd, got)
			}
			continue
		}
		if got == "" {
			t.Errorf("runMetadataJSON(%q, %q) = empty, want non-empty", tc.pipelineFile, tc.cwd)
			continue
		}
		var parsed map[string]string
		if err := json.Unmarshal([]byte(got), &parsed); err != nil {
			t.Errorf("runMetadataJSON(%q, %q) = %q, invalid JSON: %v", tc.pipelineFile, tc.cwd, got, err)
			continue
		}
		for k, v := range tc.wantKeys {
			if parsed[k] != v {
				t.Errorf("key %q: got %q, want %q", k, parsed[k], v)
			}
		}
	}
}
