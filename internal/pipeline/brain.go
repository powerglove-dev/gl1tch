package pipeline

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/8op-org/gl1tch/internal/store"
	"github.com/8op-org/gl1tch/internal/systemprompts"
)

// injectBrainContext applies brain pre-context modifications to a prompt string.
// It is called by both the legacy and DAG execution paths before dispatching to a plugin.
//   - If a BrainInjector is configured, the read preamble (which includes the
//     write instruction) is always prepended.
//   - Otherwise the write instruction is always appended — brain is always on.
func injectBrainContext(ctx context.Context, prompt string, _ *Pipeline, step *Step, ec *ExecutionContext) string {
	if inj := ec.GetBrainInjector(); inj != nil {
		if preamble, err := inj.ReadContext(ctx, ec.RunID()); err == nil && preamble != "" {
			prompt = preamble + "\n\n" + prompt
		} else if err != nil {
			fmt.Fprintf(os.Stderr, "[debug] brain read context error for step %q: %v\n", step.ID, err)
		}
		// BrainInjector preamble already includes the write instruction.
		return prompt
	}
	// No injector — append write instruction unconditionally.
	return prompt + systemprompts.Load(systemprompts.BrainWrite)
}

// TODO: future flag strip_brain_output: true on Pipeline could strip the <brain>
// block from the user-visible output string before it is published to the feed/inbox.
// Currently the <brain> XML remains visible in step output.

// brainBlockRe matches <brain ...> blocks in agent output (primary format).
var brainBlockRe = regexp.MustCompile(`(?s)<brain\b([^>]*?)>(.*?)</brain>`)

// brainNotesRe matches legacy <brain_notes> blocks for backward compatibility.
var brainNotesRe = regexp.MustCompile(`(?s)<brain_notes\b[^>]*>(.*?)</brain_notes>`)

// brainAttrRe extracts key="value" or key='value' attribute pairs.
var brainAttrRe = regexp.MustCompile(`(\w+)=["']([^"']*)["']`)

// parsedBrainBlock holds the parsed content of a <brain> or <brain_notes> block.
type parsedBrainBlock struct {
	body  string
	type_ string // "research", "finding", "data", "code", or ""
	title string
	tags  string // comma-separated user tags
}

// extractBrainBlock finds the first <brain> or <brain_notes> block in output
// and returns its parsed content. <brain> takes priority; <brain_notes> is the
// backward-compatible fallback.
func extractBrainBlock(output string) (parsedBrainBlock, bool) {
	// Try <brain ...> first (primary format).
	if m := brainBlockRe.FindStringSubmatch(output); m != nil {
		attrs := parseAttrs(m[1])
		return parsedBrainBlock{
			body:  strings.TrimSpace(m[2]),
			type_: attrs["type"],
			title: attrs["title"],
			tags:  attrs["tags"],
		}, true
	}
	// Fall back to legacy <brain_notes> — no structured attributes, tags stay empty.
	if m := brainNotesRe.FindStringSubmatch(output); m != nil {
		return parsedBrainBlock{
			body: strings.TrimSpace(m[1]),
		}, true
	}
	return parsedBrainBlock{}, false
}

// parseAttrs extracts key="value" pairs from an XML attribute string.
func parseAttrs(attrStr string) map[string]string {
	attrs := make(map[string]string)
	for _, m := range brainAttrRe.FindAllStringSubmatch(attrStr, -1) {
		attrs[m[1]] = m[2]
	}
	return attrs
}

// buildTagsColumn encodes <brain> structured attributes into the tags column.
// Format: "type:finding title:My Note tags:tag1,tag2" (only non-empty fields included).
func buildTagsColumn(b parsedBrainBlock) string {
	var parts []string
	if b.type_ != "" {
		parts = append(parts, "type:"+b.type_)
	}
	if b.title != "" {
		parts = append(parts, "title:"+b.title)
	}
	if b.tags != "" {
		parts = append(parts, "tags:"+b.tags)
	}
	return strings.Join(parts, " ")
}

// parseBrainBlock scans output for a <brain> or <brain_notes> XML block and
// persists it to the store. It is a best-effort operation: missing or malformed
// blocks are silently skipped with a debug log. The step never fails due to
// brain parsing errors. Called for every step when a store is available.
func parseBrainBlock(ctx context.Context, output, stepID string, ec *ExecutionContext) {
	block, ok := extractBrainBlock(output)
	if !ok {
		fmt.Fprintf(os.Stderr, "[debug] brain step %q: no <brain> block found in output\n", stepID)
		return
	}
	if block.body == "" {
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
		Tags:      buildTagsColumn(block),
		Body:      block.body,
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

// StoreBrainInjector is the default BrainInjector backed by the GLITCH SQLite store.
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
// they are appended (capped at 10, individual bodies truncated to 4000 chars).
// If fetching notes fails the error is silently dropped and the schema-only
// preamble is returned.
func (s *StoreBrainInjector) ReadContext(ctx context.Context, runID int64) (string, error) {
	var sb strings.Builder

	sb.WriteString("## GLITCH Database Context\n\n")
	sb.WriteString(schemaDescription)
	sb.WriteString("\n")

	notes, err := s.store.RecentBrainNotes(ctx, runID, 10)
	if err != nil {
		// Degrade gracefully: return schema-only preamble.
		sb.WriteString("\n> Do NOT modify the runs table.\n")
		sb.WriteString(systemprompts.Load(systemprompts.BrainWrite))
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
			meta := ""
			if n.Tags != "" {
				meta = " [" + n.Tags + "]"
			}
			sb.WriteString(fmt.Sprintf("[%s]%s %s\n", n.StepID, meta, body))
		}
	}

	sb.WriteString("\n> Do NOT modify the runs table.\n")
	sb.WriteString(systemprompts.Load(systemprompts.BrainWrite))
	return sb.String(), nil
}
