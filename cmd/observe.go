package cmd

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/8op-org/gl1tch/internal/observer"
)

func init() {
	rootCmd.AddCommand(observeCmd)
	observeCmd.AddCommand(observeDigestCmd)
	observeCmd.AddCommand(observeStatusCmd)
}

var observeCmd = &cobra.Command{
	Use:   "observe [question]",
	Short: "Ask the gl1tch observer about your dev activity",
	Long: `Query the gl1tch observer — an ES-backed AI that indexes your git repos,
Claude Code conversations, Copilot CLI history, GitHub PRs, and pipeline runs.

Examples:
  glitch observe "what happened today?"
  glitch observe "summarize recent commits on gl1tch"
  glitch observe "what pipelines ran this week?"
  glitch observe "show me Claude Code activity on ensemble"
  glitch observe ingest
  glitch observe digest
  glitch observe status`,
	Args: cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		question := args[0]

		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()

		engine, _, err := observer.QueryOnly(ctx)
		if err != nil {
			fmt.Fprintf(os.Stderr, "observer not available: %v\n", err)
			fmt.Fprintf(os.Stderr, "make sure elasticsearch is running: docker compose up -d\n")
			os.Exit(1)
		}

		tokenCh := make(chan string, 64)
		errCh := make(chan error, 1)

		go func() {
			errCh <- engine.Stream(ctx, question, tokenCh)
		}()

		for token := range tokenCh {
			fmt.Print(token)
		}
		fmt.Println()

		if err := <-errCh; err != nil {
			return fmt.Errorf("observer: %w", err)
		}
		return nil
	},
}

var observeDigestCmd = &cobra.Command{
	Use:   "digest",
	Short: "Generate a daily activity digest",
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
		defer cancel()

		engine, _, err := observer.QueryOnly(ctx)
		if err != nil {
			fmt.Fprintf(os.Stderr, "observer not available: %v\n", err)
			os.Exit(1)
		}

		summary, err := engine.GenerateDigest(ctx)
		if err != nil {
			return fmt.Errorf("digest: %w", err)
		}

		fmt.Println(summary)
		return nil
	},
}

var observeStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show observer status and index counts",
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		es, err := observer.Ping(ctx)
		if err != nil {
			fmt.Println("observer: offline")
			fmt.Fprintf(os.Stderr, "  reason: %v\n", err)
			fmt.Println("  fix: docker compose up -d")
			return nil
		}

		fmt.Println("observer: online")

		indices := []string{"glitch-events", "glitch-summaries", "glitch-pipelines", "glitch-insights"}
		for _, idx := range indices {
			result, err := es.Search(ctx, []string{idx}, map[string]any{
				"size":             0,
				"track_total_hits": true,
				"query":            map[string]any{"match_all": map[string]any{}},
			})
			if err != nil {
				fmt.Printf("  %s: error\n", idx)
				continue
			}
			fmt.Printf("  %s: %d docs\n", idx, result.Total)
		}

		return nil
	},
}

