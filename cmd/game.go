package cmd

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/8op-org/gl1tch/internal/game"
	"github.com/8op-org/gl1tch/internal/store"
)

var recapDays int

func init() {
	rootCmd.AddCommand(gameCmd)
	gameCmd.AddCommand(gameTuneCmd)
	gameCmd.AddCommand(gameICECmd)
	gameCmd.AddCommand(gameRecapCmd)
	gameCmd.AddCommand(gameTopCmd)
	gameRecapCmd.Flags().IntVar(&recapDays, "days", 7, "Number of days to include in recap")
}

var gameCmd = &cobra.Command{
	Use:   "game",
	Short: "Game system commands",
}

var gameTuneCmd = &cobra.Command{
	Use:   "tune",
	Short: "Manually trigger the self-evolving game pack tuner",
	Long: `Calls local Ollama to analyze your usage patterns and generate an evolved
game pack. Writes the result to ~/.local/share/glitch/agents/game-world-tuned.agent.md.
The TunedWorldPackLoader picks it up automatically on the next run.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		st, err := store.Open()
		if err != nil {
			return fmt.Errorf("game tune: open store: %w", err)
		}
		defer st.Close()

		engine := game.NewGameEngine()
		loader := game.TunedWorldPackLoader{}
		tuner := game.NewTuner(st, engine, loader)

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()

		// Gather stats.
		stats, err := st.GameStatsQuery(ctx, 30)
		if err != nil {
			return fmt.Errorf("game tune: query stats: %w", err)
		}

		// Read the current pack so we can diff what changed.
		currentPack := loader.ActivePack()

		// Dummy payload for the manual tune — we don't have a live run.
		payload := game.GameRunScoredPayload{}

		fmt.Fprintln(os.Stdout, "Running game tuner...")
		fmt.Fprintf(os.Stdout, "  Stats window: last 30 days, %d runs\n", stats.TotalRuns)
		fmt.Fprintf(os.Stdout, "  Current pack: %s\n", currentPack.Name)

		start := time.Now()
		if err := tuner.Tune(ctx, stats, payload); err != nil {
			fmt.Fprintf(os.Stderr, "Tuner failed: %v\n", err)
			os.Exit(1)
		}
		elapsed := time.Since(start).Round(time.Second)

		// Read the new pack and print a summary of what changed.
		newPack := game.TunedWorldPackLoader{}.ActivePack()
		printTuneSummary(currentPack, newPack, elapsed)

		return nil
	},
}

// printTuneSummary prints a human-readable diff of what the tuner changed.
func printTuneSummary(old, new_ game.GameWorldPack, elapsed time.Duration) {
	fmt.Printf("\nGame pack evolved in %s\n", elapsed)
	fmt.Printf("  Old pack: %s\n", old.Name)
	fmt.Printf("  New pack: %s\n", new_.Name)

	// Weight deltas.
	oldW := old.Weights
	newW := new_.Weights
	fmt.Println("\nWeight changes:")
	printWeightDelta("base_multiplier", oldW.BaseMultiplier, newW.BaseMultiplier)
	printWeightDelta("cache_bonus_rate", oldW.CacheBonusRate, newW.CacheBonusRate)
	printWeightDelta("speed_bonus_scale", oldW.SpeedBonusScale, newW.SpeedBonusScale)
	printWeightDelta("retry_penalty", float64(oldW.RetryPenalty), float64(newW.RetryPenalty))
	printWeightDelta("streak_multiplier", oldW.StreakMultiplier, newW.StreakMultiplier)

	// Provider weight changes.
	allProviders := map[string]struct{}{}
	for k := range oldW.ProviderWeights {
		allProviders[k] = struct{}{}
	}
	for k := range newW.ProviderWeights {
		allProviders[k] = struct{}{}
	}
	for p := range allProviders {
		oldV := oldW.ProviderWeights[p]
		if oldV == 0 {
			oldV = 1.0
		}
		newV := newW.ProviderWeights[p]
		if newV == 0 {
			newV = 1.0
		}
		printWeightDelta("provider_weights."+p, oldV, newV)
	}

	// Quick game_rules line count diff.
	oldLines := countLines(old.GameRules)
	newLines := countLines(new_.GameRules)
	if newLines != oldLines {
		fmt.Printf("\nGame rules: %d → %d lines (%+d)\n", oldLines, newLines, newLines-oldLines)
	} else {
		fmt.Printf("\nGame rules: %d lines (unchanged length)\n", newLines)
	}

	fmt.Println("\nPack written to ~/.local/share/glitch/agents/game-world-tuned.agent.md")
}

func printWeightDelta(name string, old, new_ float64) {
	if old == new_ {
		return
	}
	diff := new_ - old
	sign := "+"
	if diff < 0 {
		sign = ""
	}
	fmt.Printf("  %s: %.4f → %.4f (%s%.4f)\n", name, old, new_, sign, diff)
}

func countLines(s string) int {
	count := 0
	for _, c := range s {
		if c == '\n' {
			count++
		}
	}
	return count
}

// tuneStatsJSON returns a compact JSON representation of GameStats for display.
func tuneStatsJSON(gs store.GameStats) string {
	b, _ := json.Marshal(gs)
	return string(b)
}

// suppress unused warning
var _ = tuneStatsJSON

// ── glitch game ice ───────────────────────────────────────────────────────────

var gameICECmd = &cobra.Command{
	Use:   "ice",
	Short: "Resolve a pending ICE encounter",
	Long:  `Displays the active ICE encounter (if any) and lets you fight or jack out. Loss decrements your streak by 1.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		st, err := store.Open()
		if err != nil {
			return fmt.Errorf("game ice: open store: %w", err)
		}
		defer st.Close()

		enc, err := st.GetPendingICEEncounter()
		if err != nil {
			return fmt.Errorf("game ice: query encounter: %w", err)
		}
		if enc == nil {
			fmt.Fprintln(os.Stdout, "No active ICE encounter.")
			return nil
		}

		fmt.Fprintf(os.Stdout, "\n\x1b[91m[ICE DETECTED]\x1b[0m %s\n", enc.ICEClass)
		fmt.Fprintf(os.Stdout, "Deadline: %s\n\n", enc.Deadline.Format(time.RFC3339))
		fmt.Fprintln(os.Stdout, "  [1] fight  — attempt to defeat the ICE")
		fmt.Fprintln(os.Stdout, "  [2] jack-out — disconnect safely (loss)")
		fmt.Fprint(os.Stdout, "\nChoice: ")

		scanner := bufio.NewScanner(os.Stdin)
		scanner.Scan()
		choice := strings.TrimSpace(scanner.Text())

		var outcome string
		switch choice {
		case "1", "fight":
			outcome = "win"
			fmt.Fprintln(os.Stdout, "\n\x1b[92mICE defeated. Streak intact.\x1b[0m")
		default:
			outcome = "loss"
			fmt.Fprintln(os.Stdout, "\n\x1b[91mJacked out. Streak decremented.\x1b[0m")
			applyStreakPenalty(st)
		}

		if err := st.ResolveICEEncounter(enc.ID, outcome); err != nil {
			return fmt.Errorf("game ice: resolve: %w", err)
		}
		return nil
	},
}

// applyStreakPenalty decrements the player's streak by 1, bounded at 0.
func applyStreakPenalty(st *store.Store) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	us, err := st.GetUserScore(ctx)
	if err != nil {
		return
	}
	if us.StreakDays > 0 {
		us.StreakDays--
	}
	_ = st.UpdateUserScore(ctx, us)
}

// ── glitch game recap ─────────────────────────────────────────────────────────

var gameRecapCmd = &cobra.Command{
	Use:   "recap",
	Short: "Narrate your last N days as a cyberpunk story arc",
	RunE: func(cmd *cobra.Command, args []string) error {
		st, err := store.Open()
		if err != nil {
			return fmt.Errorf("game recap: open store: %w", err)
		}
		defer st.Close()

		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()

		stats, err := st.GameStatsQuery(ctx, recapDays)
		if err != nil {
			return fmt.Errorf("game recap: query stats: %w", err)
		}
		if stats.TotalRuns == 0 {
			fmt.Fprintf(os.Stdout, "No runs recorded in the last %d days.\n", recapDays)
			return nil
		}

		loader := game.TunedWorldPackLoader{}
		pack := loader.ActivePack()
		engine := game.NewGameEngine()

		us, _ := st.GetUserScore(ctx)
		summary := map[string]any{
			"days":            recapDays,
			"total_runs":      stats.TotalRuns,
			"avg_output_ratio": stats.AvgOutputRatio,
			"avg_cost_usd":    stats.AvgCostUSD,
			"step_failure_rate": stats.StepFailureRate,
			"achievements":    stats.UnlockedAchievementIDs,
			"total_xp":        us.TotalXP,
			"level":           us.Level,
			"streak_days":     us.StreakDays,
		}
		summaryJSON, _ := json.MarshalIndent(summary, "", "  ")

		prompt := fmt.Sprintf(`%s

The player's last %d days in The Gibson:
%s

Write a 6-10 line cyberpunk story arc narrating what this player's journey looked like. Reference their stats, achievements, and arc. Plain text only.`,
			pack.NarratorStyle, recapDays, string(summaryJSON))

		narration := engine.Respond(ctx, prompt, "Narrate the player's arc.")
		if narration == "" {
			// Fallback: plain stats table.
			fmt.Printf("Last %d days — %d runs | Total XP: %d | Level: %d | Streak: %d days\n",
				recapDays, stats.TotalRuns, us.TotalXP, us.Level, us.StreakDays)
			if len(stats.UnlockedAchievementIDs) > 0 {
				fmt.Printf("Achievements: %s\n", strings.Join(stats.UnlockedAchievementIDs, ", "))
			}
			return nil
		}
		fmt.Fprintln(os.Stdout, narration)
		return nil
	},
}

// ── glitch game top ───────────────────────────────────────────────────────────

var gameTopCmd = &cobra.Command{
	Use:   "top",
	Short: "Show your personal best records",
	RunE: func(cmd *cobra.Command, args []string) error {
		st, err := store.Open()
		if err != nil {
			return fmt.Errorf("game top: open store: %w", err)
		}
		defer st.Close()

		bests, err := st.GetPersonalBests()
		if err != nil {
			return fmt.Errorf("game top: query: %w", err)
		}
		if len(bests) == 0 {
			fmt.Fprintln(os.Stdout, "No personal bests recorded yet.")
			return nil
		}

		fmt.Println("\n\x1b[95m── Personal Bests ──────────────────────────────\x1b[0m")
		fmt.Printf("  %-24s  %-14s  %s\n", "METRIC", "VALUE", "DATE")
		fmt.Println("  " + strings.Repeat("─", 50))
		for _, pb := range bests {
			label, val := formatPersonalBest(pb)
			fmt.Printf("  \x1b[96m%-24s\x1b[0m  %-14s  %s\n",
				label, val, pb.RecordedAt.Format("2006-01-02"))
		}
		fmt.Println()
		return nil
	},
}

// formatPersonalBest returns a human label and formatted value for a personal best.
func formatPersonalBest(pb store.PersonalBest) (label, value string) {
	switch pb.Metric {
	case "fastest_run_ms":
		return "Fastest Run", fmt.Sprintf("%.0fms", pb.Value)
	case "highest_xp":
		return "Highest XP (single run)", fmt.Sprintf("%.0f XP", pb.Value)
	case "longest_streak":
		return "Longest Streak", fmt.Sprintf("%.0f days", pb.Value)
	case "most_cache_tokens":
		return "Most Cache Tokens", fmt.Sprintf("%.0f", pb.Value)
	case "lowest_cost_usd":
		return "Lowest Cost (non-zero)", fmt.Sprintf("$%.6f", pb.Value)
	default:
		return pb.Metric, fmt.Sprintf("%.4f", pb.Value)
	}
}
