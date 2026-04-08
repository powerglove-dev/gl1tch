package research

import (
	"context"

	"github.com/8op-org/gl1tch/internal/executor"
	"github.com/8op-org/gl1tch/internal/pipeline"
)

// SetPipelineRunner replaces the pipeline runner used by a PipelineResearcher
// with a hermetic stub. Test-only — production callers must not depend on
// this seam.
func (r *PipelineResearcher) SetPipelineRunner(fn func(ctx context.Context, p *pipeline.Pipeline, mgr *executor.Manager, userInput string) (string, error)) {
	r.runFn = fn
}
