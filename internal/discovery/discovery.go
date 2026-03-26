package discovery

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/adam-stokes/orcai/internal/providers"
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
	// SidecarPath is the path to the sidecar YAML for TypeCLIWrapper plugins
	// discovered from the wrappers directory. Empty for other types.
	SidecarPath string
}

// Discover returns all available plugins: Tier 1 (native, from configDir/plugins/),
// pipeline definitions (from configDir/pipelines/), and Tier 2 (CLI wrappers from PATH).
// Native plugins and pipelines shadow CLI wrappers of the same name.
// CLI wrappers are sourced from the providers.Registry (bundled + user-defined profiles).
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

	// Scan wrappers dir for sidecar-declared CLI plugins; check command in PATH.
	// These take priority over providers.Registry entries of the same name.
	wrappers, err := scanWrappers(filepath.Join(configDir, "wrappers"), knownNames)
	if err != nil {
		return nil, err
	}
	for _, w := range wrappers {
		knownNames[w.Name] = true
	}

	reg, err := providers.NewRegistry(filepath.Join(configDir, "providers"))
	if err != nil {
		return nil, err
	}

	plugins := append(native, pipelines...)
	plugins = append(plugins, wrappers...)
	for _, profile := range reg.Available() {
		if knownNames[profile.Name] {
			continue // native plugin, pipeline, or sidecar wrapper takes priority
		}
		plugins = append(plugins, Plugin{
			Name:    profile.Name,
			Command: profile.Binary,
			Args:    profile.Session.LaunchArgs,
			Type:    TypeCLIWrapper,
		})
	}
	return plugins, nil
}

// sidecarHeader holds just the fields needed to determine name and command.
type sidecarHeader struct {
	Name    string `yaml:"name"`
	Command string `yaml:"command"`
}

// scanWrappers reads YAML files from dir, checks that each command is in PATH,
// and returns Plugin entries (TypeCLIWrapper) for those that are available.
// Names already in knownNames are skipped to let higher-priority types shadow them.
func scanWrappers(dir string, knownNames map[string]bool) ([]Plugin, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var plugins []Plugin
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".yaml") {
			continue
		}
		path := filepath.Join(dir, e.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		var hdr sidecarHeader
		if err := yaml.Unmarshal(data, &hdr); err != nil || hdr.Name == "" || hdr.Command == "" {
			continue
		}
		if knownNames[hdr.Name] {
			continue
		}
		if _, err := exec.LookPath(hdr.Command); err != nil {
			continue // command not installed
		}
		plugins = append(plugins, Plugin{
			Name:        hdr.Name,
			Command:     hdr.Command,
			Type:        TypeCLIWrapper,
			SidecarPath: path,
		})
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
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".pipeline.yaml") || strings.HasPrefix(e.Name(), ".") {
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
