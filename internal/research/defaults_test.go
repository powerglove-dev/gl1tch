package research

import (
	"os"
	"path/filepath"
	"sort"
	"testing"
)

// TestDefaultRegistry_RegistersExistingWorkflows builds an isolated workflows
// directory containing a subset of the canonical specs and verifies the
// returned registry contains exactly those researchers — and skips the rest
// without erroring. This is the contract DefaultRegistry promises to
// workspaces that have not adopted every researcher: degrade silently, never
// fail startup over a missing workflow.
func TestDefaultRegistry_RegistersExistingWorkflows(t *testing.T) {
	dir := t.TempDir()

	// Create stub workflow files for two of the four canonical specs.
	// Their content does not matter for this test — DefaultRegistry only
	// stat()s them; they are not loaded until Gather() is called.
	for _, name := range []string{"github-prs", "git-log"} {
		path := filepath.Join(dir, name+".workflow.yaml")
		if err := os.WriteFile(path, []byte("name: "+name+"\nversion: \"1\"\nsteps: []\n"), 0o644); err != nil {
			t.Fatalf("write stub workflow %s: %v", name, err)
		}
	}

	reg, err := DefaultRegistry(nil, dir)
	if err != nil {
		t.Fatalf("DefaultRegistry: unexpected error: %v", err)
	}

	got := reg.Names()
	sort.Strings(got)
	want := []string{"git-log", "github-prs"}
	if len(got) != len(want) {
		t.Fatalf("registry names: got %v, want %v", got, want)
	}
	for i, name := range want {
		if got[i] != name {
			t.Errorf("registry name [%d]: got %q, want %q", i, got[i], name)
		}
	}

	// Sanity check: every registered researcher carries the spec's
	// canonical Describe text. The planner relies on this — if Describe
	// silently turns into the bare workflow name, the menu becomes
	// useless to qwen2.5:7b.
	for _, name := range want {
		r, ok := reg.Lookup(name)
		if !ok {
			t.Fatalf("Lookup(%q) returned not-found after Register", name)
		}
		if r.Describe() == "" || r.Describe() == name {
			t.Errorf("researcher %q has empty or trivial Describe: %q",
				name, r.Describe())
		}
	}
}

// TestDefaultRegistry_EmptyDirYieldsEmptyRegistry verifies that a workspace
// with no .glitch/workflows directory still produces a valid (empty) registry
// rather than an error. The loop's empty-plan short-circuit handles this
// case end-to-end; DefaultRegistry just has to not panic.
func TestDefaultRegistry_EmptyDirYieldsEmptyRegistry(t *testing.T) {
	reg, err := DefaultRegistry(nil, t.TempDir())
	if err != nil {
		t.Fatalf("DefaultRegistry on empty dir: unexpected error: %v", err)
	}
	if names := reg.Names(); len(names) != 0 {
		t.Errorf("empty workflows dir: expected no researchers, got %v", names)
	}
}

// TestDefaultPipelineSpecs_AreUniqueAndComplete is a static check on the
// canonical spec list itself. Names must be unique (the registry would reject
// duplicates at runtime, but a duplicate in this list is a coding bug worth
// catching at test time, not at the first `glitch ask`) and every entry must
// have a non-empty Describe so the planner has something to read.
func TestDefaultPipelineSpecs_AreUniqueAndComplete(t *testing.T) {
	seen := make(map[string]struct{}, len(DefaultPipelineResearchers))
	for _, spec := range DefaultPipelineResearchers {
		if spec.Name == "" {
			t.Errorf("spec has empty Name: %+v", spec)
		}
		if spec.Workflow == "" {
			t.Errorf("spec %q has empty Workflow", spec.Name)
		}
		if len(spec.Describe) < 20 {
			t.Errorf("spec %q has trivially short Describe (%d chars); the planner needs a real description",
				spec.Name, len(spec.Describe))
		}
		if _, dup := seen[spec.Name]; dup {
			t.Errorf("duplicate spec name in DefaultPipelineResearchers: %q", spec.Name)
		}
		seen[spec.Name] = struct{}{}
	}
}
