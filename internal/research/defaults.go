package research

import (
	"log/slog"
	"os"
	"path/filepath"

	"github.com/8op-org/gl1tch/internal/executor"
)

// DefaultPipelineResearchers is the canonical menu the research loop offers
// when no caller has assembled its own. Each entry is the (Name, Describe,
// Workflow) tuple a PipelineResearcher needs; Workflow is the bare workflow
// name (no path, no extension) so loadPipelineFile resolves it against
// .glitch/workflows on every workspace.
//
// The Describe text is what the planner LLM sees when picking — it must
// answer "what kind of question would I pick this for?" without leaking
// implementation details. Keep them short, action-framed, and free of any
// command names. The qwen2.5:7b planner is small; the more concrete the
// description, the more reliable the pick.
//
// Adding to this list is the cheapest way to make the loop smarter: a new
// workflow file in .glitch/workflows that emits an Evidence JSON, plus one
// entry here, and the planner immediately considers it for every research
// call.
var DefaultPipelineResearchers = []DefaultPipelineSpec{
	{
		Name: "github-prs",
		Describe: "Lists the currently open pull requests in the current git " +
			"repository, with PR numbers, titles, authors, states, draft " +
			"status, and last-updated timestamps.",
		Workflow: "github-prs",
	},
	{
		Name: "github-issues",
		Describe: "Lists the currently open issues in the current git " +
			"repository, with issue numbers, titles, authors, labels, and " +
			"last-updated timestamps.",
		Workflow: "github-issues",
	},
	{
		Name: "git-log",
		Describe: "Lists the most recent commits in the current git " +
			"repository, with short SHA, author, ISO date, and subject " +
			"line. Use for any question about what changed recently or " +
			"who touched what.",
		Workflow: "git-log",
	},
	{
		Name: "git-status",
		Describe: "Reports the current branch and working-tree state of " +
			"the current git repository, including any uncommitted or " +
			"untracked paths. Use for any question about what is locally " +
			"modified or which branch is checked out.",
		Workflow: "git-status",
	},
}

// DefaultPipelineSpec is the registration tuple for one entry in
// DefaultPipelineResearchers. It is exported so callers that build their own
// menu (e.g. tests, future plugin loaders) can compose it from the same type
// the defaults use.
type DefaultPipelineSpec struct {
	Name     string
	Describe string
	Workflow string
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

	var errs []error
	for _, spec := range DefaultPipelineResearchers {
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
