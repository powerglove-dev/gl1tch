// weights.go is the CLI surface for the externalized composite-score
// weights. The four scoring signals (cross_capability_agreement,
// evidence_coverage, judge_score, self_consistency) used to combine
// at hard-coded equal weights baked into Go. To favour any one
// signal you had to recompile.
//
// The weights now live as YAML — embedded for first-impression
// defaults, overridable on disk so the user can edit a value and
// have the next research call pick it up without recompiling. Same
// pattern as `glitch prompts` and `glitch researchers`.
//
// Sub-commands:
//
//   glitch weights list   — show current resolved weights + source
//   glitch weights show   — print the resolved YAML body
//   glitch weights edit   — seed override + open $EDITOR
//   glitch weights reset  — drop the user override
//   glitch weights diff   — show user override vs embedded default
//   glitch weights path   — print where the override would write
package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/8op-org/gl1tch/internal/research"
)

var weightsCmd = &cobra.Command{
	Use:   "weights",
	Short: "Inspect and edit the composite scoring weights",
	Long: `The research loop's confidence score is a weighted combination
of four per-signal scores. The weights live as a YAML file: shipped
embedded with the binary so a fresh install works, overridable on
disk so you can favour evidence_coverage over judge_score (or
disable a signal entirely) without recompiling.

Default weights are equal (0.25 each). Tune them by hand based on
what brain stats reports about which signals best predict accept,
or eventually let the brain stats engine write learned weights back
to the override file.`,
}

func init() {
	rootCmd.AddCommand(weightsCmd)
	weightsCmd.AddCommand(weightsListCmd)
	weightsCmd.AddCommand(weightsShowCmd)
	weightsCmd.AddCommand(weightsEditCmd)
	weightsCmd.AddCommand(weightsResetCmd)
	weightsCmd.AddCommand(weightsDiffCmd)
	weightsCmd.AddCommand(weightsPathCmd)
}

// ── glitch weights list ─────────────────────────────────────────────────────

var weightsListCmd = &cobra.Command{
	Use:   "list",
	Short: "Show current weights and which source resolved them",
	RunE: func(cmd *cobra.Command, args []string) error {
		w, err := research.LoadWeights()
		if err != nil {
			return err
		}
		path := research.WeightsOverridePath()
		source := "embedded"
		if path != "" {
			if _, err := os.Stat(path); err == nil {
				source = "user (" + path + ")"
			}
		}
		fmt.Printf("source: %s\n\n", source)
		fmt.Printf("%-32s  %s\n", "SIGNAL", "WEIGHT")
		fmt.Printf("%-32s  %s\n", "------", "------")
		fmt.Printf("%-32s  %.2f\n", "cross_capability_agreement", w.CrossCapabilityAgreement)
		fmt.Printf("%-32s  %.2f\n", "evidence_coverage", w.EvidenceCoverage)
		fmt.Printf("%-32s  %.2f\n", "judge_score", w.JudgeScore)
		fmt.Printf("%-32s  %.2f\n", "self_consistency", w.SelfConsistency)
		return nil
	},
}

// ── glitch weights show ─────────────────────────────────────────────────────

var weightsShowCmd = &cobra.Command{
	Use:   "show",
	Short: "Print the resolved weights YAML",
	RunE: func(cmd *cobra.Command, args []string) error {
		path := research.WeightsOverridePath()
		if path != "" {
			if data, err := os.ReadFile(path); err == nil {
				fmt.Fprintf(cmd.ErrOrStderr(), "# source: %s\n", path)
				fmt.Print(string(data))
				return nil
			}
		}
		fmt.Fprintln(cmd.ErrOrStderr(), "# source: embedded")
		fmt.Print(string(research.EmbeddedWeightsDefault()))
		return nil
	},
}

// ── glitch weights edit ─────────────────────────────────────────────────────

var weightsEditCmd = &cobra.Command{
	Use:   "edit",
	Short: "Seed the user override from the embedded default and open $EDITOR",
	RunE: func(cmd *cobra.Command, args []string) error {
		path := research.WeightsOverridePath()
		if path == "" {
			return fmt.Errorf("could not resolve user override path (HOME unset?)")
		}
		if _, err := os.Stat(path); err != nil {
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				return err
			}
			if err := os.WriteFile(path, research.EmbeddedWeightsDefault(), 0o644); err != nil {
				return err
			}
			fmt.Fprintf(cmd.ErrOrStderr(), "[weights] seeded %s from embedded default\n", path)
		}
		editor := os.Getenv("EDITOR")
		if editor == "" {
			editor = "vim"
		}
		ec := exec.Command(editor, path)
		ec.Stdin = os.Stdin
		ec.Stdout = os.Stdout
		ec.Stderr = os.Stderr
		return ec.Run()
	},
}

// ── glitch weights reset ────────────────────────────────────────────────────

var weightsResetCmd = &cobra.Command{
	Use:   "reset",
	Short: "Delete the user override and revert to the embedded default",
	RunE: func(cmd *cobra.Command, args []string) error {
		path := research.WeightsOverridePath()
		if path == "" {
			return fmt.Errorf("could not resolve user override path")
		}
		if err := os.Remove(path); err != nil {
			if os.IsNotExist(err) {
				fmt.Fprintf(cmd.ErrOrStderr(), "[weights] no user override at %s — nothing to reset\n", path)
				return nil
			}
			return err
		}
		fmt.Fprintf(cmd.ErrOrStderr(), "[weights] removed %s — next call uses embedded default\n", path)
		return nil
	},
}

// ── glitch weights diff ─────────────────────────────────────────────────────

var weightsDiffCmd = &cobra.Command{
	Use:   "diff",
	Short: "Show how the user override differs from the embedded default",
	RunE: func(cmd *cobra.Command, args []string) error {
		path := research.WeightsOverridePath()
		if path == "" {
			return fmt.Errorf("could not resolve user override path")
		}
		userBody, err := os.ReadFile(path)
		if err != nil {
			fmt.Fprintln(cmd.ErrOrStderr(), "[weights] no user override — nothing to diff")
			return nil
		}
		def := research.EmbeddedWeightsDefault()
		fmt.Println(simpleDiff(string(def), string(userBody)))
		return nil
	},
}

// ── glitch weights path ─────────────────────────────────────────────────────

var weightsPathCmd = &cobra.Command{
	Use:   "path",
	Short: "Print where the user override would live (existing or not)",
	RunE: func(cmd *cobra.Command, args []string) error {
		path := research.WeightsOverridePath()
		if path == "" {
			return fmt.Errorf("could not resolve user override path")
		}
		fmt.Println(path)
		return nil
	},
}
