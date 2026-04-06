package cmd

import (
	"github.com/8op-org/gl1tch/internal/bootstrap"
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(tuiCmd)
}

var tuiCmd = &cobra.Command{
	Use:   "tui",
	Short: "Start the BubbleTea terminal UI (legacy)",
	RunE: func(cmd *cobra.Command, args []string) error {
		return bootstrap.Run()
	},
}
