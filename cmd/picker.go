package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(pickerCmd)
}

var pickerCmd = &cobra.Command{
	Use:   "picker",
	Short: "Deprecated: use the agent runner overlay in the switchboard",
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Println("orcai picker: use 'a' in the orcai switchboard to start a new agent session.")
		return nil
	},
}
