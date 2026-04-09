// ask_research.go wires the research loop primitive (internal/research) into
// `glitch ask` as the new fallback path for natural-language questions that do
// not match a saved workflow.
//
// The motivating failure mode is captured in the project memory
// `project_research_loop_negative_example.md`: before this wiring, an ask that
// could not be routed to a workflow either generated a brand-new pipeline on
// the fly (often unhelpful) or fell through to a one-shot model call (which
// produced confident hallucinations grounded in nothing). With the loop in
// place, the same fallback now picks researchers from the canonical default
// registry, gathers evidence through them, and writes a draft that is
// constrained — at the prompt level — from inventing identifiers.
//
// This file owns the cmd-package-side glue. The loop itself, the registry,
// and the prompt rules live in internal/research; this file deliberately
// stays a thin renderer so that future tweaks to the loop's stages or
// scoring do not require touching cmd/.
package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/8op-org/gl1tch/internal/executor"
	"github.com/8op-org/gl1tch/internal/research"
)

// askResearchEnabled is the cmd-side toggle for the research-loop fallback.
// Default is true: the daily-driver path runs the loop whenever routing
// produces no match. Power users who want the legacy behaviour
// (dispatchGenerated → runOneShot) can pass --no-research.
var askResearchEnabled = true

// Research loop tuning flags. These mirror the openspec proposal's
// task-9 list one-for-one so a power user can shape the loop without
// touching code or env vars. Defaults are intentionally conservative —
// MaxPaidTokens=0 means escalation is off until you opt in by passing
// --escalate=claude.
var (
	askResearchEscalate     string
	askResearchThreshold    float64
	askResearchMaxIters     int
	askResearchMaxWallclock time.Duration
	askResearchMaxLocalTok  int
	askResearchMaxPaidTok   int
)

func init() {
	askCmd.Flags().BoolVar(&askResearchEnabled, "research", true,
		"use the research loop as the fallback when no workflow matches the prompt")
	askCmd.Flags().StringVar(&askResearchEscalate, "escalate", "off",
		"escalate to a paid verifier when local confidence is below --threshold (off|claude)")
	askCmd.Flags().Float64Var(&askResearchThreshold, "threshold", 0.7,
		"composite confidence threshold the loop must clear to accept a draft")
	askCmd.Flags().IntVar(&askResearchMaxIters, "max-iterations", 5,
		"maximum plan→score cycles before the loop returns its best draft")
	askCmd.Flags().DurationVar(&askResearchMaxWallclock, "max-wallclock", 60*time.Second,
		"maximum wallclock time per research call")
	askCmd.Flags().IntVar(&askResearchMaxLocalTok, "max-local-tokens", 50_000,
		"maximum local-model tokens per research call (advisory until LLMFn carries usage)")
	askCmd.Flags().IntVar(&askResearchMaxPaidTok, "max-paid-tokens", 0,
		"maximum paid-model tokens; setting >0 enables escalation when --escalate is also set")
}

// runAskResearch builds a research loop using the canonical default registry,
// runs it against the user's prompt, and renders the resulting evidence
// bundle and grounded draft to the command's stdout.
//
// The function falls back to runOneShot when the registry is empty (no
// workflow files installed). This preserves the ability to use `glitch ask`
// on a fresh checkout that has not yet adopted any researcher workflows —
// the loop's empty-bundle path would otherwise return "I don't have enough
// evidence" for every question, which is a worse default than the legacy
// one-shot path.
func runAskResearch(
	cmd *cobra.Command,
	prompt, providerID, model string,
	mgr *executor.Manager,
	inputVars map[string]string,
) error {
	registry, err := research.DefaultRegistry(mgr, "")
	if err != nil {
		fmt.Fprintf(cmd.ErrOrStderr(), "[research] warn: building default registry: %v\n", err)
	}

	if registry == nil || len(registry.Names()) == 0 {
		// No researchers available; fall through to legacy one-shot so a
		// fresh checkout still answers something.
		fmt.Fprintln(cmd.ErrOrStderr(), "[research] no researchers registered — falling back to one-shot")
		return runOneShot(cmd, prompt, providerID, model, mgr, inputVars)
	}

	llm := research.NewOllamaLLM(mgr, model)

	scoreOpts := research.DefaultScoreOptions()
	scoreOpts.Threshold = askResearchThreshold
	// Self-consistency is the most expensive signal; gating it on the
	// presence of a paid escalation makes sense intuitively (if you're
	// willing to pay for a verifier you probably want every signal),
	// but for the local-only default the assistant call needs to stay
	// fast — keep it off unless the operator explicitly opts in.
	scoreOpts.SkipSelfConsistency = askResearchEscalate == "off"

	loop := research.NewLoop(registry, llm).
		WithScoreOptions(scoreOpts).
		WithEventSink(research.NewFileEventSink("")).
		WithHintsProvider(research.NewFileEventHintsProvider(""))

	ctx, cancel := context.WithTimeout(cmd.Context(), askResearchMaxWallclock+30*time.Second)
	defer cancel()

	fmt.Fprintf(cmd.ErrOrStderr(), "[research] available researchers: %s\n",
		strings.Join(registry.Names(), ", "))

	budget := research.Budget{
		MaxIterations:  askResearchMaxIters,
		MaxWallclock:   askResearchMaxWallclock,
		MaxLocalTokens: askResearchMaxLocalTok,
		MaxPaidTokens:  askResearchMaxPaidTok,
	}

	// Inject the shell's working directory as q.Context["cwd"] so the
	// canonical pipeline researchers (git-log, git-status, github-prs,
	// github-issues) execute against whatever repo the user invoked
	// `glitch ask` inside. This is the same lever the desktop uses to
	// scope a thread to its workspace cwd — keeping the two in sync
	// means a smoke test from the CLI exercises exactly the code path
	// the desktop hits.
	queryContext := make(map[string]string, len(inputVars)+1)
	for k, v := range inputVars {
		queryContext[k] = v
	}
	if _, hasCwd := queryContext["cwd"]; !hasCwd {
		if cwd, err := os.Getwd(); err == nil && cwd != "" {
			queryContext["cwd"] = cwd
		}
	}

	result, err := loop.Run(ctx, research.ResearchQuery{
		Question: prompt,
		Context:  queryContext,
	}, budget)
	if err != nil {
		return fmt.Errorf("research loop: %w", err)
	}

	if askJSON {
		// JSON envelope: callers parsing this stream get the full Result
		// rather than just the draft, so they can drive their own UI off
		// the per-source evidence and the (currently always-empty) score
		// breakdown that scoring will populate in #4.
		return printResearchJSON(result, providerID, model)
	}

	renderResearchResult(cmd.OutOrStdout(), result)
	return nil
}

// renderResearchResult prints a markdown-friendly rendering of one research
// Result. The format is intentionally plain — no ANSI, no terminal-specific
// box drawing — so it composes cleanly with desktop chat panes, scroll
// buffers, and `glitch ask | tee` style usage.
//
// Sections, in order:
//   - the draft (the actual answer the user asked for)
//   - "evidence" — one bullet per Evidence item, source first, then refs
//   - "meta" — reason and iteration count
func renderResearchResult(w io.Writer, result research.Result) {
	draft := strings.TrimSpace(result.Draft)
	if draft == "" {
		draft = "(no draft was produced)"
	}
	fmt.Fprintln(w, draft)

	if result.Bundle.Len() > 0 {
		fmt.Fprintln(w, "\n---")
		fmt.Fprintln(w, "evidence:")
		for i, ev := range result.Bundle.Items {
			title := ev.Title
			if title == "" {
				title = ev.Source
			}
			fmt.Fprintf(w, "  [%d] %s — %s\n", i+1, ev.Source, title)
			if len(ev.Refs) > 0 {
				fmt.Fprintf(w, "      refs: %s\n", strings.Join(ev.Refs, ", "))
			}
		}
	}

	// Per-signal scores. Missing signals are rendered as "—" instead of
	// 0 so an operator scanning the output can tell "the loop did not
	// compute this" from "the loop computed it and got zero." Composite
	// is always shown.
	fmt.Fprintln(w, "\nconfidence:")
	fmt.Fprintf(w, "  composite:           %.2f\n", result.Score.Composite)
	fmt.Fprintf(w, "  self_consistency:    %s\n", fmtScorePtr(result.Score.SelfConsistency))
	fmt.Fprintf(w, "  evidence_coverage:   %s\n", fmtScorePtr(result.Score.EvidenceCoverage))
	fmt.Fprintf(w, "  cross_capability:    %s\n", fmtScorePtr(result.Score.CrossCapabilityAgree))
	fmt.Fprintf(w, "  judge:               %s\n", fmtScorePtr(result.Score.JudgeScore))

	fmt.Fprintf(w, "\nreason: %s  iterations: %d\n", result.Reason, result.Iterations)
}

func fmtScorePtr(p *float64) string {
	if p == nil {
		return "—"
	}
	return fmt.Sprintf("%.2f", *p)
}

// printResearchJSON serialises a Result for the --json envelope. The shape
// matches the existing `printJSON` envelope's spirit (top-level provider /
// model fields) but adds the bundle and reason so JSON consumers see the
// same data the human renderer does.
func printResearchJSON(result research.Result, providerID, model string) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(map[string]any{
		"response":   result.Draft,
		"provider":   providerID,
		"model":      model,
		"bundle":     result.Bundle,
		"reason":     string(result.Reason),
		"iterations": result.Iterations,
		"score":      result.Score,
	})
}
