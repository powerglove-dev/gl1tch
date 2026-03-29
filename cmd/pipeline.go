package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"

	"github.com/adam-stokes/orcai/internal/picker"
	"github.com/adam-stokes/orcai/internal/pipeline"
	"github.com/adam-stokes/orcai/internal/plugin"
	"github.com/adam-stokes/orcai/internal/promptbuilder"
	"github.com/adam-stokes/orcai/internal/store"
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
		providers := picker.BuildProviders()

		mgr := plugin.NewManager()
		for _, prov := range providers {
			mgr.Register(plugin.NewCliAdapter(prov.ID, prov.Label+" CLI adapter", prov.ID, prov.PipelineArgs...))
		}

		m := promptbuilder.New(mgr)
		m.SetName("new-pipeline")
		m.AddStep(pipeline.Step{ID: "input", Type: "input", Prompt: "Enter your prompt:"})
		m.AddStep(pipeline.Step{ID: "step1", Executor: "claude", Model: "claude-sonnet-4-6"})
		m.AddStep(pipeline.Step{ID: "output", Type: "output"})

		bubble := promptbuilder.NewBubble(m, providers)
		prog := tea.NewProgram(bubble, tea.WithAltScreen())
		if _, err := prog.Run(); err != nil {
			return fmt.Errorf("pipeline builder: %w", err)
		}
		return nil
	},
}

var pipelineRunCmd = &cobra.Command{
	Use:   "run <name|file>",
	Short: "Run a saved pipeline by name or file path",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		arg := args[0]

		// Accept either an absolute/relative file path or a bare name.
		// If the arg contains a path separator or ends in .yaml, treat it as a file path.
		var yamlPath string
		if strings.Contains(arg, string(filepath.Separator)) || strings.HasSuffix(arg, ".yaml") {
			yamlPath = arg
		} else {
			configDir, err := orcaiConfigDir()
			if err != nil {
				return err
			}
			yamlPath = filepath.Join(configDir, "pipelines", arg+".pipeline.yaml")
		}

		f, err := os.Open(yamlPath)
		if err != nil {
			return fmt.Errorf("pipeline %q not found: %w", arg, err)
		}
		defer f.Close()

		p, err := pipeline.Load(f)
		if err != nil {
			return err
		}

		runProviders := picker.BuildProviders()
		mgr := plugin.NewManager()
		for _, prov := range runProviders {
			// Sidecar-backed providers are fully registered by LoadWrappersFromDir below.
			if prov.SidecarPath != "" {
				continue
			}
			binary := prov.Command
			if binary == "" {
				binary = prov.ID
			}
			if err := mgr.Register(plugin.NewCliAdapter(prov.ID, prov.Label+" CLI adapter", binary, prov.PipelineArgs...)); err != nil {
				fmt.Fprintf(os.Stderr, "pipeline: register provider %q: %v\n", prov.ID, err)
			}
		}

		// Load sidecar plugins from ~/.config/orcai/wrappers/.
		wrappersConfigDir, _ := orcaiConfigDir()
		if wrappersConfigDir != "" {
			wrappersDir := filepath.Join(wrappersConfigDir, "wrappers")
			if errs := mgr.LoadWrappersFromDir(wrappersDir); len(errs) > 0 {
				for _, e := range errs {
					fmt.Fprintf(os.Stderr, "pipeline: sidecar load warning: %v\n", e)
				}
			}
		}

		// Open the result store so this run is recorded in the inbox.
		// A failure to open the store is non-fatal — the pipeline still runs.
		var storeOpts []pipeline.RunOption
		if s, serr := store.Open(); serr == nil {
			defer s.Close()
			storeOpts = append(storeOpts, pipeline.WithRunStore(s))
			// Wire brain context injection: use_brain / write_brain flags on pipeline
			// steps will prepend DB context and parse <brain> notes from responses.
			storeOpts = append(storeOpts, pipeline.WithBrainInjector(pipeline.NewStoreBrainInjector(s)))
		} else {
			fmt.Fprintf(os.Stderr, "pipeline: store unavailable: %v\n", serr)
		}

		// Wire busd publisher if the daemon is reachable.
		if pub := newBusPublisher(); pub != nil {
			storeOpts = append(storeOpts, pipeline.WithEventPublisher(pub))
		}

		result, err := pipeline.Run(cmd.Context(), p, mgr, "", storeOpts...)
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
