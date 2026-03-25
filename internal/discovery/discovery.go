package discovery

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// PluginType distinguishes native gRPC plugins from auto-detected CLI wrappers.
type PluginType int

const (
	TypeNative     PluginType = iota // implements OrcaiPlugin gRPC service
	TypeCLIWrapper                   // auto-detected tool in PATH
	TypePipeline                     // pipeline definition from *.pipeline.yaml
)

// Plugin describes a discovered plugin or CLI wrapper.
type Plugin struct {
	Name         string
	Command      string
	Args         []string
	Type         PluginType
	PipelineFile string
}

// knownCLITools is the built-in registry of AI CLI tools orcai auto-detects.
// Plugin name is derived directly from the Name field (usually matching the binary name).
// For multi-word commands like "gh copilot", Name is the logical alias ("copilot") and
// Args holds the subcommand so the name stays unique and predictable.
var knownCLITools = []Plugin{
	{Name: "claude",   Command: "claude"},
	{Name: "opencode", Command: "opencode"},
	{Name: "copilot",  Command: "gh", Args: []string{"copilot"}},
	{Name: "gemini",   Command: "gemini"},
	{Name: "aider",    Command: "aider"},
	{Name: "goose",    Command: "goose"},
}

// Discover returns all available plugins: Tier 1 (native, from configDir/plugins/),
// pipeline definitions (from configDir/pipelines/), and Tier 2 (CLI wrappers from PATH).
// Native plugins and pipelines shadow CLI wrappers of the same name.
func Discover(configDir string) ([]Plugin, error) {
	native, err := scanNative(filepath.Join(configDir, "plugins"))
	if err != nil {
		return nil, err
	}

	pipelines, err := scanPipelines(filepath.Join(configDir, "pipelines"))
	if err != nil {
		return nil, err
	}

	knownNames := make(map[string]bool, len(native)+len(pipelines))
	for _, p := range native {
		knownNames[p.Name] = true
	}
	for _, p := range pipelines {
		knownNames[p.Name] = true
	}

	plugins := append(native, pipelines...)
	for _, tool := range knownCLITools {
		if knownNames[tool.Name] {
			continue // native plugin or pipeline takes priority
		}
		if _, err := exec.LookPath(tool.Command); err == nil {
			t := tool
			t.Type = TypeCLIWrapper
			plugins = append(plugins, t)
		}
	}
	return plugins, nil
}

func scanNative(dir string) ([]Plugin, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var plugins []Plugin
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		info, err := e.Info()
		if err != nil || info.Mode()&0o111 == 0 {
			continue // not executable
		}
		plugins = append(plugins, Plugin{
			Name:    e.Name(),
			Command: filepath.Join(dir, e.Name()),
			Type:    TypeNative,
		})
	}
	return plugins, nil
}

func scanPipelines(dir string) ([]Plugin, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var plugins []Plugin
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".pipeline.yaml") {
			continue
		}
		name := strings.TrimSuffix(e.Name(), ".pipeline.yaml")
		fullPath := filepath.Join(dir, e.Name())
		plugins = append(plugins, Plugin{
			Name:         name,
			Command:      fullPath,
			Type:         TypePipeline,
			PipelineFile: fullPath,
		})
	}
	return plugins, nil
}
