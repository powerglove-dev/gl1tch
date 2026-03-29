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

// brainWriteInstruction is appended to the prompt of write_brain steps.
const brainWriteInstruction = `

---
BRAIN WRITE INSTRUCTION: You MUST include a <brain> XML element somewhere in your response to persist an insight for future steps in this pipeline. Format:

  <brain tags="optional,comma,tags">Your insight or summary here</brain>

The brain note will be stored and made available to subsequent agent steps with use_brain enabled.
---`

// TODO: future flag strip_brain_output: true on Pipeline could strip the <brain>
// block from the user-visible output string before it is published to the feed/inbox.
// Currently the <brain> XML remains visible in step output.

var brainBlockRe = regexp.MustCompile(`(?s)<brain(?:\s+tags="([^"]*)")?\s*>(.*?)</brain>`)

// parseBrainBlock scans output for a <brain> XML block and persists it to the store.
// It is a best-effort operation: malformed or missing blocks are silently skipped
// with a debug log. The step never fails due to brain parsing errors.
func parseBrainBlock(ctx context.Context, output, stepID string, ec *ExecutionContext) {
	matches := brainBlockRe.FindStringSubmatch(output)
	if matches == nil {
		fmt.Fprintf(os.Stderr, "[debug] write_brain active for step %q but no <brain> block found in output\n", stepID)
		return
	}
	tags := matches[1]
	body := strings.TrimSpace(matches[2])
	if body == "" {
		return
	}
	s := ec.DB()
	if s == nil {
		fmt.Fprintf(os.Stderr, "[debug] write_brain: no store configured, cannot persist brain note for step %q\n", stepID)
		return
	}
	note := store.BrainNote{
		RunID:     ec.RunID(),
		StepID:    stepID,
		CreatedAt: time.Now().UnixMilli(),
		Tags:      tags,
		Body:      body,
	}
	if _, err := s.InsertBrainNote(ctx, note); err != nil {
		fmt.Fprintf(os.Stderr, "[debug] write_brain: failed to insert brain note for step %q: %v\n", stepID, err)
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
		sb.WriteString("\n> Do NOT modify the runs table. Use the write_brain mechanism to persist insights.\n")
		return sb.String(), nil
	}

	if len(notes) > 0 {
		sb.WriteString("\n## Brain Notes (this run)\n\n")
		for _, n := range notes {
			body := n.Body
			runes := []rune(body)
			if len(runes) > 500 {
				body = string(runes[:500]) + "...[truncated]"
			}
			line := fmt.Sprintf("[%s] %s", n.StepID, body)
			sb.WriteString(line)
			sb.WriteString("\n")
		}
	}

	sb.WriteString("\n> Do NOT modify the runs table. Use the write_brain mechanism to persist insights.\n")
	return sb.String(), nil
}
