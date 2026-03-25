package cmd

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(configCmd)
	configCmd.AddCommand(configInitCmd)
}

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Manage orcai configuration",
}

var configInitCmd = &cobra.Command{
	Use:   "init",
	Short: "Write default layout.yaml and keybindings.yaml",
	RunE:  runConfigInit,
}

const defaultLayoutYAML = `# orcai layout configuration
# Panes are created at session attach time. Remove this file to disable layout init.
# Available widgets: welcome, sysop, pipeline-builder
panes:
  - name: welcome
    widget: welcome
    position: right
    size: 50%
  - name: sysop
    widget: sysop
    position: right
    size: 40%
`

const defaultKeybindingsYAML = `# orcai keybinding configuration
# Only keys listed here are bound. Remove entries to preserve your tmux bindings.
bindings:
  - key: "M-n"
    action: launch-session-picker
  - key: "M-t"
    action: open-sysop
  - key: "M-w"
    action: open-welcome
  # Pane resizing (5 cells per keypress; vim-style keys work reliably on macOS)
  - key: "M-h"
    action: resize-pane-left
  - key: "M-l"
    action: resize-pane-right
  - key: "M-k"
    action: resize-pane-up
  - key: "M-j"
    action: resize-pane-down
`

func runConfigInit(cmd *cobra.Command, args []string) error {
	cfgDir, err := orcaiConfigDir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		return fmt.Errorf("config init: mkdir %s: %w", cfgDir, err)
	}

	files := []struct {
		name    string
		content string
	}{
		{"layout.yaml", defaultLayoutYAML},
		{"keybindings.yaml", defaultKeybindingsYAML},
	}

	for _, f := range files {
		path := filepath.Join(cfgDir, f.name)
		if _, err := os.Stat(path); err == nil {
			if !confirm(fmt.Sprintf("%s already exists. Overwrite?", path)) {
				fmt.Printf("skipped %s\n", path)
				continue
			}
		}
		if err := os.WriteFile(path, []byte(f.content), 0o644); err != nil {
			return fmt.Errorf("config init: write %s: %w", path, err)
		}
		fmt.Printf("wrote %s\n", path)
	}
	return nil
}

// confirm prompts the user with a yes/no question and returns true for yes.
func confirm(prompt string) bool {
	fmt.Printf("%s [y/N] ", prompt)
	scanner := bufio.NewScanner(os.Stdin)
	if scanner.Scan() {
		answer := strings.TrimSpace(strings.ToLower(scanner.Text()))
		return answer == "y" || answer == "yes"
	}
	return false
}
