package pipeline

import (
	"context"
	"fmt"
	"strings"

	"github.com/adam-stokes/orcai/internal/store"
)

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
			truncated := false
			if len(body) > 500 {
				body = body[:500]
				truncated = true
			}
			line := fmt.Sprintf("[%s] %s", n.StepID, body)
			if truncated {
				line += "...[truncated]"
			}
			sb.WriteString(line)
			sb.WriteString("\n")
		}
	}

	sb.WriteString("\n> Do NOT modify the runs table. Use the write_brain mechanism to persist insights.\n")
	return sb.String(), nil
}
