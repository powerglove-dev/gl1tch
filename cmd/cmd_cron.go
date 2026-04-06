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

	"github.com/8op-org/gl1tch/internal/cron"
	robfigcron "github.com/robfig/cron/v3"
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(cronCmd)
	cronCmd.AddCommand(cronListCmd)
	cronCmd.AddCommand(cronLogsCmd)
	cronCmd.AddCommand(cronRunCmd)
}

var cronCmd = &cobra.Command{
	Use:   "cron",
	Short: "Manage recurring workflow and agent schedules",
	Long: `The cron scheduler runs automatically as part of the gl1tch supervisor
when you launch glitch. Use these subcommands to inspect and manage schedules.

  glitch cron list    — show all entries and next fire times
  glitch cron logs    — tail the cron log file
  glitch cron run     — run the scheduler standalone (for headless servers)`,
}

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
				next := sched.Next(now)
				nextStr = fmt.Sprintf("%s (%s)", humanDuration(next.Sub(now)), next.Format("15:04 MST"))
			}
			fmt.Printf("%-24s %-20s %-12s %-30s %s\n", e.Name, e.Schedule, e.Kind, e.Target, nextStr)
		}
		return nil
	},
}

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

// humanDuration formats a duration as a short human-readable string.
func humanDuration(d time.Duration) string {
	if d < 0 {
		return "overdue"
	}
	d = d.Round(time.Minute)
	if d == 0 {
		return "now"
	}
	days := int(d.Hours()) / 24
	hours := int(d.Hours()) % 24
	mins := int(d.Minutes()) % 60
	switch {
	case days > 0 && hours > 0:
		return fmt.Sprintf("in %dd %dh", days, hours)
	case days > 0:
		return fmt.Sprintf("in %dd", days)
	case hours > 0 && mins > 0:
		return fmt.Sprintf("in %dh %dm", hours, mins)
	case hours > 0:
		return fmt.Sprintf("in %dh", hours)
	default:
		return fmt.Sprintf("in %dm", mins)
	}
}

// cronRunCmd is the standalone daemon for headless servers.
var cronRunCmd = &cobra.Command{
	Use:   "run",
	Short: "Run the cron scheduler standalone (for headless servers without the TUI)",
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

		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		<-sigCh

		signal.Stop(sigCh)
		scheduler.Stop()
		return nil
	},
}
