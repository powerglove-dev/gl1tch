package cmd

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"time"

	"github.com/spf13/cobra"

	"github.com/8op-org/gl1tch/internal/capability"
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

// printAnalysisStatus reports the attention + deep-analysis
// subsystem configuration so `glitch observe status` gives a
// one-command answer to "why isn't gl1tch drafting anything for
// me?". Looks at four things per workspace:
//
//   - Analysis.Enabled (from collectors.yaml) — the gate on the
//     heavy opencode path
//   - Analysis.Model — which coder model runOne will hand to
//     opencode; printed so a user can spot a typo or a model
//     they never pulled
//   - research.md presence — per-workspace file / global fallback
//     / bundled default; the classifier's effective rule source
//   - opencode binary availability and qwen2.5:7b presence — the
//     two hard dependencies without which the stack can't run
//
// The output is human-readable, not machine-parseable. This is a
// diagnostic command, not a control plane; `glitch attention`
// is where automation belongs.
func printAnalysisStatus() {
	fmt.Println("attention + analysis:")

	// opencode availability — single line because it's the same
	// for every workspace. The version check is best-effort so a
	// broken opencode build still reports "installed" with a warn.
	if path, err := exec.LookPath("opencode"); err == nil {
		version := "(version check failed)"
		if out, err := exec.Command("opencode", "--version").Output(); err == nil {
			version = "v" + string(out)
		}
		fmt.Printf("  opencode: %s %s", path, version)
	} else {
		fmt.Println("  opencode: MISSING — install with `brew install sst/tap/opencode`")
	}

	// Global research prompt fallback. The per-workspace files
	// shadow it; we report whether the global exists so users who
	// want a single shared rule file know where to put it.
	home, _ := os.UserHomeDir()
	globalResearch := filepath.Join(home, ".config", "glitch", "research.md")
	if _, err := os.Stat(globalResearch); err == nil {
		fmt.Printf("  global research prompt: %s\n", globalResearch)
	} else {
		fmt.Println("  global research prompt: (none — using bundled default for workspaces without their own)")
	}

	// Per-workspace analysis config. Walk ~/.config/glitch/workspaces
	// directly rather than going through capability.LoadWorkspaceConfig
	// in a loop so we can print an orderable list — the directory
	// entries have stable sorted ids which is good enough for this
	// diagnostic's output.
	wsRoot := filepath.Join(home, ".config", "glitch", "workspaces")
	entries, err := os.ReadDir(wsRoot)
	if err != nil {
		fmt.Printf("  workspaces: (%v)\n", err)
		return
	}
	ids := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() {
			ids = append(ids, e.Name())
		}
	}
	sort.Strings(ids)
	if len(ids) == 0 {
		fmt.Println("  workspaces: (none configured)")
		return
	}

	for _, id := range ids {
		cfg, err := capability.LoadWorkspaceConfig(id)
		if err != nil {
			fmt.Printf("  workspace %s: load error: %v\n", shortID(id), err)
			continue
		}
		enabled := "OFF"
		if cfg != nil && cfg.Analysis.Enabled {
			enabled = "on"
		}
		model := "(default)"
		if cfg != nil && cfg.Analysis.Model != "" {
			model = cfg.Analysis.Model
		}
		researchSource := resolveResearchSource(id)
		fmt.Printf("  workspace %s: analysis=%s model=%s research=%s\n",
			shortID(id), enabled, model, researchSource)
	}
}

// resolveResearchSource reports where the classifier will get
// this workspace's research prompt from: per-workspace file,
// global fallback, or bundled default. Does NOT load the prompt
// itself — the status command is read-only and we don't need the
// contents, only the source.
func resolveResearchSource(workspaceID string) string {
	home, err := os.UserHomeDir()
	if err != nil {
		return "unknown"
	}
	ws := filepath.Join(home, ".config", "glitch", "workspaces", workspaceID, "research.md")
	if _, err := os.Stat(ws); err == nil {
		return "workspace"
	}
	global := filepath.Join(home, ".config", "glitch", "research.md")
	if _, err := os.Stat(global); err == nil {
		return "global"
	}
	return "bundled-default"
}

// shortID truncates a UUID-shaped workspace id to its first 8
// characters for table readability. Collisions are possible but
// vanishingly unlikely in a human's 3-5 workspace install, and
// any real collision would just show up as two rows with the
// same short id, which is still more useful than dumping full
// UUIDs across 80+ columns.
func shortID(id string) string {
	if len(id) > 8 {
		return id[:8]
	}
	return id
}

var observeStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show observer + analysis + attention status",
	Long: `Report the current state of every moving part the attention +
deep-analysis stack depends on: Elasticsearch, per-index doc counts,
the analysis config for each workspace, opencode availability, and
the resolution source for each workspace's research prompt.

The typical use is diagnosing "I'm not seeing any attention
announcements" — one command tells you whether analysis is even
enabled, whether opencode is installed, whether a research prompt
exists beyond the bundled default, and whether any analyses have
actually been produced yet.`,
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

		indices := []string{"glitch-events", "glitch-analyses", "glitch-summaries", "glitch-pipelines", "glitch-insights"}
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

		// Attention + analysis subsystem diagnostics. Kept out of
		// the index loop above because the inputs are
		// config-file-based, not ES-based, and they tell a very
		// different debugging story: "is gl1tch *allowed* to
		// produce analyses?" rather than "has it produced any?".
		fmt.Println()
		printAnalysisStatus()

		return nil
	},
}

