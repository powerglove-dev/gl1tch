package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/8op-org/gl1tch/internal/assistant"
	"github.com/8op-org/gl1tch/internal/picker"
)

var (
	modelFlagJSON  bool
	modelFlagLocal bool
)

func init() {
	rootCmd.AddCommand(modelCmd)
	modelCmd.Flags().BoolVar(&modelFlagJSON, "json", false, `output as JSON: {"provider":"...","model":"..."}`)
	modelCmd.Flags().BoolVar(&modelFlagLocal, "local", false, "restrict to local providers only (ollama)")
}

var modelCmd = &cobra.Command{
	Use:   "model",
	Short: "Print the best available model as provider/model",
	Long: `Prints the best available model to stdout as "provider/model".

Reads ~/.config/glitch/.glitch_backend if present, then falls back to
live provider discovery. Exit code 1 if no model is available.

Designed for use in shell scripts and pipeline steps:

  GLITCH_MODEL=$(glitch model)
  GLITCH_MODEL=$(glitch model --local)`,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Fast path: read the user's persisted backend selection.
		if s := readBackendFile(); s != "" {
			if modelFlagLocal && !strings.HasPrefix(s, "ollama/") {
				// Fall through to live discovery for a local model.
			} else {
				return printModelStr(s)
			}
		}

		// Slow path: live provider discovery.
		for _, p := range picker.BuildProviders() {
			if modelFlagLocal && p.ID != "ollama" {
				continue
			}
			if len(p.Models) > 0 {
				return printModelStr(p.ID + "/" + p.Models[0].ID)
			}
		}

		// Ollama direct query as final fallback.
		if assistant.OllamaAvailable() {
			return printModelStr("ollama/" + assistant.BestOllamaModel())
		}

		return fmt.Errorf("no model available — is ollama running or a provider configured?")
	},
}

func readBackendFile() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	data, err := os.ReadFile(filepath.Join(home, ".config", "glitch", ".glitch_backend"))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

func printModelStr(providerModel string) error {
	parts := strings.SplitN(providerModel, "/", 2)
	provider, model := "", providerModel
	if len(parts) == 2 {
		provider, model = parts[0], parts[1]
	}
	if modelFlagJSON {
		fmt.Printf("{\"provider\":%q,\"model\":%q}\n", provider, model)
		return nil
	}
	fmt.Printf("%s/%s\n", provider, model)
	return nil
}
