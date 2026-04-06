package cmd

import (
	"github.com/8op-org/gl1tch/internal/bootstrap"
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(serveCmd)
}

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Run glitch backend services without the TUI",
	Long:  "Start the supervisor, collectors, busd, and cron in headless mode. Used by the desktop GUI.",
	RunE: func(cmd *cobra.Command, args []string) error {
		return bootstrap.RunHeadless(cmd.Context())
	},
}
