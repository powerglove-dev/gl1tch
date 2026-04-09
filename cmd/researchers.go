// researchers.go is the CLI surface for the externalized canonical
// researcher menu. It exists for the same reason `glitch prompts`
// exists: the Describe text the planner reads is the most-tuned
// surface in the loop, and forcing a recompile to change a single
// description string makes iterative tuning impossible.
//
// Sub-commands:
//
//   glitch researchers list           — every researcher + source
//                                       (embedded / user override)
//   glitch researchers show           — print the resolved YAML body
//   glitch researchers edit           — seed the user override file
//                                       from the embedded default,
//                                       open in $EDITOR
//   glitch researchers reset          — drop the user override
//   glitch researchers diff           — show user override vs
//                                       embedded default
//   glitch researchers path           — print the override path
//                                       (where edit would write)
//
// Editing the override file takes effect on the very next research
// call — DefaultRegistry re-reads on every invocation. Same
// cycle-time as the prompts: vim, save, re-run.
package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/8op-org/gl1tch/internal/research"
)

var researchersCmd = &cobra.Command{
	Use:   "researchers",
	Short: "Inspect and edit the canonical researcher menu (planner sees this)",
	Long: `The research loop's planner picks researchers from a small
canonical menu — name, describe, workflow. The menu lives as a YAML
file: shipped embedded with the binary so a fresh install works,
overridable on disk so you can edit a description and have the
next research call pick it up without recompiling.

Sub-commands let you list which researchers exist, show the
resolved YAML, edit it in $EDITOR, diff your override against the
default, or reset to the default. The default planner template
reads each researcher's Describe field as one bullet in its menu;
tuning a description is the highest-leverage thing you can do to
bias future picks without touching Go.`,
}

func init() {
	rootCmd.AddCommand(researchersCmd)
	researchersCmd.AddCommand(researchersListCmd)
	researchersCmd.AddCommand(researchersShowCmd)
	researchersCmd.AddCommand(researchersEditCmd)
	researchersCmd.AddCommand(researchersResetCmd)
	researchersCmd.AddCommand(researchersDiffCmd)
	researchersCmd.AddCommand(researchersPathCmd)
}

// ── glitch researchers list ─────────────────────────────────────────────────

var researchersListCmd = &cobra.Command{
	Use:   "list",
	Short: "List every canonical researcher and which source it resolves to",
	RunE: func(cmd *cobra.Command, args []string) error {
		specs, err := research.LoadDefaultPipelineSpecs()
		if err != nil {
			return err
		}
		path := research.DiskOverridePath()
		source := "embedded"
		if path != "" {
			if _, statErr := os.Stat(path); statErr == nil {
				source = "user (" + path + ")"
			}
		}
		fmt.Printf("source: %s\n", source)
		fmt.Println()
		fmt.Printf("%-18s  %-18s  %s\n", "NAME", "WORKFLOW", "DESCRIBE")
		fmt.Printf("%-18s  %-18s  %s\n", "----", "--------", "--------")
		for _, spec := range specs {
			fmt.Printf("%-18s  %-18s  %s\n", spec.Name, spec.Workflow, truncateForList(spec.Describe, 70))
		}
		return nil
	},
}

func truncateForList(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-1] + "…"
}

// ── glitch researchers show ─────────────────────────────────────────────────

var researchersShowCmd = &cobra.Command{
	Use:   "show",
	Short: "Print the resolved researchers YAML",
	RunE: func(cmd *cobra.Command, args []string) error {
		path := research.DiskOverridePath()
		if path != "" {
			if data, err := os.ReadFile(path); err == nil {
				fmt.Fprintf(cmd.ErrOrStderr(), "# source: %s\n", path)
				fmt.Print(string(data))
				return nil
			}
		}
		fmt.Fprintln(cmd.ErrOrStderr(), "# source: embedded")
		fmt.Print(string(research.EmbeddedResearchersDefault()))
		return nil
	},
}

// ── glitch researchers edit ─────────────────────────────────────────────────

var researchersEditCmd = &cobra.Command{
	Use:   "edit",
	Short: "Seed the user override from the embedded default and open $EDITOR",
	Long: `If no user override exists yet, copies the embedded default to
~/.config/glitch/researchers.yaml so $EDITOR opens with the
canonical YAML already in place — saving immediately yields the same
behaviour as the default until you tweak it.

$EDITOR honoured (defaults to 'vim'). The next research call after
the editor exits will pick up your changes — no recompile, no
restart, no other steps.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		path := research.DiskOverridePath()
		if path == "" {
			return fmt.Errorf("could not resolve user override path (HOME unset?)")
		}
		if _, err := os.Stat(path); err != nil {
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				return err
			}
			if err := os.WriteFile(path, research.EmbeddedResearchersDefault(), 0o644); err != nil {
				return err
			}
			fmt.Fprintf(cmd.ErrOrStderr(), "[researchers] seeded %s from embedded default\n", path)
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

// ── glitch researchers reset ────────────────────────────────────────────────

var researchersResetCmd = &cobra.Command{
	Use:   "reset",
	Short: "Delete the user override and revert to the embedded default",
	RunE: func(cmd *cobra.Command, args []string) error {
		path := research.DiskOverridePath()
		if path == "" {
			return fmt.Errorf("could not resolve user override path")
		}
		if err := os.Remove(path); err != nil {
			if os.IsNotExist(err) {
				fmt.Fprintf(cmd.ErrOrStderr(), "[researchers] no user override at %s — nothing to reset\n", path)
				return nil
			}
			return err
		}
		fmt.Fprintf(cmd.ErrOrStderr(), "[researchers] removed %s — next call uses embedded default\n", path)
		return nil
	},
}

// ── glitch researchers diff ─────────────────────────────────────────────────

var researchersDiffCmd = &cobra.Command{
	Use:   "diff",
	Short: "Show how the user override differs from the embedded default",
	RunE: func(cmd *cobra.Command, args []string) error {
		path := research.DiskOverridePath()
		if path == "" {
			return fmt.Errorf("could not resolve user override path")
		}
		userBody, err := os.ReadFile(path)
		if err != nil {
			fmt.Fprintln(cmd.ErrOrStderr(), "[researchers] no user override — nothing to diff")
			return nil
		}
		def := research.EmbeddedResearchersDefault()
		// Re-uses the unified-style diff helper from `glitch
		// prompts diff` so the two CLIs produce visually
		// consistent output.
		fmt.Println(simpleDiff(string(def), string(userBody)))
		return nil
	},
}

// ── glitch researchers path ─────────────────────────────────────────────────

var researchersPathCmd = &cobra.Command{
	Use:   "path",
	Short: "Print where the user override would live (existing or not)",
	RunE: func(cmd *cobra.Command, args []string) error {
		path := research.DiskOverridePath()
		if path == "" {
			return fmt.Errorf("could not resolve user override path")
		}
		fmt.Println(path)
		return nil
	},
}
