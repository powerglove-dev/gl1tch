package glitchd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// withIsolatedPromptEnv redirects HOME and GLITCH_PROMPTS_DIR at a
// clean temp tree so each test case sees its own prompt search path
// without leaking into the user's real config.
//
// The returned cleanup restores the original env vars AND resets the
// prompt cache so a subsequent test that loads a different override
// actually re-reads from disk instead of hitting the memoized copy.
func withIsolatedPromptEnv(t *testing.T) (homeDir, promptsDir string) {
	t.Helper()
	homeDir = t.TempDir()
	promptsDir = t.TempDir()

	// Write a bundled-default research prompt into the override
	// dir so LoadPrompt("research_default") succeeds. This mirrors
	// the file that ships in pkg/glitchd/prompts/.
	if err := os.WriteFile(
		filepath.Join(promptsDir, "research_default.md"),
		[]byte("# bundled default\n\nescalate: on-request\n"),
		0o644,
	); err != nil {
		t.Fatalf("seed bundled default: %v", err)
	}

	t.Setenv("HOME", homeDir)
	t.Setenv("GLITCH_PROMPTS_DIR", promptsDir)
	ResetPromptCache()
	t.Cleanup(ResetPromptCache)
	return homeDir, promptsDir
}

func TestResearchPromptPath_Workspace(t *testing.T) {
	home, _ := withIsolatedPromptEnv(t)
	got, err := ResearchPromptPath("ws-123")
	if err != nil {
		t.Fatalf("ResearchPromptPath: %v", err)
	}
	want := filepath.Join(home, ".config", "glitch", "workspaces", "ws-123", "research.md")
	if got != want {
		t.Errorf("workspace path:\n  got  %q\n  want %q", got, want)
	}
}

func TestResearchPromptPath_Global(t *testing.T) {
	home, _ := withIsolatedPromptEnv(t)
	got, err := ResearchPromptPath("")
	if err != nil {
		t.Fatalf("ResearchPromptPath: %v", err)
	}
	want := filepath.Join(home, ".config", "glitch", "research.md")
	if got != want {
		t.Errorf("global path:\n  got  %q\n  want %q", got, want)
	}
}

func TestLoadResearchPrompt_BundledDefault(t *testing.T) {
	_, _ = withIsolatedPromptEnv(t)
	got, err := LoadResearchPrompt("")
	if err != nil {
		t.Fatalf("LoadResearchPrompt: %v", err)
	}
	if !strings.Contains(got, "bundled default") {
		t.Errorf("expected bundled default content, got:\n%s", got)
	}
}

func TestLoadResearchPrompt_GlobalOverridesBundled(t *testing.T) {
	home, _ := withIsolatedPromptEnv(t)
	globalPath := filepath.Join(home, ".config", "glitch", "research.md")
	if err := os.MkdirAll(filepath.Dir(globalPath), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(globalPath, []byte("# global override\n"), 0o644); err != nil {
		t.Fatalf("write global: %v", err)
	}

	got, err := LoadResearchPrompt("")
	if err != nil {
		t.Fatalf("LoadResearchPrompt: %v", err)
	}
	if !strings.Contains(got, "global override") {
		t.Errorf("expected global override, got:\n%s", got)
	}
}

func TestLoadResearchPrompt_WorkspaceOverridesGlobal(t *testing.T) {
	home, _ := withIsolatedPromptEnv(t)
	// Write a global override that MUST NOT win.
	globalPath := filepath.Join(home, ".config", "glitch", "research.md")
	_ = os.MkdirAll(filepath.Dir(globalPath), 0o755)
	_ = os.WriteFile(globalPath, []byte("# global override\n"), 0o644)

	// Write a workspace override that MUST win.
	wsPath := filepath.Join(home, ".config", "glitch", "workspaces", "ws-abc", "research.md")
	_ = os.MkdirAll(filepath.Dir(wsPath), 0o755)
	_ = os.WriteFile(wsPath, []byte("# workspace override\n"), 0o644)

	got, err := LoadResearchPrompt("ws-abc")
	if err != nil {
		t.Fatalf("LoadResearchPrompt: %v", err)
	}
	if !strings.Contains(got, "workspace override") {
		t.Errorf("expected workspace override, got:\n%s", got)
	}
	if strings.Contains(got, "global override") {
		t.Errorf("workspace override should have shadowed the global one")
	}
}

func TestEnsureResearchPrompt_SeedsFromBundledDefault(t *testing.T) {
	home, _ := withIsolatedPromptEnv(t)

	if err := EnsureResearchPrompt("ws-new"); err != nil {
		t.Fatalf("EnsureResearchPrompt: %v", err)
	}

	wsPath := filepath.Join(home, ".config", "glitch", "workspaces", "ws-new", "research.md")
	b, err := os.ReadFile(wsPath)
	if err != nil {
		t.Fatalf("read seeded file: %v", err)
	}
	if !strings.Contains(string(b), "bundled default") {
		t.Errorf("seeded file should contain bundled default content, got:\n%s", b)
	}
}

func TestEnsureResearchPrompt_Idempotent(t *testing.T) {
	home, _ := withIsolatedPromptEnv(t)
	wsPath := filepath.Join(home, ".config", "glitch", "workspaces", "ws-existing", "research.md")
	_ = os.MkdirAll(filepath.Dir(wsPath), 0o755)
	existing := []byte("# user's own prompt — keep me\n")
	if err := os.WriteFile(wsPath, existing, 0o644); err != nil {
		t.Fatalf("pre-write: %v", err)
	}

	if err := EnsureResearchPrompt("ws-existing"); err != nil {
		t.Fatalf("EnsureResearchPrompt: %v", err)
	}

	got, err := os.ReadFile(wsPath)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(got) != string(existing) {
		t.Errorf("Ensure must not overwrite an existing file.\n got: %q\n want: %q",
			got, existing)
	}
}

func TestEnsureResearchPrompt_EmptyWorkspaceIDRejected(t *testing.T) {
	_, _ = withIsolatedPromptEnv(t)
	if err := EnsureResearchPrompt(""); err == nil {
		t.Errorf("empty workspace id should be rejected")
	}
}

func TestResearchEscalationMode(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  string
	}{
		{"default when absent", "# no escalate line here\n", "on-request"},
		{"explicit on-request", "text\nescalate: on-request\nmore\n", "on-request"},
		{"auto high", "escalate: auto-high\n", "auto-high"},
		{"auto alias", "escalate: auto\n", "auto-high"},
		{"manual alias", "escalate: manual\n", "on-request"},
		{"off alias", "escalate: off\n", "on-request"},
		{"unknown value degrades", "escalate: nuclear\n", "on-request"},
		{"case insensitive key", "ESCALATE: auto-high\n", "auto-high"},
		{"whitespace tolerant", "  escalate:   auto-high   \n", "auto-high"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ResearchEscalationMode(tc.input); got != tc.want {
				t.Errorf("ResearchEscalationMode(%q) = %q, want %q",
					tc.input, got, tc.want)
			}
		})
	}
}
