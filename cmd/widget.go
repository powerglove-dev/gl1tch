package cmd

import (
	"github.com/spf13/cobra"

	"github.com/8op-org/gl1tch/internal/jumpwindow"
)

func init() {
	rootCmd.AddCommand(widgetCmd)
	widgetCmd.AddCommand(jumpWindowCmd)
}

var widgetCmd = &cobra.Command{
	Use:   "widget",
	Short: "Reusable TUI widget subcommands",
}

var jumpWindowCmd = &cobra.Command{
	Use:   "jump-window",
	Short: "Open the jump window TUI (standalone)",
	RunE: func(cmd *cobra.Command, args []string) error {
		jumpwindow.Run()
		return nil
	},
}
