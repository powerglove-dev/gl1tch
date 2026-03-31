package cmd

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"

	"github.com/adam-stokes/orcai/internal/assistant"
	"github.com/adam-stokes/orcai/internal/picker"
	"github.com/adam-stokes/orcai/internal/store"
)

func init() {
	rootCmd.AddCommand(assistantCmd)
}

var assistantCmd = &cobra.Command{
	Use:   "assistant",
	Short: "GLITCH AI assistant",
	Long:  "Opens the GLITCH AI assistant TUI for free-form conversation.",
	RunE:  runAssistantTUI,
}

func runAssistantTUI(cmd *cobra.Command, args []string) error {
	// If inside tmux and not already in the assistant window, open a new window.
	if os.Getenv("TMUX") != "" {
		out, _ := exec.Command("tmux", "display-message", "-p", "#W").Output()
		if strings.TrimSpace(string(out)) != "orcai-assistant" {
			self, err := os.Executable()
			if err != nil {
				return fmt.Errorf("assistant: resolve executable: %w", err)
			}
			return exec.Command("tmux", "new-window", "-n", "orcai-assistant",
				filepath.Clean(self)+" assistant").Run()
		}
	}

	defer func() {
		if r := recover(); r != nil {
			fmt.Fprintf(os.Stderr, "assistant: panic: %v\n", r)
			os.Exit(2)
		}
	}()

	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("assistant: home dir: %w", err)
	}
	cfgDir := filepath.Join(home, ".config", "orcai")

	st, err := store.Open()
	if err != nil {
		return fmt.Errorf("assistant: open store: %w", err)
	}
	defer st.Close()

	providers := picker.BuildProviders()
	backend := assistant.NewBestBackend(providers)

	m := assistant.New(cfgDir, backend)

	p := tea.NewProgram(m, tea.WithAltScreen())
	finalModel, err := p.Run()
	if err != nil {
		return err
	}

	if am, ok := finalModel.(assistant.Model); ok {
		assistant.SaveToBrain(context.Background(), st, am.Turns()) //nolint:errcheck
	}

	return nil
}
