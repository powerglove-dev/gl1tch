package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"

	"github.com/adam-stokes/orcai/internal/pipeline"
	"github.com/adam-stokes/orcai/internal/plugin"
	"github.com/adam-stokes/orcai/internal/promptbuilder"
)

func init() {
	rootCmd.AddCommand(pipelineCmd)
	pipelineCmd.AddCommand(pipelineBuildCmd)
	pipelineCmd.AddCommand(pipelineRunCmd)
}

var pipelineCmd = &cobra.Command{
	Use:   "pipeline",
	Short: "Manage and run AI pipelines",
}

var pipelineBuildCmd = &cobra.Command{
	Use:   "build",
	Short: "Open the interactive pipeline builder",
	RunE: func(cmd *cobra.Command, args []string) error {
		mgr := plugin.NewManager()
		for _, name := range []string{"claude", "gemini", "openspec", "openclaw"} {
			mgr.Register(plugin.NewCliAdapter(name, name+" CLI adapter", name))
		}

		m := promptbuilder.New(mgr)
		m.SetName("new-pipeline")
		m.AddStep(pipeline.Step{ID: "input", Type: "input", Prompt: "Enter your prompt:"})
		m.AddStep(pipeline.Step{ID: "step1", Plugin: "claude", Model: "claude-sonnet-4-6"})
		m.AddStep(pipeline.Step{ID: "output", Type: "output"})

		bubble := promptbuilder.NewBubble(m, nil)
		p := tea.NewProgram(bubble, tea.WithAltScreen())
		if _, err := p.Run(); err != nil {
			return fmt.Errorf("pipeline builder: %w", err)
		}
		return nil
	},
}

var pipelineRunCmd = &cobra.Command{
	Use:   "run <name>",
	Short: "Run a saved pipeline by name",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]
		configDir, err := orcaiConfigDir()
		if err != nil {
			return err
		}

		yamlPath := filepath.Join(configDir, "pipelines", name+".pipeline.yaml")
		f, err := os.Open(yamlPath)
		if err != nil {
			return fmt.Errorf("pipeline %q not found: %w", name, err)
		}
		defer f.Close()

		p, err := pipeline.Load(f)
		if err != nil {
			return err
		}

		mgr := plugin.NewManager()
		for _, n := range []string{"claude", "gemini", "openspec", "openclaw"} {
			mgr.Register(plugin.NewCliAdapter(n, n+" CLI adapter", n))
		}

		result, err := pipeline.Run(cmd.Context(), p, mgr, "")
		if err != nil {
			return err
		}
		fmt.Println(result)
		return nil
	},
}

func orcaiConfigDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "orcai"), nil
}
