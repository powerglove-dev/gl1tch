package glitchctx

import (
	"os"
	"path/filepath"
	"testing"
)

// TestResolveSideEffectPath covers the path-coercion logic that fixes the
// "LLM emits /.glitch/foo and we try to write to root" failure mode.
func TestResolveSideEffectPath(t *testing.T) {
	tmp := t.TempDir()
	// A real subdirectory we can use for the "absolute path that exists"
	// pass-through case. resolveSideEffectPath only coerces an absolute
	// path when its parent directory does NOT already exist.
	realDir := filepath.Join(tmp, "real")
	if err := os.MkdirAll(realDir, 0o755); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name     string
		raw      string
		cwd      string
		want     string
		coerced  bool
	}{
		{
			name: "relative path joined onto cwd",
			raw:  ".glitch/workflows/foo.yaml",
			cwd:  "/workspace",
			want: "/workspace/.glitch/workflows/foo.yaml",
		},
		{
			name:    "LLM-style /.glitch absolute coerced under cwd",
			raw:     "/.glitch/workflows/security-scan.workflow.yaml",
			cwd:     tmp,
			want:    filepath.Join(tmp, ".glitch/workflows/security-scan.workflow.yaml"),
			coerced: true,
		},
		{
			name: "absolute path with existing parent passes through",
			raw:  filepath.Join(realDir, "out.txt"),
			cwd:  tmp,
			want: filepath.Join(realDir, "out.txt"),
		},
		{
			name: "relative path with empty cwd left alone",
			raw:  "foo/bar.yaml",
			cwd:  "",
			want: "foo/bar.yaml",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, coerced := resolveSideEffectPath(tt.raw, tt.cwd)
			if got != tt.want {
				t.Errorf("path: got %q, want %q", got, tt.want)
			}
			if coerced != tt.coerced {
				t.Errorf("coerced: got %v, want %v", coerced, tt.coerced)
			}
		})
	}
}

// TestWorkspaceCwdPrefersEnv proves that GLITCH_CWD wins over the
// process working directory. This is the contract the runner relies
// on — it sets GLITCH_CWD on the plugin subprocess via cli_adapter.go
// and expects the plugin to honor it for both write blocks and shell
// runs, regardless of where glitch-desktop itself was launched.
func TestWorkspaceCwdPrefersEnv(t *testing.T) {
	t.Setenv("GLITCH_CWD", "/some/workspace")
	if got := workspaceCwd(); got != "/some/workspace" {
		t.Errorf("workspaceCwd: got %q, want %q", got, "/some/workspace")
	}
}
