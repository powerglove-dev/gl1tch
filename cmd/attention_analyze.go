// attention_analyze.go implements `glitch attention analyze` —
// the end-to-end smoke driver for the whole attention + deep
// analysis ladder.
//
// Flow (all local, no collector, no ES, no desktop):
//
//  1. Build one AnalyzableEvent from --from-pr (via gh) or --stdin.
//  2. Run ClassifyAttention on it to get a verdict.
//  3. Call AnalyzeOne, which builds the right prompt (artifact-mode
//     for high, summary-mode otherwise) and shells out to opencode.
//  4. Print the classifier verdict, the model used, and the markdown
//     artifact. With --json emit a machine-readable envelope instead
//     so smoke tests can assert on fields without parsing markdown.
//
// --force-high bypasses the classifier and stamps Attention=high on
// the event directly, so you can exercise the artifact template
// even when the classifier would have voted normal. This is the
// right tool for validating the deep_analysis_artifact.md rubric
// in isolation from the classifier's judgment.
package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/8op-org/gl1tch/internal/capability"
	"github.com/8op-org/gl1tch/pkg/glitchd"
)

var (
	analyzeFromPR    fromPRFlag
	analyzeStdin     bool
	analyzeJSON      bool
	analyzeForceHigh bool
	analyzeSkipClass bool
)

var attentionAnalyzeCmd = &cobra.Command{
	Use:   "analyze",
	Short: "Classify + deep-analyze one event end-to-end",
	Long: `Run the full attention + deep-analysis pipeline on a single event
and print the generated artifact.

Input (pick exactly one):
  --from-pr owner/repo#N   build one event from a github PR via gh
  --stdin                  read a single JSON AnalyzableEvent from stdin
                           (accepts either a JSON object or a one-element array)

Mode flags:
  --force-high             stamp Attention=high before running (skips classifier
                           and always uses the artifact-mode rubric)
  --no-classify            skip the classifier entirely; the event's existing
                           Attention field is honoured (empty → summary mode)
  --json                   emit a JSON envelope instead of the default
                           human-readable output (for smoke tests)

Examples:

  glitch attention analyze --from-pr elastic/ensemble#1246
  glitch attention analyze --from-pr elastic/ensemble#1246 --force-high
  glitch attention analyze --stdin --json < event.json
`,
	RunE: func(cmd *cobra.Command, _ []string) error {
		return runAnalyze(cmd.Context())
	},
}

func init() {
	analyzeFromPR.register(attentionAnalyzeCmd)
	attentionAnalyzeCmd.Flags().BoolVar(&analyzeStdin, "stdin", false,
		"read one AnalyzableEvent from stdin (object or single-element array)")
	attentionAnalyzeCmd.Flags().BoolVar(&analyzeJSON, "json", false,
		"output a JSON envelope instead of human-readable text")
	attentionAnalyzeCmd.Flags().BoolVar(&analyzeForceHigh, "force-high", false,
		"stamp Attention=high on the event (skips classifier; always artifact mode)")
	attentionAnalyzeCmd.Flags().BoolVar(&analyzeSkipClass, "no-classify", false,
		"skip the classifier; honour the event's existing Attention field verbatim")
}

func runAnalyze(ctx context.Context) error {
	events, err := gatherAnalyzeEvents(ctx)
	if err != nil {
		return err
	}

	// Stage 1: classify all events (unless bypassed), then pick the
	// most interesting one to analyze. Priority: high > normal > low.
	// Within a tier, prefer non-PR events (reviews, comments) over
	// the bare PR event since those carry the actionable signal.
	switch {
	case analyzeForceHigh:
		for i := range events {
			events[i].Attention = glitchd.AttentionHigh
			if events[i].AttentionReason == "" {
				events[i].AttentionReason = "forced via --force-high"
			}
		}
	case analyzeSkipClass:
		// Honour whatever the caller put on the events.
	default:
		verdicts, err := glitchd.ClassifyAttention(ctx, events, attentionWorkspace)
		if err != nil {
			return fmt.Errorf("classify: %w", err)
		}
		for i := range events {
			if i < len(verdicts) {
				events[i].Attention = verdicts[i].Level
				events[i].AttentionReason = verdicts[i].Reason
			}
		}
	}

	ev := pickBestEvent(events)

	// Stage 2: deep analyze. We load the global config here so the
	// analyzer picks up the operator's preferred coder model; the
	// function tolerates nil, so loading failures fall back to the
	// baked-in default rather than aborting the command.
	cfg, _ := capability.LoadConfig()
	result := glitchd.AnalyzeOne(ctx, ev, cfg)

	if analyzeJSON {
		return printAnalyzeJSON(ev, result)
	}
	return printAnalyzeHuman(ev, result)
}

// pickBestEvent selects the event most worth analyzing from a batch.
// High > normal > low. Within a tier, prefer review/comment events
// over bare PR events since those carry the actionable signal.
func pickBestEvent(events []glitchd.AnalyzableEvent) glitchd.AnalyzableEvent {
	if len(events) == 0 {
		return glitchd.AnalyzableEvent{}
	}
	tierRank := func(level glitchd.AttentionLevel) int {
		switch level {
		case glitchd.AttentionHigh:
			return 3
		case glitchd.AttentionNormal:
			return 2
		case glitchd.AttentionLow:
			return 1
		default:
			return 0
		}
	}
	typeRank := func(t string) int {
		switch t {
		case "github.pr_review":
			return 3
		case "github.pr_comment", "github.issue_comment":
			return 2
		case "github.pr", "github.issue":
			return 1
		default:
			return 0
		}
	}
	best := events[0]
	for _, ev := range events[1:] {
		if tierRank(ev.Attention) > tierRank(best.Attention) {
			best = ev
			continue
		}
		if tierRank(ev.Attention) == tierRank(best.Attention) && typeRank(ev.Type) > typeRank(best.Type) {
			best = ev
			continue
		}
		// Within same tier and type, prefer the most recent.
		if tierRank(ev.Attention) == tierRank(best.Attention) && typeRank(ev.Type) == typeRank(best.Type) && ev.Timestamp.After(best.Timestamp) {
			best = ev
		}
	}
	return best
}

// gatherAnalyzeEvents resolves the input flags into a slice of
// events. With --from-pr, this is the PR event plus each review and
// comment as separate events (matching the real collector). The
// caller classifies all of them and picks the most interesting one
// to feed into deep analysis.
func gatherAnalyzeEvents(ctx context.Context) ([]glitchd.AnalyzableEvent, error) {
	owner, repo, number, hasPR, err := parseFromPR(analyzeFromPR.raw)
	if err != nil {
		return nil, err
	}
	if hasPR && analyzeStdin {
		return nil, fmt.Errorf("--from-pr and --stdin are mutually exclusive")
	}
	if !hasPR && !analyzeStdin {
		return nil, fmt.Errorf("no event input — pass --from-pr or --stdin")
	}
	if hasPR {
		return eventsFromPR(ctx, owner, repo, number, attentionWorkspace)
	}
	return eventsFromStdin()
}

// printAnalyzeHuman writes the human-readable report to stdout:
// a header line with the verdict, the model used, and then the
// full markdown artifact as-is so it renders in any terminal that
// supports ANSI.
func printAnalyzeHuman(ev glitchd.AnalyzableEvent, r glitchd.AnalysisResult) error {
	printEventSummary(os.Stdout, ev)
	fmt.Printf("attention: %s — %s\n", orPlaceholder(ev.Attention, "(skipped)"),
		orPlaceholder(ev.AttentionReason, "(no reason)"))
	fmt.Printf("model:     %s\n", r.Model)
	fmt.Printf("duration:  %s\n", r.Duration.Truncate(100_000_000)) // 0.1s precision
	fmt.Printf("exit_code: %d\n", r.ExitCode)
	fmt.Println("──────────────────────────────────────────────")
	if strings.TrimSpace(r.Markdown) == "" {
		fmt.Println("(no markdown output — is opencode installed and the model pulled?)")
		return fmt.Errorf("empty analysis output")
	}
	fmt.Println(r.Markdown)
	return nil
}

// printAnalyzeJSON writes a structured envelope that smoke tests
// and shell scripts can consume with `jq`. The shape mirrors the
// AnalysisResult struct but flattens the fields the caller actually
// needs without leaking internal ones (dedupe key format, etc.).
func printAnalyzeJSON(ev glitchd.AnalyzableEvent, r glitchd.AnalysisResult) error {
	envelope := map[string]any{
		"event": map[string]any{
			"source": ev.Source,
			"type":   ev.Type,
			"repo":   ev.Repo,
			"author": ev.Author,
			"title":  ev.Title,
			"url":    ev.URL,
		},
		"attention": map[string]any{
			"level":  ev.Attention,
			"reason": ev.AttentionReason,
		},
		"analysis": map[string]any{
			"model":       r.Model,
			"markdown":    r.Markdown,
			"exit_code":   r.ExitCode,
			"duration_ms": r.Duration.Milliseconds(),
		},
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(envelope)
}

// orPlaceholder returns s when it is non-empty after trimming,
// otherwise the fallback. Used to render empty attention fields as
// "(skipped)" so the user can tell a bypass apart from a literal
// empty verdict.
func orPlaceholder(s, fallback string) string {
	if strings.TrimSpace(s) == "" {
		return fallback
	}
	return s
}
