package cmd

import (
	"bufio"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/8op-org/gl1tch/internal/assets"
)

func init() {
	rootCmd.AddCommand(configCmd)
	configCmd.AddCommand(configInitCmd)
}

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Manage glitch configuration",
}

var configInitCmd = &cobra.Command{
	Use:   "init",
	Short: "Write default config files and bundled executor wrappers",
	RunE:  runConfigInit,
}

const defaultLayoutYAML = `# glitch layout configuration
# Panes are created at session attach time. Remove this file to disable layout init.
# Available widgets: welcome, sysop, pipeline-builder
panes:
  - name: welcome
    widget: welcome
    position: right
    size: 50%
  - name: sysop
    widget: sysop
    position: right
    size: 40%
`

func runConfigInit(cmd *cobra.Command, args []string) error {
	cfgDir, err := glitchConfigDir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		return fmt.Errorf("config init: mkdir %s: %w", cfgDir, err)
	}

	// Write layout config.
	files := []struct {
		name    string
		content string
	}{
		{"layout.yaml", defaultLayoutYAML},
	}

	for _, f := range files {
		path := filepath.Join(cfgDir, f.name)
		if _, err := os.Stat(path); err == nil {
			if !confirm(fmt.Sprintf("%s already exists. Overwrite?", path)) {
				fmt.Printf("skipped %s\n", path)
				continue
			}
		}
		if err := os.WriteFile(path, []byte(f.content), 0o644); err != nil {
			return fmt.Errorf("config init: write %s: %w", path, err)
		}
		fmt.Printf("wrote %s\n", path)
	}

	// Write bundled executor wrapper YAMLs to ~/.config/glitch/wrappers/.
	wrappersDir := filepath.Join(cfgDir, "wrappers")
	if err := os.MkdirAll(wrappersDir, 0o755); err != nil {
		return fmt.Errorf("config init: mkdir wrappers: %w", err)
	}
	if err := writeEmbeddedDir(assets.WrappersFS, "wrappers", wrappersDir); err != nil {
		return fmt.Errorf("config init: write wrappers: %w", err)
	}

	return nil
}

// writeEmbeddedDir writes all files from an embedded FS subdirectory into dst.
// Existing files are skipped.
func writeEmbeddedDir(fsys fs.FS, srcDir, dst string) error {
	return writeEmbeddedDirFilter(fsys, srcDir, dst, "")
}

// writeEmbeddedDirFilter is like writeEmbeddedDir but skips files whose names
// don't end with suffix. Pass an empty suffix to write all files.
func writeEmbeddedDirFilter(fsys fs.FS, srcDir, dst, suffix string) error {
	entries, err := fs.ReadDir(fsys, srcDir)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if suffix != "" && !strings.HasSuffix(e.Name(), suffix) {
			continue
		}
		destPath := filepath.Join(dst, e.Name())
		if _, err := os.Stat(destPath); err == nil {
			fmt.Printf("skipped %s (already exists)\n", destPath)
			continue
		}
		data, err := fs.ReadFile(fsys, srcDir+"/"+e.Name())
		if err != nil {
			return err
		}
		if err := os.WriteFile(destPath, data, 0o644); err != nil {
			return err
		}
		fmt.Printf("wrote %s\n", destPath)
	}
	return nil
}

// confirm prompts the user with a yes/no question and returns true for yes.
func confirm(prompt string) bool {
	fmt.Printf("%s [y/N] ", prompt)
	scanner := bufio.NewScanner(os.Stdin)
	if scanner.Scan() {
		answer := strings.TrimSpace(strings.ToLower(scanner.Text()))
		return answer == "y" || answer == "yes"
	}
	return false
}
