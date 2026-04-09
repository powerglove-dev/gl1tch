// attention_classify.go implements `glitch attention classify`.
//
// This subcommand is a direct smoke-test for the attention
// classifier: take one or more AnalyzableEvents, hand them to the
// local LLM via ClassifyAttention, and print the verdicts. It does
// NOT touch the deep-analysis opencode path — use
// `glitch attention analyze` for that.
//
// Three input modes, exactly one must be chosen:
//
//	--from-pr owner/repo#N   build one event by shelling out to `gh pr view`
//	--stdin                  read a JSON array of events from stdin
//	(neither)                error — ambiguous, we refuse to guess
//
// Output: either a human-readable table (default) or a JSON array
// of verdicts (`--json`) so shell scripts and the smoke-test suite
// can parse the result deterministically.
package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/8op-org/gl1tch/pkg/glitchd"
)

var (
	classifyFromPR fromPRFlag
	classifyStdin  bool
	classifyJSON   bool
)

var attentionClassifyCmd = &cobra.Command{
	Use:   "classify",
	Short: "Run the attention classifier against one or more events",
	Long: `Classify events with the local attention model (qwen2.5:7b by default)
and print the verdicts.

Input (pick exactly one):
  --from-pr owner/repo#N   build one event from a github PR via gh
  --stdin                  read a JSON array of events from stdin

Output:
  default                  human table: level, reason, event header
  --json                   JSON array of verdicts (for scripts / smoke tests)

Examples:

  glitch attention classify --from-pr elastic/ensemble#1246
  echo '[{"source":"github","type":"github.pr","author":"amannocci","title":"review"}]' \
      | glitch attention classify --stdin --json
`,
	RunE: func(cmd *cobra.Command, _ []string) error {
		events, err := gatherClassifyEvents(cmd.Context())
		if err != nil {
			return err
		}
		if len(events) == 0 {
			return fmt.Errorf("no events to classify — pass --from-pr or --stdin")
		}

		verdicts, err := glitchd.ClassifyAttention(cmd.Context(), events, attentionWorkspace)
		if err != nil {
			return fmt.Errorf("classify: %w", err)
		}

		if classifyJSON {
			return printClassifyJSON(events, verdicts)
		}
		return printClassifyTable(events, verdicts)
	},
}

func init() {
	classifyFromPR.register(attentionClassifyCmd)
	attentionClassifyCmd.Flags().BoolVar(&classifyStdin, "stdin", false,
		"read a JSON array of AnalyzableEvent objects from stdin")
	attentionClassifyCmd.Flags().BoolVar(&classifyJSON, "json", false,
		"output verdicts as JSON instead of a table")
}

// gatherClassifyEvents resolves the input flags into a concrete
// []AnalyzableEvent. Exactly one of --from-pr / --stdin must be
// set; specifying both is an error so a mistaken invocation fails
// fast instead of silently ignoring one input.
func gatherClassifyEvents(ctx context.Context) ([]glitchd.AnalyzableEvent, error) {
	owner, repo, number, hasPR, err := parseFromPR(classifyFromPR.raw)
	if err != nil {
		return nil, err
	}
	if hasPR && classifyStdin {
		return nil, fmt.Errorf("--from-pr and --stdin are mutually exclusive")
	}

	switch {
	case hasPR:
		return eventsFromPR(ctx, owner, repo, number, attentionWorkspace)
	case classifyStdin:
		return eventsFromStdin()
	}
	return nil, nil
}

// printClassifyTable writes a padded, aligned table of verdicts to
// stdout. The format is:
//
//	LEVEL   REASON                                  EVENT
//	high    review on your PR                       github/github.pr #1246 …
//
// Designed for quick visual scanning during manual smoke tests.
func printClassifyTable(events []glitchd.AnalyzableEvent, verdicts []glitchd.AttentionVerdict) error {
	tw := tabwriter.NewWriter(os.Stdout, 0, 2, 2, ' ', 0)
	fmt.Fprintln(tw, "LEVEL\tREASON\tEVENT")
	for i, ev := range events {
		v := glitchd.AttentionVerdict{Level: "?", Reason: "(no verdict)"}
		if i < len(verdicts) {
			v = verdicts[i]
		}
		title := ev.Title
		if len(title) > 60 {
			title = title[:57] + "…"
		}
		reason := v.Reason
		if len(reason) > 60 {
			reason = reason[:57] + "…"
		}
		fmt.Fprintf(tw, "%s\t%s\t%s/%s %s\n",
			v.Level, reason, ev.Source, ev.Type, title)
	}
	return tw.Flush()
}

// printClassifyJSON writes a JSON array of {event_title, level,
// reason} objects to stdout. Chosen shape: flat, one object per
// event, so `jq` scripts can pluck fields without nesting.
func printClassifyJSON(events []glitchd.AnalyzableEvent, verdicts []glitchd.AttentionVerdict) error {
	type row struct {
		EventTitle string `json:"event_title"`
		Source     string `json:"source"`
		Type       string `json:"type"`
		Repo       string `json:"repo,omitempty"`
		Author     string `json:"author,omitempty"`
		Level      string `json:"level"`
		Reason     string `json:"reason"`
	}
	out := make([]row, 0, len(events))
	for i, ev := range events {
		var level, reason string
		if i < len(verdicts) {
			level = verdicts[i].Level
			reason = verdicts[i].Reason
		}
		out = append(out, row{
			EventTitle: ev.Title,
			Source:     ev.Source,
			Type:       ev.Type,
			Repo:       ev.Repo,
			Author:     ev.Author,
			Level:      level,
			Reason:     reason,
		})
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}
