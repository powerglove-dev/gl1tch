package collector

import "testing"

// TestEncodeClaudeDirName covers the lossless path → Claude-projects
// directory-name mapping. Hyphens in path components are the gotcha:
// they survive the encode (because we only replace slashes), but the
// reverse direction is ambiguous, which is why ClaudeProjectCollector's
// workspace filter compares in the encode direction.
func TestEncodeClaudeDirName(t *testing.T) {
	cases := []struct {
		path string
		want string
	}{
		{"/Users/stokes/Projects/ensemble", "-Users-stokes-Projects-ensemble"},
		{"/Users/stokes/Projects/oblt-cli", "-Users-stokes-Projects-oblt-cli"},
		{"/Users/stokes/Projects/observability-robots", "-Users-stokes-Projects-observability-robots"},
		{"/", "-"},
	}
	for _, tc := range cases {
		t.Run(tc.path, func(t *testing.T) {
			if got := encodeClaudeDirName(tc.path); got != tc.want {
				t.Errorf("encodeClaudeDirName(%q) = %q, want %q",
					tc.path, got, tc.want)
			}
		})
	}
}

// TestPathInDirs locks down the workspace-scoping predicate that
// drives Claude collector filtering. The function is small but the
// edge cases (empty dirs, prefix-without-separator, exact match,
// trailing slashes) all matter — getting any of them wrong silently
// re-enables the cross-workspace duplication bug it was written to
// fix, and the brain popover would go back to showing identical
// counts in every workspace.
func TestPathInDirs(t *testing.T) {
	cases := []struct {
		name string
		path string
		dirs []string
		want bool
	}{
		{
			name: "empty dirs means include all",
			path: "/anything",
			dirs: nil,
			want: true,
		},
		{
			name: "exact match",
			path: "/Users/stokes/Projects/foo",
			dirs: []string{"/Users/stokes/Projects/foo"},
			want: true,
		},
		{
			name: "child path",
			path: "/Users/stokes/Projects/foo/bar/baz",
			dirs: []string{"/Users/stokes/Projects/foo"},
			want: true,
		},
		{
			name: "sibling path is not a match",
			path: "/Users/stokes/Projects/foobar",
			dirs: []string{"/Users/stokes/Projects/foo"},
			want: false,
		},
		{
			name: "unrelated path",
			path: "/tmp/something",
			dirs: []string{"/Users/stokes/Projects/foo"},
			want: false,
		},
		{
			name: "matches second of multiple dirs",
			path: "/Users/stokes/Projects/bar/cmd/main.go",
			dirs: []string{"/foo", "/Users/stokes/Projects/bar"},
			want: true,
		},
		{
			name: "trailing slash on dir is normalized",
			path: "/Users/stokes/Projects/foo/x",
			dirs: []string{"/Users/stokes/Projects/foo/"},
			want: true,
		},
		{
			name: "empty path is never a match",
			path: "",
			dirs: []string{"/anywhere"},
			want: false,
		},
		{
			name: "empty string in dirs is ignored",
			path: "/Users/stokes/Projects/foo",
			dirs: []string{"", "/Users/stokes/Projects/foo"},
			want: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := pathInDirs(tc.path, tc.dirs); got != tc.want {
				t.Errorf("pathInDirs(%q, %v) = %v, want %v",
					tc.path, tc.dirs, got, tc.want)
			}
		})
	}
}
