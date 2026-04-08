// research.go implements `glitch research smoke` — a runnable end-to-end
// validation of the research loop primitive defined in internal/research.
//
// This is intentionally a debug surface, not a user-facing command. It exists
// so that when the loop's prompts or stages change, an operator can run one
// command and see whether the loop still produces grounded answers against
// the canonical "github-prs" researcher. The command makes no decisions on
// the user's behalf and writes no files.
//
// The smoke target is the failure mode captured in the project memory
// `project_research_loop_negative_example.md`: "verify the status of recent
// PRs in the current repo." Before this loop existed, glitch would
// hallucinate PR numbers from training-set patterns. After this loop is
// wired, the planner picks the github-prs researcher, the workflow runs gh
// pr list, the drafter writes an answer grounded in the actual PR list, and
// no fabricated identifiers appear.
package cmd

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/8op-org/gl1tch/internal/research"
)

var (
	researchSmokeQuestion string
	researchSmokeWorkflow string
	researchSmokeModel    string
	researchSmokeVerbose  bool
)

var researchCmd = &cobra.Command{
	Use:   "research",
	Short: "Inspect and exercise the research loop primitive",
	Long: `The research loop is gl1tch's bounded iterative researcher. It picks
researchers via the local model, gathers evidence, drafts a grounded answer,
and refuses to invent identifiers that are not in the gathered bundle.

The subcommands here are debug surfaces for operators iterating on the loop
itself. End users get the loop transparently through the assistant.`,
}

var researchSmokeCmd = &cobra.Command{
	Use:   "smoke",
	Short: "Run an end-to-end research loop call against the github-prs workflow",
	Long: `Wires the github-prs PipelineResearcher into a fresh research loop,
runs the configured question through plan → gather → draft, and prints the
draft along with the per-iteration evidence summary.

This is the smoke test for the loop architecture. If gh CLI is authenticated
and Ollama is running with qwen2.5:7b, the output should be a grounded
summary of the actual open PRs in the current repository — no hallucinated
PR numbers, no "you should run gh pr list" deflections.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()

		// Build the executor manager once and reuse it for the workflow
		// runner and the local LLM seam. Both go through the same Ollama
		// path.
		mgr := buildFullManager()

		// Build the canonical default registry (github-prs, github-issues,
		// git-log, git-status). Workflows that don't exist on this
		// workspace are silently skipped — see DefaultRegistry. The
		// --workflow flag is honoured by appending an override entry
		// after the defaults so an operator can swap in an experimental
		// pipeline without touching the canonical specs.
		registry, err := research.DefaultRegistry(mgr, "")
		if err != nil {
			return fmt.Errorf("default registry: %w", err)
		}
		if researchSmokeWorkflow != "" && researchSmokeWorkflow != "github-prs" {
			ghr := research.NewPipelineResearcher(
				"github-prs",
				"Lists the currently open pull requests in the current git "+
					"repository, with PR numbers, titles, authors, states, "+
					"draft status, and last-updated timestamps.",
				researchSmokeWorkflow,
				mgr,
			)
			// Best effort: a duplicate-name conflict here means the
			// default registry already provided github-prs and the
			// caller's --workflow override is the same name; favour
			// the override by recreating the registry without it.
			if err := registry.Register(ghr); err != nil {
				return fmt.Errorf("register override github-prs researcher: %w", err)
			}
		}

		llm := research.NewOllamaLLM(mgr, researchSmokeModel)
		loop := research.NewLoop(registry, llm)
		if researchSmokeVerbose {
			loop = loop.WithLogger(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
				Level: slog.LevelDebug,
			})))
		}

		fmt.Fprintln(cmd.OutOrStdout(), "── research loop smoke ──────────────────────────────────────")
		fmt.Fprintf(cmd.OutOrStdout(), "question: %s\n", researchSmokeQuestion)
		fmt.Fprintf(cmd.OutOrStdout(), "model:    %s\n", researchSmokeModel)
		fmt.Fprintf(cmd.OutOrStdout(), "registry: %v\n\n", registry.Names())

		result, err := loop.Run(ctx, research.ResearchQuery{
			Question: researchSmokeQuestion,
		}, research.DefaultBudget())
		if err != nil {
			return fmt.Errorf("loop.Run: %w", err)
		}

		fmt.Fprintln(cmd.OutOrStdout(), "── evidence ─────────────────────────────────────────────────")
		if result.Bundle.Len() == 0 {
			fmt.Fprintln(cmd.OutOrStdout(), "(no evidence gathered)")
		} else {
			for i, ev := range result.Bundle.Items {
				fmt.Fprintf(cmd.OutOrStdout(), "[%d] %s — %s\n", i+1, ev.Source, ev.Title)
				if len(ev.Refs) > 0 {
					fmt.Fprintf(cmd.OutOrStdout(), "    refs: %s\n", strings.Join(ev.Refs, ", "))
				}
				if researchSmokeVerbose {
					fmt.Fprintf(cmd.OutOrStdout(), "    body:\n      %s\n",
						strings.ReplaceAll(strings.TrimSpace(ev.Body), "\n", "\n      "))
				}
			}
		}

		fmt.Fprintln(cmd.OutOrStdout(), "\n── draft ────────────────────────────────────────────────────")
		fmt.Fprintln(cmd.OutOrStdout(), strings.TrimSpace(result.Draft))
		fmt.Fprintln(cmd.OutOrStdout(), "\n── meta ─────────────────────────────────────────────────────")
		fmt.Fprintf(cmd.OutOrStdout(), "reason:     %s\n", result.Reason)
		fmt.Fprintf(cmd.OutOrStdout(), "iterations: %d\n", result.Iterations)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(researchCmd)
	researchCmd.AddCommand(researchSmokeCmd)
	researchSmokeCmd.Flags().StringVarP(&researchSmokeQuestion, "question", "q",
		"there have been recent updates to the pr's, can you verify their statuses?",
		"the question to run through the research loop")
	researchSmokeCmd.Flags().StringVarP(&researchSmokeWorkflow, "workflow", "w",
		"github-prs",
		"workflow name or path to use as the github-prs researcher")
	researchSmokeCmd.Flags().StringVarP(&researchSmokeModel, "model", "m",
		research.DefaultLocalModel,
		"local Ollama model to use for plan and draft stages")
	researchSmokeCmd.Flags().BoolVarP(&researchSmokeVerbose, "verbose", "v", false,
		"print evidence bodies and per-stage debug logs to stderr")
}
