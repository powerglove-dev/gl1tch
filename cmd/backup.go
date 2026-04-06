package cmd

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/8op-org/gl1tch/internal/backup"
	"github.com/8op-org/gl1tch/internal/store"
)

var (
	backupOutput        string
	backupExcludeAgents bool
)

func init() {
	rootCmd.AddCommand(backupCmd)
	backupCmd.Flags().StringVar(&backupOutput, "output", "", "output path for the backup archive (default: glitch-backup-<date>.tar.gz)")
	backupCmd.Flags().BoolVar(&backupExcludeAgents, "no-agents", false, "(deprecated, no-op) exclude auto-generated agent files")
}

var backupCmd = &cobra.Command{
	Use:   "backup",
	Short: "Backup config, prompts, and brain data",
	RunE: func(cmd *cobra.Command, args []string) error {
		s, err := store.Open()
		if err != nil {
			return fmt.Errorf("open store: %w", err)
		}
		defer s.Close()

		m, err := backup.Run(context.Background(), s, backup.BackupOptions{
			Output:        backupOutput,
			ExcludeAgents: backupExcludeAgents,
		})
		if err != nil {
			return err
		}

		fmt.Printf("Backup created: %s\n", m.Path)
		fmt.Printf("  Config files:  %d\n", m.FileCount)
		fmt.Printf("  Brain notes:   %d\n", m.NoteCount)
		fmt.Printf("  Saved prompts: %d\n", m.PromptCount)
		return nil
	},
}
