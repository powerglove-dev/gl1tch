package cmd

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/8op-org/gl1tch/internal/cron"
	crontui "github.com/8op-org/gl1tch/internal/crontui"
	"github.com/8op-org/gl1tch/internal/themes"
	robfigcron "github.com/robfig/cron/v3"
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(cronCmd)
	cronCmd.AddCommand(cronStartCmd)
	cronCmd.AddCommand(cronStopCmd)
	cronCmd.AddCommand(cronListCmd)
	cronCmd.AddCommand(cronLogsCmd)
	cronCmd.AddCommand(cronRunCmd)
	cronCmd.AddCommand(cronTuiCmd)

	cronStartCmd.Flags().Bool("force", false, "kill an existing cron session before starting")
}

// cronCmd is the top-level cron command group.
var cronCmd = &cobra.Command{
	Use:   "cron",
	Short: "Manage recurring pipeline and agent schedules",
}

// cronStartCmd starts the cron daemon in a detached tmux session named
// "glitch-cron". Use --force to replace an existing session.
var cronStartCmd = &cobra.Command{
	Use:   "start",
	Short: "Start the cron daemon in a background tmux session",
	RunE: func(cmd *cobra.Command, args []string) error {
		force, _ := cmd.Flags().GetBool("force")

		// Check whether the session already exists.
		checkCmd := exec.Command("tmux", "has-session", "-t", "glitch-cron")
		sessionExists := checkCmd.Run() == nil

		if sessionExists {
			if !force {
				fmt.Fprintln(os.Stderr, "cron: session 'glitch-cron' is already running. Use --force to restart.")
				os.Exit(1)
			}
			// Kill the existing session.
			if err := exec.Command("tmux", "kill-session", "-t", "glitch-cron").Run(); err != nil {
				return fmt.Errorf("cron: kill existing session: %w", err)
			}
		}

		// Resolve absolute path of the running binary so the tmux session
		// can invoke it even when glitch is not in PATH.
		self, err := os.Executable()
		if err != nil {
			return fmt.Errorf("cron: resolve executable: %w", err)
		}
		self = filepath.Clean(self)

		// Create the new session running the TUI (falls back to bare daemon
		// if invoked in a non-interactive/CI context via "cron run").
		newArgs := []string{
			"new-session", "-d", "-s", "glitch-cron",
			"-x", "220", "-y", "50",
			self + " cron tui",
		}
		if err := exec.Command("tmux", newArgs...).Run(); err != nil {
			return fmt.Errorf("cron: start session: %w", err)
		}
		// Label the window so the jump window popup shows "glitch-cron".
		exec.Command("tmux", "set-window-option", "-t", "glitch-cron:0", "@glitch-label", "glitch-cron").Run() //nolint:errcheck
		fmt.Println("cron: daemon started in tmux session 'glitch-cron'")
		return nil
	},
}

// cronStopCmd kills the glitch-cron tmux session.
var cronStopCmd = &cobra.Command{
	Use:   "stop",
	Short: "Stop the cron daemon",
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := exec.Command("tmux", "kill-session", "-t", "glitch-cron").Run(); err != nil {
			fmt.Fprintln(os.Stderr, "cron: daemon is not running (no 'glitch-cron' session found)")
			return nil
		}
		fmt.Println("cron: daemon stopped")
		return nil
	},
}

// cronListCmd prints all configured schedule entries with their next fire time.
var cronListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all configured cron entries with next-fire times",
	RunE: func(cmd *cobra.Command, args []string) error {
		entries, err := cron.LoadConfig()
		if err != nil {
			return fmt.Errorf("cron: load config: %w", err)
		}
		if len(entries) == 0 {
			fmt.Println("cron: no entries configured in ~/.config/glitch/cron.yaml")
			return nil
		}

		parser := robfigcron.NewParser(
			robfigcron.Minute | robfigcron.Hour | robfigcron.Dom | robfigcron.Month | robfigcron.Dow,
		)

		now := time.Now()
		fmt.Printf("%-24s %-20s %-12s %-30s %s\n", "NAME", "SCHEDULE", "KIND", "TARGET", "NEXT RUN")
		fmt.Println("--------------------------------------------------------------------------------------------------------------")
		for _, e := range entries {
			nextStr := "invalid schedule"
			if sched, err := parser.Parse(e.Schedule); err == nil {
				nextStr = sched.Next(now).Format("2006-01-02 15:04:05 MST")
			}
			fmt.Printf("%-24s %-20s %-12s %-30s %s\n", e.Name, e.Schedule, e.Kind, e.Target, nextStr)
		}
		return nil
	},
}

// cronLogsCmd tails the cron daemon log file.
var cronLogsCmd = &cobra.Command{
	Use:   "logs",
	Short: "Tail the cron daemon log file",
	RunE: func(cmd *cobra.Command, args []string) error {
		home, err := os.UserHomeDir()
		if err != nil {
			return err
		}
		logPath := filepath.Join(home, ".local", "share", "glitch", "cron.log")

		tail := exec.Command("tail", "-f", logPath)
		tail.Stdout = os.Stdout
		tail.Stderr = os.Stderr
		if err := tail.Run(); err != nil {
			return fmt.Errorf("cron: tail logs: %w", err)
		}
		return nil
	},
}

// cronTuiCmd runs the interactive BubbleTea TUI for glitch-cron.
var cronTuiCmd = &cobra.Command{
	Use:   "tui",
	Short: "Launch the interactive cron job manager TUI",
	RunE: func(cmd *cobra.Command, args []string) error {
		// Capture panics so the error is visible in the terminal.
		defer func() {
			if r := recover(); r != nil {
				fmt.Fprintf(os.Stderr, "cron tui: panic: %v\n", r)
				os.Exit(2)
			}
		}()

		// Load theme registry (built-in + user themes) and make it globally
		// accessible so busd theme-change events can look up bundles by name.
		var bundle *themes.Bundle
		home, _ := os.UserHomeDir()
		userThemesDir := filepath.Join(home, ".config", "glitch", "themes")
		if reg, err := themes.NewRegistry(userThemesDir); err == nil {
			bundle = reg.Active()
			themes.SetGlobalRegistry(reg)
		}

		m, err := crontui.New(bundle)
		if err != nil {
			return fmt.Errorf("cron tui: %w", err)
		}
		p := tea.NewProgram(m, tea.WithAltScreen())
		_, err = p.Run()
		return err
	},
}

// cronRunCmd is the internal daemon entry point. It is invoked by
// cronStartCmd inside the tmux session and should not normally be called
// directly by users.
var cronRunCmd = &cobra.Command{
	Use:    "run",
	Short:  "Run the cron daemon (internal — use 'cron start' instead)",
	Hidden: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		logger, err := cron.NewLogger()
		if err != nil {
			fmt.Fprintf(os.Stderr, "cron: logger setup warning: %v\n", err)
		}

		scheduler := cron.New(logger)

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		if err := scheduler.Start(ctx); err != nil {
			return fmt.Errorf("cron: start scheduler: %w", err)
		}

		// Block until SIGINT or SIGTERM.
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		<-sigCh

		signal.Stop(sigCh)
		scheduler.Stop()
		return nil
	},
}
