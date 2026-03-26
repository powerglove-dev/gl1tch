package cmd

import (
	"github.com/adam-stokes/orcai/internal/switchboard"
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(welcomeCmd)
	welcomeCmd.Flags().String("bus-socket", "", "Path to orcai bus Unix socket (unused; retained for backwards compat)")
}

var welcomeCmd = &cobra.Command{
	Use:   "welcome",
	Short: "Open the ABBS Switchboard",
	RunE: func(cmd *cobra.Command, args []string) error {
		switchboard.Run()
		return nil
	},
}
