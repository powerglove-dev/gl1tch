// prompts.go is the user-facing CLI for the externalized research-loop
// prompts. It exists because the planner / drafter / critique / judge /
// self-consistency / verify templates used to be hardcoded Go strings —
// every copy change required a recompile, which made iterative tuning
// (the kind a learning system NEEDS) painfully slow. The templates now
// live as .tmpl files: shipped embedded for the first-impression
// experience, overridable on disk so the user can `vim` and re-run.
//
// Sub-commands:
//
//   glitch prompts list           — every slot, which source it's
//                                   resolving to (embedded / user)
//   glitch prompts show <name>    — print the resolved template body
//   glitch prompts edit <name>    — seed the user override from the
//                                   embedded default and open $EDITOR
//   glitch prompts reset <name>   — delete the user override
//   glitch prompts diff <name>    — show user override vs embedded
//                                   default (only when an override
//                                   exists)
//   glitch prompts path <name>    — print the user override path
//                                   (where edit would write)
//
// Editing the override file takes effect on the very next research
// call — there is NO restart, NO recompile. That is the entire point.
package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/8op-org/gl1tch/internal/research"
)

var promptsCmd = &cobra.Command{
	Use:   "prompts",
	Short: "Inspect and edit the research-loop prompt templates",
	Long: `The research loop's planner / drafter / critique / judge /
self-consistency / verify prompts live as .tmpl files. The defaults
ship embedded with the binary so a fresh install works out of the box;
disk overrides at ~/.config/glitch/prompts/<name>.tmpl take effect on
the next research call without recompiling.

Sub-commands let you list which slots exist, show what the loop is
about to send to the model, edit a slot in $EDITOR, diff your
override against the default, or reset back to the default.`,
}

func init() {
	rootCmd.AddCommand(promptsCmd)
	promptsCmd.AddCommand(promptsListCmd)
	promptsCmd.AddCommand(promptsShowCmd)
	promptsCmd.AddCommand(promptsEditCmd)
	promptsCmd.AddCommand(promptsResetCmd)
	promptsCmd.AddCommand(promptsDiffCmd)
	promptsCmd.AddCommand(promptsPathCmd)
}

// promptsStore returns the same package-level loader the research
// loop is using, so `glitch prompts` shows exactly what the loop
// would render — not a parallel store with different lookup rules.
func promptsStore() *research.PromptStore {
	// We construct a fresh store rather than reaching into the
	// research package's package-level singleton because the CLI
	// is short-lived and the store is stateless w.r.t. caching.
	// Workspace overrides are not wired here yet — when they are,
	// the CLI will gain a -w flag like `glitch threads`.
	return research.NewPromptStore("")
}

// ── glitch prompts list ─────────────────────────────────────────────────────

var promptsListCmd = &cobra.Command{
	Use:   "list",
	Short: "List every prompt slot and which source it resolves to",
	RunE: func(cmd *cobra.Command, args []string) error {
		store := promptsStore()
		fmt.Printf("%-18s  %-10s  %s\n", "NAME", "SOURCE", "PATH")
		fmt.Printf("%-18s  %-10s  %s\n", "----", "------", "----")
		for _, name := range research.AllPromptNames {
			_, source, err := store.Resolve(name)
			if err != nil {
				fmt.Printf("%-18s  %-10s  %v\n", name, "ERROR", err)
				continue
			}
			path := ""
			if source == research.PromptSourceUser {
				path = store.UserOverridePath(name)
			} else {
				path = "(embedded)"
			}
			fmt.Printf("%-18s  %-10s  %s\n", name, source, path)
		}
		return nil
	},
}

// ── glitch prompts show ─────────────────────────────────────────────────────

var promptsShowCmd = &cobra.Command{
	Use:   "show <name>",
	Short: "Print the resolved template body for one slot",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := research.PromptName(args[0])
		body, source, err := promptsStore().Resolve(name)
		if err != nil {
			return err
		}
		fmt.Fprintf(cmd.ErrOrStderr(), "# source: %s\n", source)
		fmt.Print(body)
		return nil
	},
}

// ── glitch prompts edit ─────────────────────────────────────────────────────

var promptsEditCmd = &cobra.Command{
	Use:   "edit <name>",
	Short: "Seed the user override from the embedded default and open $EDITOR",
	Long: `If no user override exists yet, copies the embedded default to
~/.config/glitch/prompts/<name>.tmpl so $EDITOR opens with the
canonical text already in place — saving immediately yields the same
behaviour as the default until you tweak it.

$EDITOR honoured (defaults to 'vim'). The next research call after
the editor exits will pick up your changes — no recompile, no
restart, no other steps.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		store := promptsStore()
		name := research.PromptName(args[0])

		path := store.UserOverridePath(name)
		if path == "" {
			return fmt.Errorf("could not resolve user override path (HOME unset?)")
		}

		// Seed if missing.
		if _, err := os.Stat(path); err != nil {
			body, err := store.EmbeddedDefault(name)
			if err != nil {
				return err
			}
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				return err
			}
			if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
				return err
			}
			fmt.Fprintf(cmd.ErrOrStderr(), "[prompts] seeded %s from embedded default\n", path)
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

// ── glitch prompts reset ────────────────────────────────────────────────────

var promptsResetCmd = &cobra.Command{
	Use:   "reset <name>",
	Short: "Delete the user override and revert to the embedded default",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		store := promptsStore()
		name := research.PromptName(args[0])
		path := store.UserOverridePath(name)
		if path == "" {
			return fmt.Errorf("could not resolve user override path")
		}
		if err := os.Remove(path); err != nil {
			if os.IsNotExist(err) {
				fmt.Fprintf(cmd.ErrOrStderr(), "[prompts] no user override at %s — nothing to reset\n", path)
				return nil
			}
			return err
		}
		fmt.Fprintf(cmd.ErrOrStderr(), "[prompts] removed %s — next call uses embedded default\n", path)
		return nil
	},
}

// ── glitch prompts diff ─────────────────────────────────────────────────────

var promptsDiffCmd = &cobra.Command{
	Use:   "diff <name>",
	Short: "Show how the user override differs from the embedded default",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		store := promptsStore()
		name := research.PromptName(args[0])
		if !store.HasUserOverride(name) {
			fmt.Fprintln(cmd.ErrOrStderr(), "[prompts] no user override — nothing to diff")
			return nil
		}
		def, err := store.EmbeddedDefault(name)
		if err != nil {
			return err
		}
		override, _, err := store.Resolve(name)
		if err != nil {
			return err
		}
		fmt.Println(simpleDiff(def, override))
		return nil
	},
}

// ── glitch prompts path ─────────────────────────────────────────────────────

var promptsPathCmd = &cobra.Command{
	Use:   "path <name>",
	Short: "Print where the user override would live (existing or not)",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		store := promptsStore()
		name := research.PromptName(args[0])
		path := store.UserOverridePath(name)
		if path == "" {
			return fmt.Errorf("could not resolve user override path")
		}
		fmt.Println(path)
		return nil
	},
}

// simpleDiff is a one-screen unified-style diff that doesn't depend on
// an external diff binary. The CLI uses it for `glitch prompts diff`
// because we don't want to fork to /usr/bin/diff just to render a
// 30-line file comparison. The output is informational, not
// machine-parseable.
func simpleDiff(a, b string) string {
	la := strings.Split(a, "\n")
	lb := strings.Split(b, "\n")
	var out strings.Builder
	max := len(la)
	if len(lb) > max {
		max = len(lb)
	}
	for i := 0; i < max; i++ {
		var av, bv string
		if i < len(la) {
			av = la[i]
		}
		if i < len(lb) {
			bv = lb[i]
		}
		if av == bv {
			fmt.Fprintf(&out, "  %s\n", av)
			continue
		}
		if av != "" {
			fmt.Fprintf(&out, "- %s\n", av)
		}
		if bv != "" {
			fmt.Fprintf(&out, "+ %s\n", bv)
		}
	}
	return out.String()
}
