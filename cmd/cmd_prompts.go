package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"

	"github.com/adam-stokes/orcai/internal/plugin"
	"github.com/adam-stokes/orcai/internal/promptmgr"
	"github.com/adam-stokes/orcai/internal/store"
	"github.com/adam-stokes/orcai/internal/themes"
)

func init() {
	rootCmd.AddCommand(promptsCmd)
	promptsCmd.AddCommand(promptsTuiCmd)
	promptsCmd.AddCommand(promptsStartCmd)
}

var promptsCmd = &cobra.Command{
	Use:   "prompts",
	Short: "Manage AI prompts",
}

var promptsTuiCmd = &cobra.Command{
	Use:   "tui",
	Short: "Launch the interactive prompt manager TUI",
	RunE: func(cmd *cobra.Command, args []string) error {
		defer func() {
			if r := recover(); r != nil {
				fmt.Fprintf(os.Stderr, "prompts tui: panic: %v\n", r)
				os.Exit(2)
			}
		}()

		var bundle *themes.Bundle
		home, _ := os.UserHomeDir()
		userThemesDir := filepath.Join(home, ".config", "orcai", "themes")
		if reg, err := themes.NewRegistry(userThemesDir); err == nil {
			bundle = reg.Active()
			themes.SetGlobalRegistry(reg)
		}

		st, err := store.Open()
		if err != nil {
			return fmt.Errorf("prompts tui: open store: %w", err)
		}
		defer st.Close()

		pluginMgr := plugin.NewManager()
		home2, _ := os.UserHomeDir()
		pluginDir := filepath.Join(home2, ".config", "orcai", "plugins")
		pluginMgr.LoadWrappersFromDir(pluginDir)

		m := promptmgr.New(st, pluginMgr, bundle)
		p := tea.NewProgram(m, tea.WithAltScreen())
		_, err = p.Run()
		return err
	},
}

var promptsStartCmd = &cobra.Command{
	Use:   "start",
	Short: "Open the prompt manager in a tmux window",
	RunE: func(cmd *cobra.Command, args []string) error {
		self, err := os.Executable()
		if err != nil {
			return fmt.Errorf("prompts: resolve executable: %w", err)
		}
		self = filepath.Clean(self)
		newArgs := []string{
			"new-window", "-n", "orcai-prompts",
			self + " prompts tui",
		}
		if err := exec.Command("tmux", newArgs...).Run(); err != nil {
			return fmt.Errorf("prompts: open window: %w", err)
		}
		return nil
	},
}
