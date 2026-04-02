package cmd

import (
	"fmt"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/8op-org/gl1tch/internal/pluginmanager"
)

func init() {
	rootCmd.AddCommand(pluginCmd)
	pluginCmd.AddCommand(pluginInstallCmd)
	pluginCmd.AddCommand(pluginRemoveCmd)
	pluginCmd.AddCommand(pluginListCmd)
}

var pluginCmd = &cobra.Command{
	Use:   "plugin",
	Short: "Manage glitch plugins",
}

var pluginInstallCmd = &cobra.Command{
	Use:   "install <owner/repo[@version]>",
	Short: "Install a plugin from a GitHub repository",
	Args:  cobra.ExactArgs(1),
	RunE:  runPluginInstall,
}

var pluginRemoveCmd = &cobra.Command{
	Use:     "remove <name>",
	Aliases: []string{"rm", "uninstall"},
	Short:   "Remove an installed plugin",
	Args:    cobra.ExactArgs(1),
	RunE:    runPluginRemove,
}

var pluginListCmd = &cobra.Command{
	Use:     "list",
	Aliases: []string{"ls"},
	Short:   "List installed plugins",
	RunE:    runPluginList,
}

func runPluginInstall(cmd *cobra.Command, args []string) error {
	configDir, err := glitchConfigDir()
	if err != nil {
		return err
	}
	inst := pluginmanager.NewInstaller(configDir)
	result, err := inst.Install(cmd.Context(), args[0])
	if err != nil {
		return err
	}
	fmt.Fprintf(cmd.OutOrStdout(), "installed %s (%s) → %s\n",
		result.Plugin.Name, result.Plugin.Source, result.BinaryPath)
	fmt.Fprintf(cmd.OutOrStdout(), "sidecar written to %s\n", result.Plugin.SidecarPath)
	fmt.Fprintf(cmd.OutOrStdout(), "restart glitch or run `glitch ask` to activate\n")
	return nil
}

func runPluginRemove(cmd *cobra.Command, args []string) error {
	configDir, err := glitchConfigDir()
	if err != nil {
		return err
	}
	inst := pluginmanager.NewInstaller(configDir)
	if err := inst.Remove(args[0]); err != nil {
		return err
	}
	fmt.Fprintf(cmd.OutOrStdout(), "removed plugin %q\n", args[0])
	fmt.Fprintf(cmd.OutOrStdout(), "note: binary not removed; delete it from GOPATH/bin or ~/.local/bin if needed\n")
	return nil
}

func runPluginList(cmd *cobra.Command, args []string) error {
	configDir, err := glitchConfigDir()
	if err != nil {
		return err
	}
	inst := pluginmanager.NewInstaller(configDir)
	plugins, err := inst.List()
	if err != nil {
		return err
	}
	if len(plugins) == 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "no plugins installed")
		return nil
	}
	w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "NAME\tSOURCE\tVERSION\tBINARY")
	for _, p := range plugins {
		version := p.Version
		if version == "" {
			version = "latest"
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", p.Name, p.Source, version, p.BinaryPath)
	}
	return w.Flush()
}
