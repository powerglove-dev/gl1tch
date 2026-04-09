package research

import (
	_ "embed"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"

	"gopkg.in/yaml.v3"

	"github.com/8op-org/gl1tch/internal/executor"
)

//go:embed researchers.yaml
var embeddedResearchersYAML []byte

// DefaultPipelineResearchers used to be a hand-curated package-level
// slice. It is now a deprecated alias backed by LoadDefaultPipelineSpecs
// so any caller that imports the slice still gets the canonical list,
// but new code should call LoadDefaultPipelineSpecs() to get the live
// (re-read on every call) view that picks up disk overrides.
//
// Adding to the menu is now a YAML edit, not a Go change: drop a
// workflow in .glitch/workflows, add a row to
// internal/research/researchers.yaml (or a user override at
// ~/.config/glitch/researchers.yaml), done.
var DefaultPipelineResearchers = func() []DefaultPipelineSpec {
	specs, _ := LoadDefaultPipelineSpecs()
	return specs
}()

// researchersDoc is the on-disk YAML shape for the researcher menu.
// Kept private — callers consume the flat []DefaultPipelineSpec slice
// LoadDefaultPipelineSpecs returns.
type researchersDoc struct {
	Researchers []DefaultPipelineSpec `yaml:"researchers"`
}

// loadResearchersOnce caches the disk-vs-embedded resolution for the
// duration of the test suite, but production callers go through the
// non-cached LoadDefaultPipelineSpecs path so a tweak to the user
// override file takes effect on the next call without process
// restart. The cache exists so the deprecated package-level
// DefaultPipelineResearchers var doesn't re-parse on every import.
var loadResearchersOnce sync.Once

// LoadDefaultPipelineSpecs returns the canonical researcher menu the
// loop's DefaultRegistry registers. Resolution order, lowest wins:
//
//   1. ~/.config/glitch/researchers.yaml (user override)
//   2. internal/research/researchers.yaml (embedded default)
//
// Re-reads on every call (no caching) so the tuning loop is
// `vim ~/.config/glitch/researchers.yaml` → `glitch threads new` →
// see the planner pick differently. Errors collapse to "fall back
// to embedded" so a malformed user file never crashes the loop.
func LoadDefaultPipelineSpecs() ([]DefaultPipelineSpec, error) {
	if specs, src, err := loadResearchersFromDisk(); err == nil && len(specs) > 0 {
		_ = src // logged by callers that care
		return specs, nil
	}
	return parseResearchersYAML(embeddedResearchersYAML)
}

// loadResearchersFromDisk probes the user override path. Returns
// the parsed specs + the source path, or err when no override exists
// or the file is malformed.
func loadResearchersFromDisk() ([]DefaultPipelineSpec, string, error) {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return nil, "", os.ErrNotExist
	}
	path := filepath.Join(home, ".config", "glitch", "researchers.yaml")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, "", err
	}
	specs, err := parseResearchersYAML(data)
	if err != nil {
		return nil, path, err
	}
	return specs, path, nil
}

// parseResearchersYAML decodes the embedded or override YAML and
// trims whitespace off every Describe so the planner sees clean
// menu rows. The YAML pipe-block (`describe: |`) form preserves
// newlines for readability in the file but the planner gets a
// single trimmed paragraph.
func parseResearchersYAML(data []byte) ([]DefaultPipelineSpec, error) {
	var doc researchersDoc
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("research: parse researchers yaml: %w", err)
	}
	for i, spec := range doc.Researchers {
		// Collapse Describe whitespace so a YAML pipe-block
		// describes the planner like a one-paragraph summary.
		doc.Researchers[i].Describe = collapseWhitespace(spec.Describe)
	}
	return doc.Researchers, nil
}

// collapseWhitespace flattens runs of whitespace (including newlines)
// into single spaces and trims the ends. The YAML loader keeps the
// pipe-block formatting in the file for readability; the planner
// sees the flat form.
func collapseWhitespace(s string) string {
	var b []byte
	prevSpace := true
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == ' ' || c == '\t' || c == '\n' || c == '\r' {
			if !prevSpace {
				b = append(b, ' ')
				prevSpace = true
			}
			continue
		}
		b = append(b, c)
		prevSpace = false
	}
	out := string(b)
	if n := len(out); n > 0 && out[n-1] == ' ' {
		out = out[:n-1]
	}
	return out
}

// DiskOverridePath returns the absolute path the user override
// would live at, even if the file does not yet exist. Used by
// `glitch researchers edit` to figure out where to write the
// seed copy.
func DiskOverridePath() string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return ""
	}
	return filepath.Join(home, ".config", "glitch", "researchers.yaml")
}

// EmbeddedResearchersDefault returns the embedded default YAML
// regardless of any disk override. Used by `glitch researchers edit`
// (which seeds the user override from this) and `glitch researchers
// diff` (which compares against this).
func EmbeddedResearchersDefault() []byte {
	return append([]byte(nil), embeddedResearchersYAML...)
}

// DefaultPipelineSpec is the registration tuple for one entry in
// the canonical researcher menu. It is exported so callers that
// build their own menu (e.g. tests, future plugin loaders) can
// compose it from the same type the defaults use. The yaml tags
// match researchers.yaml's field names so a single struct round-
// trips through both Go callers and disk overrides.
type DefaultPipelineSpec struct {
	Name     string `yaml:"name"`
	Describe string `yaml:"describe"`
	Workflow string `yaml:"workflow"`
}

// DefaultRegistry builds a Registry pre-populated with the canonical
// pipeline-backed researcher set. Researchers whose backing workflow file is
// not present in the workspace are silently skipped — this lets the loop
// degrade gracefully on workspaces that have not adopted every researcher.
//
// The mgr argument is the executor manager pipeline.Run uses to look up step
// executors; pass the same value the rest of the command path uses
// (buildFullManager() in cmd/glitch). Production callers always supply a
// real manager — passing nil will work for tests against synthetic pipelines
// but will fail any real shell or ollama step.
//
// Researcher resolution searches both the current working directory and any
// ancestor directory containing .glitch/workflows, mirroring how
// findAncestorWorkflowsDir resolves the workflow root for `glitch ask`. The
// caller may also pass an explicit workflowsDir override (empty string falls
// back to the auto-discovery behaviour).
func DefaultRegistry(mgr *executor.Manager, workflowsDir string) (*Registry, error) {
	reg := NewRegistry()

	// Resolve where workflows live. An explicit override wins; otherwise we
	// walk up from cwd looking for .glitch/workflows. The walk is the same
	// behaviour cmd/glitch/ask.go uses for routing, so a workspace that
	// works for `glitch ask` will work here without further configuration.
	dir := workflowsDir
	if dir == "" {
		dir = findAncestorWorkflowsDirFromCwd()
	}

	// Re-read the canonical researcher menu on every DefaultRegistry
	// call so a disk override at ~/.config/glitch/researchers.yaml
	// takes effect on the next research run without a process
	// restart. The cost is one filesystem stat + a small YAML
	// parse — trivial against the latency of the LLM call that
	// follows. Errors collapse to the embedded default; the loop
	// must always have a usable menu.
	specs, err := LoadDefaultPipelineSpecs()
	if err != nil || len(specs) == 0 {
		slog.Debug("research: falling back to embedded researcher menu", "err", err)
		specs, _ = parseResearchersYAML(embeddedResearchersYAML)
	}

	var errs []error
	for _, spec := range specs {
		path := resolveWorkflowPath(dir, spec.Workflow)
		if path == "" {
			slog.Debug("research: default registry skipping researcher (workflow not found)",
				"researcher", spec.Name, "workflow", spec.Workflow)
			continue
		}
		r := NewPipelineResearcher(spec.Name, spec.Describe, path, mgr)
		if err := reg.Register(r); err != nil {
			errs = append(errs, err)
		}
	}
	return reg, joinErrs(errs)
}

// resolveWorkflowPath turns a bare workflow name into an absolute path under
// the given workflowsDir, returning "" when no candidate exists. We try the
// `.workflow.yaml` and `.yaml` extensions in that order to match the rest of
// the workflow loader's tolerance.
func resolveWorkflowPath(workflowsDir, name string) string {
	if workflowsDir == "" || name == "" {
		return ""
	}
	for _, ext := range []string{".workflow.yaml", ".yaml"} {
		candidate := filepath.Join(workflowsDir, name+ext)
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}
	return ""
}

// findAncestorWorkflowsDirFromCwd walks up from the process working directory
// looking for a .glitch/workflows directory. Returns "" when none is found
// before the filesystem root. This is the same lookup `glitch ask` does in
// findAncestorWorkflowsDir; duplicated here so the research package does not
// have to depend on the cmd package.
func findAncestorWorkflowsDirFromCwd() string {
	cwd, err := os.Getwd()
	if err != nil {
		return ""
	}
	for {
		candidate := filepath.Join(cwd, ".glitch", "workflows")
		if info, err := os.Stat(candidate); err == nil && info.IsDir() {
			return candidate
		}
		parent := filepath.Dir(cwd)
		if parent == cwd {
			return ""
		}
		cwd = parent
	}
}
