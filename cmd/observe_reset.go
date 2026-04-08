// observe_reset.go implements `glitch observe reset` — a
// scoped wipe of the observer's collected state so you can start
// classification and deep-analysis smoke tests from a clean slate
// without losing the brain the user has built up across pipeline
// runs.
//
// What it clears:
//
//  1. glitch-events       (the raw observation feed)
//  2. glitch-analyses     (deep-analysis markdown artifacts)
//  3. analysis_dedupe     (SQLite table that prevents re-analysis)
//
// What it LEAVES ALONE — on purpose:
//
//  1. glitch-vectors      (code-index embeddings — expensive to rebuild)
//  2. brain_notes         (SQLite table of brain learning from runs)
//  3. all other SQLite tables (workspaces, directories, cron, etc.)
//  4. the research prompt files on disk
//
// The separation matters because the user's explicit ask is "clear
// out the events but I don't want to lose the brain learning" — the
// two live in different stores, so this command is the one place
// that knows which is which and cleans only the event-side half.
package cmd

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/8op-org/gl1tch/internal/capability"
	"github.com/8op-org/gl1tch/internal/esearch"
	"github.com/8op-org/gl1tch/internal/store"
)

var observeResetYes bool

func init() {
	observeCmd.AddCommand(observeResetCmd)
	observeResetCmd.Flags().BoolVarP(&observeResetYes, "yes", "y", false,
		"skip the confirmation prompt (for scripts and smoke tests)")
}

var observeResetCmd = &cobra.Command{
	Use:   "reset",
	Short: "Clear collected events and analyses (preserves brain and code index)",
	Long: `Clear the observer's raw event feed and deep-analysis history so the
next collector tick starts from a fresh slate.

What this clears:
  • glitch-events       — the raw observation feed from git/github/etc.
  • glitch-analyses     — deep-analysis markdown artifacts
  • analysis_dedupe     — the SQLite table that prevents re-analysis

What this keeps (on purpose):
  • glitch-vectors      — code-index embeddings (expensive to rebuild)
  • brain_notes         — brain learning from pipeline runs
  • all other SQLite tables (workspaces, directories, cron, …)

The typical use is smoke-testing the attention classifier after
fixing an event-pipeline bug: reset, let the collectors re-ingest,
and watch the classifier fire on freshly-indexed events.

Examples:

  glitch observe reset           # prompts for confirmation
  glitch observe reset -y        # no prompt (scripts / smoke tests)
`,
	RunE: func(cmd *cobra.Command, _ []string) error {
		ctx, cancel := context.WithTimeout(cmd.Context(), 60*time.Second)
		defer cancel()

		if !observeResetYes {
			fmt.Fprintln(os.Stderr,
				"This will delete every document in glitch-events and glitch-analyses,")
			fmt.Fprintln(os.Stderr,
				"and truncate the analysis_dedupe SQLite table.")
			fmt.Fprintln(os.Stderr,
				"Brain notes and the code index (glitch-vectors) will be preserved.")
			fmt.Fprint(os.Stderr, "Continue? [y/N] ")
			var answer string
			_, _ = fmt.Fscanln(os.Stdin, &answer)
			if answer != "y" && answer != "Y" && answer != "yes" {
				return fmt.Errorf("aborted")
			}
		}

		// Resolve the ES address the same way the rest of the
		// observer does so a user who moved ES onto a non-default
		// port doesn't have to pass a flag here too.
		addr := "http://localhost:9200"
		if cfg, _ := capability.LoadConfig(); cfg != nil && cfg.Elasticsearch.Address != "" {
			addr = cfg.Elasticsearch.Address
		}

		es, err := esearch.New(addr)
		if err != nil {
			return fmt.Errorf("elasticsearch: %w", err)
		}

		// match_all with DeleteByQuery keeps the index mappings in
		// place (we'd have to reapply them if we dropped the index
		// outright) while still flushing every doc. Refresh=true on
		// the DeleteByQuery path so a subsequent search shows zero
		// hits immediately, which matters when the smoke test runs
		// reset → classify in the same shell.
		//
		// esearch.Client.DeleteByQuery wraps the argument in a
		// {"query": ...} envelope itself, so we pass just the inner
		// query body — double-wrapping triggers an ES 400 parse error.
		matchAll := map[string]any{"match_all": map[string]any{}}

		eventsDeleted, err := es.DeleteByQuery(ctx,
			[]string{esearch.IndexEvents}, matchAll)
		if err != nil {
			// A 404 means the index never existed yet. Not an
			// error — a fresh install hits this and deserves a
			// quiet success so the reset command is idempotent.
			fmt.Fprintf(os.Stderr, "warn: delete %s: %v\n",
				esearch.IndexEvents, err)
		}

		analysesDeleted, err := es.DeleteByQuery(ctx,
			[]string{esearch.IndexAnalyses}, matchAll)
		if err != nil {
			fmt.Fprintf(os.Stderr, "warn: delete %s: %v\n",
				esearch.IndexAnalyses, err)
		}

		dedupeDeleted, err := clearAnalysisDedupe(ctx)
		if err != nil {
			return fmt.Errorf("clear analysis_dedupe: %w", err)
		}

		fmt.Fprintf(os.Stderr, "reset complete\n")
		fmt.Fprintf(os.Stderr, "  %s:    %d docs deleted\n",
			esearch.IndexEvents, eventsDeleted)
		fmt.Fprintf(os.Stderr, "  %s:  %d docs deleted\n",
			esearch.IndexAnalyses, analysesDeleted)
		fmt.Fprintf(os.Stderr, "  analysis_dedupe:  %d rows deleted\n",
			dedupeDeleted)
		fmt.Fprintln(os.Stderr,
			"brain_notes and glitch-vectors preserved")
		return nil
	},
}

// clearAnalysisDedupe opens the store, truncates the dedupe table,
// and closes the handle. Kept in its own helper so the main reset
// flow reads as a sequence of three deletes without the SQLite
// boilerplate inline.
func clearAnalysisDedupe(ctx context.Context) (int64, error) {
	s, err := store.Open()
	if err != nil {
		return 0, err
	}
	defer s.Close()
	return s.ClearAnalysisDedupe(ctx)
}
