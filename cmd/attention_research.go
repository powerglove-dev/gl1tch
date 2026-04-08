// attention_research.go implements `glitch attention research
// {show|path|edit|ensure}` — the CLI entry point for inspecting and
// editing the workspace research prompt that drives the attention
// classifier and the deep-analysis artifact rubric.
//
// The research prompt is free-form markdown (see
// pkg/glitchd/prompts/research_default.md) so this subcommand does
// nothing clever — it just resolves the right file path, opens it
// in $EDITOR, or dumps the effective content. The complexity lives
// in the loader's fallback chain, which this file intentionally
// leaves alone.
package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/8op-org/gl1tch/pkg/glitchd"
)

var attentionResearchCmd = &cobra.Command{
	Use:   "research {show|path|edit|ensure}",
	Short: "View or edit the workspace research prompt",
	Long: `Show, locate, edit, or seed the workspace research prompt — the
markdown file that tells gl1tch what counts as "needs my attention"
and what artifact to produce for high-attention events.

Subcommands:
  show      print the effective research prompt (per-workspace → global → bundled default)
  path      print the per-workspace file path (whether it exists or not)
  ensure    create the per-workspace file from the bundled default if missing
  edit      open the per-workspace file in $EDITOR (creates from default first)
`,
	ValidArgs: []string{"show", "path", "edit", "ensure"},
	Args:      cobra.ExactValidArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		switch args[0] {
		case "show":
			return runResearchShow()
		case "path":
			return runResearchPath()
		case "ensure":
			return runResearchEnsure()
		case "edit":
			return runResearchEdit()
		}
		return fmt.Errorf("unknown subcommand: %s", args[0])
	},
}

// runResearchShow dumps whatever the loader would hand to the
// classifier right now — the per-workspace file if it exists, else
// the global fallback, else the bundled default. This is the
// "what rules is gl1tch actually following?" answer.
func runResearchShow() error {
	text, err := glitchd.LoadResearchPrompt(attentionWorkspace)
	if err != nil {
		return fmt.Errorf("load research prompt: %w", err)
	}
	fmt.Println(text)
	return nil
}

// runResearchPath prints the per-workspace file location. The file
// does not need to exist — this is the "where do I put my override?"
// answer. Empty --workspace prints the global fallback path.
func runResearchPath() error {
	path, err := glitchd.ResearchPromptPath(attentionWorkspace)
	if err != nil {
		return fmt.Errorf("resolve research prompt path: %w", err)
	}
	fmt.Println(path)
	return nil
}

// runResearchEnsure seeds the per-workspace file from the bundled
// default if it doesn't already exist. Idempotent — existing files
// are preserved. A workspace id is required because we refuse to
// auto-seed the global fallback from this command.
func runResearchEnsure() error {
	if attentionWorkspace == "" {
		return fmt.Errorf("--workspace is required for ensure")
	}
	if err := glitchd.EnsureResearchPrompt(attentionWorkspace); err != nil {
		return fmt.Errorf("ensure research prompt: %w", err)
	}
	path, _ := glitchd.ResearchPromptPath(attentionWorkspace)
	fmt.Fprintf(os.Stderr, "seeded %s\n", path)
	return nil
}

// runResearchEdit opens the per-workspace file in $EDITOR (falls
// back to `vi` like git does), seeding it first if it doesn't
// exist so the editor always opens onto something usable.
func runResearchEdit() error {
	if attentionWorkspace == "" {
		return fmt.Errorf("--workspace is required for edit")
	}
	if err := glitchd.EnsureResearchPrompt(attentionWorkspace); err != nil {
		return fmt.Errorf("seed research prompt: %w", err)
	}
	path, err := glitchd.ResearchPromptPath(attentionWorkspace)
	if err != nil {
		return fmt.Errorf("resolve research prompt path: %w", err)
	}

	editor := os.Getenv("VISUAL")
	if editor == "" {
		editor = os.Getenv("EDITOR")
	}
	if editor == "" {
		editor = "vi"
	}

	// The editor string may contain flags (e.g. "code -w"); split
	// on spaces so `code -w` becomes argv rather than a single
	// binary name that exec can't find.
	parts := splitEditor(editor)
	parts = append(parts, path)
	c := exec.Command(parts[0], parts[1:]...) // #nosec G204 — user-controlled $EDITOR is intentional
	c.Stdin = os.Stdin
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	if err := c.Run(); err != nil {
		return fmt.Errorf("editor %q: %w", editor, err)
	}

	fmt.Fprintf(os.Stderr, "wrote %s\n", filepath.Clean(path))
	return nil
}

// splitEditor splits an $EDITOR value into argv the way a POSIX
// shell would for the common cases. It is NOT a full shell parser —
// quoted paths with embedded spaces will be mis-split — but the
// common cases ("code -w", "nvim --noplugin", "vim -p") work, and
// anything fancier can be set via VISUAL on a per-session basis.
func splitEditor(s string) []string {
	out := make([]string, 0, 2)
	start := -1
	for i, r := range s {
		if r == ' ' || r == '\t' {
			if start >= 0 {
				out = append(out, s[start:i])
				start = -1
			}
			continue
		}
		if start < 0 {
			start = i
		}
	}
	if start >= 0 {
		out = append(out, s[start:])
	}
	if len(out) == 0 {
		out = []string{"vi"}
	}
	return out
}
