package cmd

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/8op-org/gl1tch/internal/backup"
	"github.com/8op-org/gl1tch/internal/store"
)

var (
	restoreOverwrite bool
	restoreDryRun    bool
)

func init() {
	rootCmd.AddCommand(restoreCmd)
	restoreCmd.Flags().BoolVar(&restoreOverwrite, "overwrite", false, "overwrite existing config files")
	restoreCmd.Flags().BoolVar(&restoreDryRun, "dry-run", false, "preview changes without writing anything")
}

var restoreCmd = &cobra.Command{
	Use:   "restore <backup-file>",
	Short: "Restore config and brain data from a backup archive",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		s, err := store.Open()
		if err != nil {
			return fmt.Errorf("open store: %w", err)
		}
		defer s.Close()

		summary, err := backup.Restore(context.Background(), s, args[0], backup.RestoreOptions{
			Overwrite: restoreOverwrite,
			DryRun:    restoreDryRun,
		})
		if err != nil {
			return err
		}

		if restoreDryRun {
			fmt.Println("Dry run — no changes written.")
		}
		fmt.Printf("Config files:  %d written, %d skipped, %d overwritten\n",
			summary.FilesWritten, summary.FilesSkipped, summary.FilesOverwritten)
		fmt.Printf("Brain notes:   %d imported, %d skipped\n",
			summary.NotesImported, summary.NotesSkipped)
		fmt.Printf("Saved prompts: %d imported, %d skipped\n",
			summary.PromptsImported, summary.PromptsSkipped)
		return nil
	},
}
