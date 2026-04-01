package cmd

import (
	"github.com/spf13/cobra"

	"github.com/8op-org/gl1tch/internal/opsx"
)

var opsxCmd = &cobra.Command{
	Use:    "_opsx",
	Hidden: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		feature := opsx.Prompt()
		if feature == "" {
			return nil
		}
		providerID := opsx.DetectActiveProvider()
		workdir := opsx.ActivePanePath()
		opsx.ProviderSend(feature, providerID, workdir)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(opsxCmd)
}
