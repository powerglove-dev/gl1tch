package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"

	"github.com/adam-stokes/orcai/internal/braineditor"
	"github.com/adam-stokes/orcai/internal/picker"
	"github.com/adam-stokes/orcai/internal/store"
	"github.com/adam-stokes/orcai/internal/styles"
	"github.com/adam-stokes/orcai/internal/themes"
)

func init() {
	rootCmd.AddCommand(brainCmd)
	brainCmd.AddCommand(brainStartCmd)
}

// brainCmd is the top-level brain sub-command.
var brainCmd = &cobra.Command{
	Use:   "brain",
	Short: "Browse and edit brain notes",
	Long:  "Opens the interactive brain note editor (two-column TUI).",
	RunE:  runBrainTUI,
}

// brainStartCmd opens the brain editor in a new tmux window.
var brainStartCmd = &cobra.Command{
	Use:   "start",
	Short: "Open the brain editor in a new tmux window",
	RunE: func(cmd *cobra.Command, args []string) error {
		self, err := os.Executable()
		if err != nil {
			return fmt.Errorf("brain: resolve executable: %w", err)
		}
		self = filepath.Clean(self)
		if err := exec.Command("tmux", "new-window", "-n", "orcai-brain", self+" brain").Run(); err != nil {
			return fmt.Errorf("brain: open tmux window: %w", err)
		}
		return nil
	},
}

func runBrainTUI(cmd *cobra.Command, args []string) error {
	// If inside tmux and not already in the brain window, open a new window.
	if os.Getenv("TMUX") != "" {
		out, _ := exec.Command("tmux", "display-message", "-p", "#W").Output()
		if strings.TrimSpace(string(out)) != "orcai-brain" {
			self, err := os.Executable()
			if err != nil {
				return fmt.Errorf("brain: resolve executable: %w", err)
			}
			return exec.Command("tmux", "new-window", "-n", "orcai-brain",
				filepath.Clean(self)+" brain").Run()
		}
	}

	defer func() {
		if r := recover(); r != nil {
			fmt.Fprintf(os.Stderr, "brain: panic: %v\n", r)
			os.Exit(2)
		}
	}()

	home, _ := os.UserHomeDir()
	var pal styles.ANSIPalette
	if reg, err := themes.NewRegistry(filepath.Join(home, ".config", "orcai", "themes")); err == nil {
		themes.SetGlobalRegistry(reg)
		if bundle := reg.Active(); bundle != nil {
			pal = styles.BundleANSI(bundle)
		}
	}

	st, err := store.Open()
	if err != nil {
		return fmt.Errorf("brain: open store: %w", err)
	}
	defer st.Close()

	providers := picker.BuildProviders()
	m := braineditor.New(st, providers)
	if pal.Accent != "" {
		m.SetPalette(pal)
	}

	p := tea.NewProgram(m, tea.WithAltScreen())
	_, err = p.Run()
	return err
}
