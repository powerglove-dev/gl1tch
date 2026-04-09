// slash.go is the CLI surface for user-defined slash command
// aliases. The Go-registered slash commands (/help, /research) live
// in code because they ARE Go logic. Aliases are the user-extensible
// layer: short name → expansion line that re-dispatches.
//
// Example user file (~/.config/glitch/slash.yaml):
//
//   aliases:
//     - name: prs
//       describe: List open pull requests
//       expand: "/research what pull requests are currently open"
//
// Sub-commands parallel `glitch prompts` / `glitch researchers` /
// `glitch weights`:
//
//   glitch slash list   — current aliases + source
//   glitch slash show   — print the resolved YAML body
//   glitch slash edit   — seed override + open $EDITOR
//   glitch slash reset  — drop the override
//   glitch slash diff   — override vs embedded seed
//   glitch slash path   — print where the override would write
package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/8op-org/gl1tch/internal/chatui"
)

var slashCmd = &cobra.Command{
	Use:   "slash",
	Short: "Inspect and edit user-defined slash command aliases",
	Long: `Slash command aliases let you map short names to longer
research expansions without recompiling. Each alias is one entry in
~/.config/glitch/slash.yaml. The chatui slash dispatcher loads them
on every registry construction so the next chat session picks up
your edits automatically.

Aliases NEVER override built-ins like /help and /research — those
always win on name collisions. Aliases are for the user-extensible
layer: muscle-memory shortcuts, templated research calls, and
project-specific shortcuts.`,
}

func init() {
	rootCmd.AddCommand(slashCmd)
	slashCmd.AddCommand(slashListCmd)
	slashCmd.AddCommand(slashShowCmd)
	slashCmd.AddCommand(slashEditCmd)
	slashCmd.AddCommand(slashResetCmd)
	slashCmd.AddCommand(slashDiffCmd)
	slashCmd.AddCommand(slashPathCmd)
}

// ── glitch slash list ───────────────────────────────────────────────────────

var slashListCmd = &cobra.Command{
	Use:   "list",
	Short: "List the user-defined aliases and their source",
	RunE: func(cmd *cobra.Command, args []string) error {
		aliases, err := chatui.LoadSlashAliases()
		if err != nil {
			return err
		}
		path := chatui.SlashAliasOverridePath()
		source := "embedded (no aliases)"
		if path != "" {
			if _, err := os.Stat(path); err == nil {
				source = path
			}
		}
		fmt.Printf("source: %s\n\n", source)
		if len(aliases) == 0 {
			fmt.Println("(no aliases configured — try `glitch slash edit`)")
			return nil
		}
		fmt.Printf("%-16s  %s\n", "NAME", "EXPANDS TO")
		fmt.Printf("%-16s  %s\n", "----", "----------")
		for _, a := range aliases {
			fmt.Printf("%-16s  %s\n", "/"+a.Name, a.Expand)
			if a.Describe != "" {
				fmt.Printf("%-16s    %s\n", "", a.Describe)
			}
		}
		return nil
	},
}

// ── glitch slash show ───────────────────────────────────────────────────────

var slashShowCmd = &cobra.Command{
	Use:   "show",
	Short: "Print the resolved aliases YAML",
	RunE: func(cmd *cobra.Command, args []string) error {
		path := chatui.SlashAliasOverridePath()
		if path != "" {
			if data, err := os.ReadFile(path); err == nil {
				fmt.Fprintf(cmd.ErrOrStderr(), "# source: %s\n", path)
				fmt.Print(string(data))
				return nil
			}
		}
		fmt.Fprintln(cmd.ErrOrStderr(), "# source: embedded seed (no override)")
		fmt.Print(string(chatui.EmbeddedSlashAliasesDefault()))
		return nil
	},
}

// ── glitch slash edit ───────────────────────────────────────────────────────

var slashEditCmd = &cobra.Command{
	Use:   "edit",
	Short: "Seed the user override from the embedded template and open $EDITOR",
	Long: `If no user file exists, copies the embedded seed (which has
example aliases commented out) so $EDITOR opens with a working
template. The next slash registry construction (next desktop
session, next CLI invocation) will pick up your changes.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		path := chatui.SlashAliasOverridePath()
		if path == "" {
			return fmt.Errorf("could not resolve user override path")
		}
		if _, err := os.Stat(path); err != nil {
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				return err
			}
			if err := os.WriteFile(path, chatui.EmbeddedSlashAliasesDefault(), 0o644); err != nil {
				return err
			}
			fmt.Fprintf(cmd.ErrOrStderr(), "[slash] seeded %s from embedded template\n", path)
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

// ── glitch slash reset ──────────────────────────────────────────────────────

var slashResetCmd = &cobra.Command{
	Use:   "reset",
	Short: "Delete the user override",
	RunE: func(cmd *cobra.Command, args []string) error {
		path := chatui.SlashAliasOverridePath()
		if path == "" {
			return fmt.Errorf("could not resolve user override path")
		}
		if err := os.Remove(path); err != nil {
			if os.IsNotExist(err) {
				fmt.Fprintf(cmd.ErrOrStderr(), "[slash] no user override at %s — nothing to reset\n", path)
				return nil
			}
			return err
		}
		fmt.Fprintf(cmd.ErrOrStderr(), "[slash] removed %s\n", path)
		return nil
	},
}

// ── glitch slash diff ───────────────────────────────────────────────────────

var slashDiffCmd = &cobra.Command{
	Use:   "diff",
	Short: "Show how the user override differs from the embedded seed",
	RunE: func(cmd *cobra.Command, args []string) error {
		path := chatui.SlashAliasOverridePath()
		if path == "" {
			return fmt.Errorf("could not resolve user override path")
		}
		userBody, err := os.ReadFile(path)
		if err != nil {
			fmt.Fprintln(cmd.ErrOrStderr(), "[slash] no user override — nothing to diff")
			return nil
		}
		def := chatui.EmbeddedSlashAliasesDefault()
		fmt.Println(simpleDiff(string(def), string(userBody)))
		return nil
	},
}

// ── glitch slash path ───────────────────────────────────────────────────────

var slashPathCmd = &cobra.Command{
	Use:   "path",
	Short: "Print where the user override would live (existing or not)",
	RunE: func(cmd *cobra.Command, args []string) error {
		path := chatui.SlashAliasOverridePath()
		if path == "" {
			return fmt.Errorf("could not resolve user override path")
		}
		fmt.Println(path)
		return nil
	},
}
