package pluginmanager

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v3"
)

// Installer handles the full lifecycle of plugin install/remove/list.
type Installer struct {
	// WrappersDir is where sidecar YAMLs are written (~/.config/glitch/wrappers).
	WrappersDir string
	// RegistryDir is where plugins.yaml is stored (~/.config/glitch).
	RegistryDir string
}

// NewInstaller creates an Installer using standard glitch config paths.
func NewInstaller(configDir string) *Installer {
	return &Installer{
		WrappersDir: filepath.Join(configDir, "wrappers"),
		RegistryDir: configDir,
	}
}

// InstallResult is returned after a successful install.
type InstallResult struct {
	Plugin     InstalledPlugin
	BinaryPath string
}

// Install fetches the manifest, installs the binary, writes the sidecar YAML,
// and records the plugin in the registry.
func (inst *Installer) Install(ctx context.Context, rawRef string) (*InstallResult, error) {
	ref, err := ParseRef(rawRef)
	if err != nil {
		return nil, err
	}

	fmt.Fprintf(os.Stderr, "fetching manifest for %s...\n", ref)
	manifest, err := FetchManifest(ctx, ref)
	if err != nil {
		return nil, err
	}

	// Check for existing install (allow re-install / upgrade).
	registry, err := LoadRegistry(inst.RegistryDir)
	if err != nil {
		return nil, err
	}

	fmt.Fprintf(os.Stderr, "installing binary %q via %s...\n", manifest.Binary, manifest.Install.method())
	binPath, err := InstallBinary(ctx, ref, manifest)
	if err != nil {
		return nil, err
	}

	sidecarPath, err := inst.writeSidecar(manifest, binPath)
	if err != nil {
		return nil, err
	}

	version := manifest.Version
	if version == "" {
		version = ref.Version
	}

	entry := InstalledPlugin{
		Name:        manifest.Name,
		Source:      ref.Owner + "/" + ref.Repo,
		Version:     version,
		BinaryPath:  binPath,
		SidecarPath: sidecarPath,
		InstalledAt: time.Now().UTC(),
	}
	registry.Add(entry)
	if err := registry.Save(); err != nil {
		return nil, fmt.Errorf("save registry: %w", err)
	}

	return &InstallResult{Plugin: entry, BinaryPath: binPath}, nil
}

// Remove unregisters a plugin: deletes its sidecar YAML and registry entry.
// The binary itself is not removed (it may be on GOPATH/bin or ~/.local/bin).
func (inst *Installer) Remove(name string) error {
	registry, err := LoadRegistry(inst.RegistryDir)
	if err != nil {
		return err
	}
	entry, ok := registry.Get(name)
	if !ok {
		return fmt.Errorf("plugin %q is not installed", name)
	}

	if entry.SidecarPath != "" {
		if err := os.Remove(entry.SidecarPath); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("remove sidecar: %w", err)
		}
	}

	registry.Remove(name)
	return registry.Save()
}

// List returns all installed plugins from the registry.
func (inst *Installer) List() ([]InstalledPlugin, error) {
	registry, err := LoadRegistry(inst.RegistryDir)
	if err != nil {
		return nil, err
	}
	return registry.Plugins, nil
}

// writeSidecar generates and writes a sidecar YAML from the plugin manifest.
// Returns the path to the written file.
func (inst *Installer) writeSidecar(m *PluginManifest, binPath string) (string, error) {
	if err := os.MkdirAll(inst.WrappersDir, 0o755); err != nil {
		return "", fmt.Errorf("create wrappers dir: %w", err)
	}

	sidecar := buildSidecarSchema(m, binPath)
	data, err := yaml.Marshal(sidecar)
	if err != nil {
		return "", fmt.Errorf("marshal sidecar for %q: %w", m.Name, err)
	}

	dest := filepath.Join(inst.WrappersDir, m.Name+".yaml")
	if err := os.WriteFile(dest, data, 0o644); err != nil {
		return "", fmt.Errorf("write sidecar %s: %w", dest, err)
	}
	return dest, nil
}

// sidecarOut mirrors executor.SidecarSchema for marshalling without importing that package.
type sidecarOut struct {
	Name         string      `yaml:"name"`
	Description  string      `yaml:"description,omitempty"`
	Command      string      `yaml:"command"`
	Args         []string    `yaml:"args,omitempty"`
	Category     string      `yaml:"category,omitempty"`
	Kind         string      `yaml:"kind,omitempty"`
	Daemon       bool        `yaml:"daemon,omitempty"`
	Display      string      `yaml:"display,omitempty"`
	InputSchema  string      `yaml:"input_schema,omitempty"`
	OutputSchema string      `yaml:"output_schema,omitempty"`
	Signals      []signalOut `yaml:"signals,omitempty"`
}

type signalOut struct {
	Topic   string `yaml:"topic"`
	Handler string `yaml:"handler"`
}

func buildSidecarSchema(m *PluginManifest, binPath string) sidecarOut {
	cmd := m.Sidecar.Command
	if cmd == "" {
		cmd = binPath // use full path so it works even if ~/.local/bin isn't on PATH yet
	}

	kind := m.Sidecar.Kind
	if kind == "" {
		kind = "tool"
	}

	desc := m.Sidecar.Description
	if desc == "" {
		desc = m.Description
	}

	var sigs []signalOut
	for _, s := range m.Sidecar.Signals {
		sigs = append(sigs, signalOut{Topic: s.Topic, Handler: s.Handler})
	}

	return sidecarOut{
		Name:         m.Name,
		Description:  desc,
		Command:      cmd,
		Args:         m.Sidecar.Args,
		Category:     m.Sidecar.Category,
		Kind:         kind,
		Daemon:       m.Sidecar.Daemon,
		Display:      m.Sidecar.Display,
		InputSchema:  m.Sidecar.InputSchema,
		OutputSchema: m.Sidecar.OutputSchema,
		Signals:      sigs,
	}
}
