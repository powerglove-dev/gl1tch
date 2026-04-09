// attention.go wires the `glitch attention` cobra command and its
// three subcommands:
//
//	glitch attention research   — view/edit the workspace research prompt
//	glitch attention classify   — run the attention classifier against events
//	glitch attention analyze    — run the full deep-analysis pipeline on one event
//
// The point of this surface is smoke-testing the AI-first attention
// stack end-to-end without the desktop UI, without the collector
// loop, and without Elasticsearch. Every subcommand accepts input
// through plain mechanisms (stdin JSON, a `--from-pr owner/repo#N`
// flag that shells out to `gh`) so a fresh dev environment can
// exercise the whole ladder in one command:
//
//	glitch attention analyze --from-pr elastic/ensemble#1246
//
// Shared flags live here; subcommand-specific flags and the actual
// RunE handlers live in attention_research.go, attention_classify.go,
// and attention_analyze.go so each file stays focused on one verb.
package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/8op-org/gl1tch/pkg/glitchd"
)

// attentionWorkspace is the workspace id every subcommand scopes its
// prompt / config lookups to. Empty means "use the global fallback
// files under ~/.config/glitch/" — useful for quick tests before a
// workspace exists.
var attentionWorkspace string

func init() {
	rootCmd.AddCommand(attentionCmd)
	attentionCmd.PersistentFlags().StringVarP(&attentionWorkspace, "workspace", "w", "",
		"workspace id to scope prompts/config to (empty = global fallback)")

	attentionCmd.AddCommand(attentionResearchCmd)
	attentionCmd.AddCommand(attentionClassifyCmd)
	attentionCmd.AddCommand(attentionAnalyzeCmd)
}

var attentionCmd = &cobra.Command{
	Use:   "attention",
	Short: "Smoke-test the attention classifier and deep-analysis artifact loop",
	Long: strings.TrimSpace(`
Smoke-test the AI-first attention stack end-to-end without the
desktop UI or the collector loop.

Subcommands:
  research   view, edit, or print the workspace research prompt
  classify   classify one or more events with the local attention model
  analyze    classify + run deep analysis (artifact mode for high events)

The whole surface is designed so you can drive the full ladder
against a real github PR with one command, e.g.:

    glitch attention analyze --from-pr elastic/ensemble#1246

No Elasticsearch, no collector, no desktop needed. Ollama and (for
analyze) opencode must be running locally.
`),
}

// ── shared helpers ────────────────────────────────────────────────

// fromPR is the shared --from-pr flag used by classify and analyze.
// Parsed by parseFromPR into (owner, repo, number).
type fromPRFlag struct {
	raw string
}

func (f *fromPRFlag) register(cmd *cobra.Command) {
	cmd.Flags().StringVar(&f.raw, "from-pr", "",
		"build an AnalyzableEvent from a github PR (format: owner/repo#number, e.g. elastic/ensemble#1246)")
}

// parseFromPR splits "owner/repo#number" into its components.
// Returns ok=false when the value is empty; errors only when the
// value is present but malformed so callers can distinguish "flag
// not set" from "flag set wrong".
func parseFromPR(raw string) (owner, repo string, number int, ok bool, err error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", "", 0, false, nil
	}
	slash := strings.Index(raw, "/")
	hash := strings.Index(raw, "#")
	if slash < 0 || hash < 0 || hash < slash {
		return "", "", 0, false,
			fmt.Errorf("--from-pr %q: expected owner/repo#number", raw)
	}
	owner = raw[:slash]
	repo = raw[slash+1 : hash]
	numStr := raw[hash+1:]
	if owner == "" || repo == "" || numStr == "" {
		return "", "", 0, false,
			fmt.Errorf("--from-pr %q: owner, repo, and number are all required", raw)
	}
	if _, err := fmt.Sscanf(numStr, "%d", &number); err != nil || number <= 0 {
		return "", "", 0, false,
			fmt.Errorf("--from-pr %q: %q is not a positive integer", raw, numStr)
	}
	return owner, repo, number, true, nil
}

// eventsFromPR builds a slice of AnalyzableEvents from a live github
// PR by shelling out to `gh pr view --json`. Each review and comment
// becomes its own event with the correct type and author — matching
// what the real collector emits. The PR itself is always the first
// event; reviews and comments follow in chronological order.
//
// This lets smoke tests exercise the same classifier code paths the
// production pipeline uses, including the "my own activity" filter.
func eventsFromPR(ctx context.Context, owner, repo string, number int, workspaceID string) ([]glitchd.AnalyzableEvent, error) {
	slug := owner + "/" + repo
	ghCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ghCtx, "gh", "pr", "view",
		fmt.Sprintf("%d", number),
		"--repo", slug,
		"--json", "number,title,body,author,url,updatedAt,state,comments,reviews")
	cmd.Stderr = os.Stderr
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("gh pr view %s#%d: %w", slug, number, err)
	}

	var parsed struct {
		Number  int    `json:"number"`
		Title   string `json:"title"`
		Body    string `json:"body"`
		Author  struct {
			Login string `json:"login"`
		} `json:"author"`
		URL       string    `json:"url"`
		UpdatedAt time.Time `json:"updatedAt"`
		State     string    `json:"state"`
		Comments  []struct {
			Author struct {
				Login string `json:"login"`
			} `json:"author"`
			Body      string    `json:"body"`
			CreatedAt time.Time `json:"createdAt"`
		} `json:"comments"`
		Reviews []struct {
			Author struct {
				Login string `json:"login"`
			} `json:"author"`
			Body        string    `json:"body"`
			State       string    `json:"state"`
			ID          string    `json:"id"`
			SubmittedAt time.Time `json:"submittedAt"`
		} `json:"reviews"`
	}
	if err := json.Unmarshal(out, &parsed); err != nil {
		return nil, fmt.Errorf("decode gh pr view output: %w", err)
	}

	prTitle := fmt.Sprintf("#%d %s", parsed.Number, parsed.Title)

	var events []glitchd.AnalyzableEvent

	// 1. The PR itself — type github.pr, author is the PR author.
	events = append(events, glitchd.AnalyzableEvent{
		Type:        "github.pr",
		Source:      "github",
		Repo:        slug,
		Author:      parsed.Author.Login,
		Title:       prTitle,
		Body:        truncateStr(parsed.Body, 3000),
		Identifier:  fmt.Sprintf("%d", parsed.Number),
		URL:         parsed.URL,
		WorkspaceID: workspaceID,
		Timestamp:   parsed.UpdatedAt,
	})

	// 2. Each review → type github.pr_review, author is the reviewer.
	for _, r := range parsed.Reviews {
		id := r.ID
		if id == "" {
			id = fmt.Sprintf("%d:review:%s:%s", parsed.Number, r.Author.Login, r.SubmittedAt.Format(time.RFC3339))
		}
		events = append(events, glitchd.AnalyzableEvent{
			Type:        "github.pr_review",
			Source:      "github",
			Repo:        slug,
			Author:      r.Author.Login,
			Title:       fmt.Sprintf("Review on PR %s: %s", prTitle, r.State),
			Body:        truncateStr(r.Body, 2000),
			Identifier:  id,
			URL:         parsed.URL,
			WorkspaceID: workspaceID,
			Timestamp:   r.SubmittedAt,
		})
	}

	// 3. Each comment → type github.pr_comment, author is the commenter.
	for _, c := range parsed.Comments {
		id := fmt.Sprintf("%d:comment:%s:%s", parsed.Number, c.Author.Login, c.CreatedAt.Format(time.RFC3339))
		events = append(events, glitchd.AnalyzableEvent{
			Type:        "github.pr_comment",
			Source:      "github",
			Repo:        slug,
			Author:      c.Author.Login,
			Title:       fmt.Sprintf("Comment on PR %s", prTitle),
			Body:        truncateStr(c.Body, 2000),
			Identifier:  id,
			URL:         parsed.URL,
			WorkspaceID: workspaceID,
			Timestamp:   c.CreatedAt,
		})
	}

	return events, nil
}

// truncateStr caps s at n bytes, appending "…" if truncated.
func truncateStr(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-1] + "…"
}

// eventsFromStdin reads a JSON array of AnalyzableEvent-shaped
// objects from stdin. Used by `glitch attention classify --stdin`
// so tests can feed known payloads without the gh shell-out. The
// accepted shape is a subset of AnalyzableEvent's JSON form — the
// classifier only reads a handful of fields so we don't require the
// full struct.
func eventsFromStdin() ([]glitchd.AnalyzableEvent, error) {
	dec := json.NewDecoder(os.Stdin)
	var raw []struct {
		Type            string `json:"type"`
		Source          string `json:"source"`
		Repo            string `json:"repo"`
		Author          string `json:"author"`
		Title           string `json:"title"`
		Body            string `json:"body"`
		Identifier      string `json:"identifier"`
		URL             string `json:"url"`
		WorkspaceID     string `json:"workspace_id"`
		Attention       string `json:"attention"`
		AttentionReason string `json:"attention_reason"`
	}
	if err := dec.Decode(&raw); err != nil {
		return nil, fmt.Errorf("decode stdin events: %w", err)
	}
	out := make([]glitchd.AnalyzableEvent, 0, len(raw))
	for _, r := range raw {
		out = append(out, glitchd.AnalyzableEvent{
			Type:            r.Type,
			Source:          r.Source,
			Repo:            r.Repo,
			Author:          r.Author,
			Title:           r.Title,
			Body:            r.Body,
			Identifier:      r.Identifier,
			URL:             r.URL,
			WorkspaceID:     r.WorkspaceID,
			Attention:       r.Attention,
			AttentionReason: r.AttentionReason,
		})
	}
	return out, nil
}

// printEventSummary writes a one-line identification for an event
// to the given writer. Shared by classify and analyze so the user
// sees the same "this is what we ran on" header regardless of
// which subcommand produced it.
func printEventSummary(w *os.File, ev glitchd.AnalyzableEvent) {
	fmt.Fprintf(w, "event: %s/%s %s", ev.Source, ev.Type, ev.Title)
	if ev.Repo != "" {
		fmt.Fprintf(w, " [%s]", ev.Repo)
	}
	if ev.Author != "" {
		fmt.Fprintf(w, " @%s", ev.Author)
	}
	fmt.Fprintln(w)
}
