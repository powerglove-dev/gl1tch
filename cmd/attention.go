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
	"bytes"
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

// eventFromPR builds an AnalyzableEvent from a live github PR by
// shelling out to `gh pr view --json`. This lets smoke tests work
// without ES or the collector — the user points at any real PR and
// the rest of the pipeline gets a realistic event to chew on.
//
// Fields mapped from the gh response:
//   - Title       ← "#<n> <title>"
//   - Body        ← body + the last few review/issue comments
//   - Author      ← author.login
//   - Repo        ← "owner/repo"
//   - URL         ← url
//   - Identifier  ← number as string
//   - Timestamp   ← updatedAt (so stale-event filter doesn't bite on old PRs)
//
// Returns an error when gh is missing, the PR can't be fetched, or
// the JSON shape is unexpected.
func eventFromPR(ctx context.Context, owner, repo string, number int, workspaceID string) (glitchd.AnalyzableEvent, error) {
	slug := owner + "/" + repo
	ghCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	// Pull the fields we need in a single call. Comments/reviews
	// give the classifier something concrete to latch onto when the
	// user's research prompt is keyed on "someone reviewed my PR".
	cmd := exec.CommandContext(ghCtx, "gh", "pr", "view",
		fmt.Sprintf("%d", number),
		"--repo", slug,
		"--json", "number,title,body,author,url,updatedAt,state,comments,reviews")
	cmd.Stderr = os.Stderr
	out, err := cmd.Output()
	if err != nil {
		return glitchd.AnalyzableEvent{}, fmt.Errorf("gh pr view %s#%d: %w", slug, number, err)
	}

	var parsed struct {
		Number    int    `json:"number"`
		Title     string `json:"title"`
		Body      string `json:"body"`
		Author    struct {
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
			Body      string    `json:"body"`
			State     string    `json:"state"`
			SubmittedAt time.Time `json:"submittedAt"`
		} `json:"reviews"`
	}
	if err := json.Unmarshal(out, &parsed); err != nil {
		return glitchd.AnalyzableEvent{}, fmt.Errorf("decode gh pr view output: %w", err)
	}

	// Stitch a body the classifier can reason about: the PR
	// description followed by the most recent comments and reviews.
	// Not the whole thread — we cap to keep the prompt bounded.
	var body bytes.Buffer
	if strings.TrimSpace(parsed.Body) != "" {
		body.WriteString("## PR description\n")
		body.WriteString(parsed.Body)
		body.WriteString("\n")
	}
	const maxThreadItems = 6
	items := 0
	for i := len(parsed.Reviews) - 1; i >= 0 && items < maxThreadItems; i-- {
		r := parsed.Reviews[i]
		fmt.Fprintf(&body, "\n## Review by @%s (%s)\n%s\n",
			r.Author.Login, r.State, strings.TrimSpace(r.Body))
		items++
	}
	for i := len(parsed.Comments) - 1; i >= 0 && items < maxThreadItems; i-- {
		c := parsed.Comments[i]
		fmt.Fprintf(&body, "\n## Comment by @%s\n%s\n",
			c.Author.Login, strings.TrimSpace(c.Body))
		items++
	}

	return glitchd.AnalyzableEvent{
		Type:        "github.pr",
		Source:      "github",
		Repo:        slug,
		Author:      parsed.Author.Login,
		Title:       fmt.Sprintf("#%d %s", parsed.Number, parsed.Title),
		Body:        body.String(),
		Identifier:  fmt.Sprintf("%d", parsed.Number),
		URL:         parsed.URL,
		WorkspaceID: workspaceID,
		Timestamp:   parsed.UpdatedAt,
	}, nil
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
