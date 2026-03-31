package pipeline

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/adam-stokes/orcai/internal/store"
)

// injectBrainContext applies brain pre-context modifications to a prompt string.
// It is called by both the legacy and DAG execution paths before dispatching to a plugin.
//   - Brain is always on: if a BrainInjector is configured, the read preamble
//     (which includes the brain_notes write instruction) is always prepended.
//   - If write_brain is active and no injector is configured, the write instruction is appended.
func injectBrainContext(ctx context.Context, prompt string, p *Pipeline, step *Step, ec *ExecutionContext) string {
	if inj := ec.GetBrainInjector(); inj != nil {
		if preamble, err := inj.ReadContext(ctx, ec.RunID()); err == nil && preamble != "" {
			prompt = preamble + "\n\n" + prompt
		} else if err != nil {
			fmt.Fprintf(os.Stderr, "[debug] brain read context error for step %q: %v\n", step.ID, err)
		}
		// BrainInjector preamble already includes the write instruction.
		return prompt
	}
	// No injector configured — fall back to write instruction only when write_brain is set.
	if stepWriteBrain(p, step) {
		prompt = prompt + brainWriteInstruction
	}
	return prompt
}

// brainWriteInstruction is appended to the prompt of write_brain-only steps
// (steps where write_brain is true but use_brain is false). For use_brain steps
// the instruction is embedded directly in the ReadContext preamble.
const brainWriteInstruction = `

---
BRAIN NOTE INSTRUCTION: Include a <brain_notes> block somewhere in your response to persist an insight for future steps in this pipeline.

  Human-readable summary goes directly as text. Use nested XML elements for structured data:

  <brain_notes>
  Plain text insight or summary here.
  <detail key="metric">structured value</detail>
  </brain_notes>

The brain note will be stored and made available to subsequent agent steps with use_brain enabled.
---`

// TODO: future flag strip_brain_output: true on Pipeline could strip the <brain_notes>
// block from the user-visible output string before it is published to the feed/inbox.
// Currently the <brain_notes> XML remains visible in step output.

// brainBlockRe matches <brain_notes> blocks in agent output.
// The content may contain nested XML elements for structured data.
var brainBlockRe = regexp.MustCompile(`(?s)<brain_notes\b[^>]*>(.*?)</brain_notes>`)

// parseBrainBlock scans output for a <brain_notes> XML block and persists it to the store.
// It is a best-effort operation: missing or malformed blocks are silently skipped with a
// debug log. The step never fails due to brain parsing errors.
func parseBrainBlock(ctx context.Context, output, stepID string, ec *ExecutionContext) {
	matches := brainBlockRe.FindStringSubmatch(output)
	if matches == nil {
		fmt.Fprintf(os.Stderr, "[debug] brain step %q: no <brain_notes> block found in output\n", stepID)
		return
	}
	body := strings.TrimSpace(matches[1])
	if body == "" {
		return
	}
	s := ec.DB()
	if s == nil {
		fmt.Fprintf(os.Stderr, "[debug] brain: no store configured, cannot persist brain note for step %q\n", stepID)
		return
	}
	note := store.BrainNote{
		RunID:     ec.RunID(),
		StepID:    stepID,
		CreatedAt: time.Now().UnixMilli(),
		Body:      body,
	}
	if _, err := s.InsertBrainNote(ctx, note); err != nil {
		fmt.Fprintf(os.Stderr, "[debug] brain: failed to insert brain note for step %q: %v\n", stepID, err)
	}
}

// BrainInjector assembles pre-context text for brain-aware pipeline steps.
// ReadContext returns a plain-text preamble to prepend to agent prompts.
// runID is the current pipeline run's store run ID (0 if store not configured).
type BrainInjector interface {
	ReadContext(ctx context.Context, runID int64) (string, error)
}

// StoreBrainInjector is the default BrainInjector backed by the ORCAI SQLite store.
type StoreBrainInjector struct {
	store *store.Store
}

var _ BrainInjector = (*StoreBrainInjector)(nil)

// NewStoreBrainInjector creates a BrainInjector backed by the given store.
func NewStoreBrainInjector(s *store.Store) *StoreBrainInjector {
	return &StoreBrainInjector{store: s}
}

// schemaDescription is the hardcoded schema summary included in every preamble.
const schemaDescription = `### Schema: runs table (read-only)
Columns: id (INTEGER PK), kind (TEXT), name (TEXT), started_at (INTEGER unix-ms),
finished_at (INTEGER unix-ms, nullable), exit_status (INTEGER, nullable),
stdout (TEXT), stderr (TEXT), metadata (TEXT JSON), steps (TEXT JSON array).
This table is READ-ONLY. Do not issue INSERT, UPDATE, or DELETE against it.`

// ReadContext assembles a brain context preamble for the given runID.
// It always includes a schema summary. If brain notes exist for the run,
// they are appended (capped at 10, individual bodies truncated to 500 chars).
// If fetching notes fails the error is silently dropped and the schema-only
// preamble is returned.
func (s *StoreBrainInjector) ReadContext(ctx context.Context, runID int64) (string, error) {
	var sb strings.Builder

	sb.WriteString("## ORCAI Database Context\n\n")
	sb.WriteString(schemaDescription)
	sb.WriteString("\n")

	notes, err := s.store.RecentBrainNotes(ctx, runID, 10)
	if err != nil {
		// Degrade gracefully: return schema-only preamble.
		sb.WriteString("\n> Do NOT modify the runs table.\n")
		sb.WriteString(brainWriteInstruction)
		return sb.String(), nil
	}

	if len(notes) > 0 {
		sb.WriteString("\n## Brain Notes (this run)\n\n")
		for _, n := range notes {
			body := n.Body
			runes := []rune(body)
			if len(runes) > 4000 {
				body = string(runes[:4000]) + "...[truncated]"
			}
			sb.WriteString(fmt.Sprintf("[%s] %s\n", n.StepID, body))
		}
	}

	sb.WriteString("\n> Do NOT modify the runs table.\n")
	sb.WriteString(brainWriteInstruction)
	return sb.String(), nil
}
