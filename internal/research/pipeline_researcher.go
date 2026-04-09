package research

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/8op-org/gl1tch/internal/executor"
	"github.com/8op-org/gl1tch/internal/pipeline"
)

// PipelineResearcher wraps a `.glitch/workflows/*.pipeline.yaml` whose final
// step prints an Evidence JSON. It is the user-extensible path: anyone who
// can write a pipeline can add a researcher with no Go code.
//
// The researcher loads the pipeline from disk, runs it via the existing
// pipeline.Run path with the query passed as user input, and parses the
// final step output through ParseEvidence. Validation failures (malformed
// JSON, missing required fields, schema version mismatch) are returned as
// errors so the loop's gather stage can drop the evidence and continue with
// the rest of the bundle, exactly per the partial-bundle requirement.
type PipelineResearcher struct {
	displayName  string
	displayDescr string
	pipelinePath string
	mgr          *executor.Manager
	// runFn is the indirection that lets tests substitute a hermetic
	// runner for pipeline.Run. Production callers leave it nil and the
	// researcher uses pipeline.Run directly.
	runFn pipelineRunFn
}

// pipelineRunFn matches the subset of pipeline.Run the researcher needs.
// Defining it here lets us swap the implementation in tests without pulling
// in any of the executor or store machinery.
type pipelineRunFn func(ctx context.Context, p *pipeline.Pipeline, mgr *executor.Manager, userInput string) (string, error)

// NewPipelineResearcher constructs a researcher from a pipeline file. The
// name and describe arguments are used by the planner; they are not derived
// from the pipeline name so that several researchers can wrap variants of
// the same pipeline with different framings (e.g. "github-prs" vs
// "github-issues").
//
// The mgr argument is the executor manager pipeline.Run uses to look up
// step executors. Tests can pass nil and rely on the underlying pipeline
// having no executor steps; production callers always pass the live mgr.
func NewPipelineResearcher(name, describe, pipelinePath string, mgr *executor.Manager) *PipelineResearcher {
	return &PipelineResearcher{
		displayName:  name,
		displayDescr: describe,
		pipelinePath: pipelinePath,
		mgr:          mgr,
	}
}

// Name implements Researcher.
func (r *PipelineResearcher) Name() string { return r.displayName }

// Describe implements Researcher.
func (r *PipelineResearcher) Describe() string { return r.displayDescr }

// Gather implements Researcher.
func (r *PipelineResearcher) Gather(ctx context.Context, q ResearchQuery, _ EvidenceBundle) (Evidence, error) {
	if r.pipelinePath == "" {
		return Evidence{}, errors.New("research: PipelineResearcher: empty pipeline path")
	}

	p, err := loadPipelineFile(r.pipelinePath)
	if err != nil {
		return Evidence{}, fmt.Errorf("load pipeline %q: %w", r.pipelinePath, err)
	}

	// Inject every var on q.Context as a step-level Var on every step
	// of the loaded pipeline. The pipeline runner promotes step.Vars to
	// the executor, which (a) sets `cmd.Dir = vars["cwd"]` for tool-kind
	// plugins like the shell wrapper and (b) overlays the rest as
	// GLITCH_<KEY> env vars on the subprocess. This is the lever that
	// makes a thread anchored on the ensemble workspace actually run
	// `git -C <ensemble>` instead of `git -C <gl1tch>`.
	//
	// We mutate the loaded pipeline (a fresh copy each call from
	// loadPipelineFile) so the injected vars don't persist across
	// calls. Existing per-step Vars take precedence — a workflow that
	// declares its own cwd wins over the loop's default.
	if len(q.Context) > 0 {
		for i := range p.Steps {
			if p.Steps[i].Vars == nil {
				p.Steps[i].Vars = make(map[string]string, len(q.Context))
			}
			for k, v := range q.Context {
				if _, exists := p.Steps[i].Vars[k]; exists {
					continue
				}
				p.Steps[i].Vars[k] = v
			}
		}
	}

	run := r.runFn
	if run == nil {
		run = defaultPipelineRun
	}
	out, err := run(ctx, p, r.mgr, q.Question)
	if err != nil {
		return Evidence{}, fmt.Errorf("run pipeline %q: %w", r.displayName, err)
	}

	ev, err := ParseEvidence([]byte(out))
	if err != nil {
		// The wire format requires the body in the parsed JSON. If
		// parsing failed but we got *something* on stdout, surface a
		// short prefix in the error so the loop log shows what the
		// pipeline actually produced — debugging a malformed
		// researcher is otherwise miserable.
		preview := strings.TrimSpace(out)
		if len(preview) > 200 {
			preview = preview[:200] + "…"
		}
		return Evidence{}, fmt.Errorf("%w: pipeline=%q preview=%q", err, r.displayName, preview)
	}

	// Force the source to match the researcher name regardless of what the
	// pipeline self-reported. The planner picks researchers by name and we
	// want every Evidence to be unambiguously attributable to the picked
	// researcher.
	ev.Source = r.displayName
	return ev, nil
}

// defaultPipelineRun is the production implementation of pipelineRunFn that
// delegates to the real pipeline.Run with the silent-status and
// no-clarification options the loop needs.
func defaultPipelineRun(ctx context.Context, p *pipeline.Pipeline, mgr *executor.Manager, userInput string) (string, error) {
	return pipeline.Run(ctx, p, mgr, userInput, pipeline.WithSilentStatus(), pipeline.WithNoClarification())
}

// loadPipelineFile resolves a workflow file path and returns the parsed
// Pipeline. Resolution tries, in order: the path as given, the path joined
// to .glitch/workflows, and the .workflow.yaml suffix added to either of
// the above. This lets a researcher be registered with the bare workflow
// name (e.g. "github-prs") when it lives next to the other workflows.
func loadPipelineFile(path string) (*pipeline.Pipeline, error) {
	var candidates []string
	add := func(p string) {
		candidates = append(candidates, p)
		if !strings.HasSuffix(p, ".workflow.yaml") && !strings.HasSuffix(p, ".yaml") {
			candidates = append(candidates, p+".workflow.yaml")
		}
	}
	add(path)
	if !filepath.IsAbs(path) {
		add(filepath.Join(".glitch", "workflows", path))
	}
	var lastErr error
	for _, c := range candidates {
		f, err := os.Open(c)
		if err != nil {
			lastErr = err
			continue
		}
		p, err := pipeline.Load(f)
		_ = f.Close()
		if err != nil {
			return nil, err
		}
		return p, nil
	}
	if lastErr != nil {
		return nil, lastErr
	}
	return nil, fmt.Errorf("pipeline file not found: %s", path)
}
