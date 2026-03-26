package cmd

import (
	"github.com/adam-stokes/orcai/internal/switchboard"
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(sysopCmd)
	sysopCmd.Flags().String("bus-socket", "", "Path to orcai bus Unix socket")
}

var sysopCmd = &cobra.Command{
	Use:   "sysop",
	Short: "Open the ABBS Switchboard",
	RunE: func(cmd *cobra.Command, args []string) error {
		switchboard.Run()
		return nil
	},
}
