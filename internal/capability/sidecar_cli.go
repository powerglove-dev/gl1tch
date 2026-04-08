package capability

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// LoadCliSidecar loads an existing-style CliAdapter sidecar YAML (the format
// currently used by ~/.config/glitch/wrappers/<name>.yaml and
// plugins/<name>/<name>.yaml) and adapts it to a Capability. It is the bridge
// that lets every existing AI provider plugin keep working under the unified
// runner without rewriting their sidecar files.
//
// Semantics: a CliAdapter sidecar always becomes an on-demand capability with
// stream sink and parser=raw. That is the literal behaviour of the existing
// CliAdapter — write input to stdin, stream stdout to the writer. The new
// capability package gains nothing by inventing different defaults for these.
//
// Once the migration to skill markdown files is complete, this loader can be
// deleted in favour of LoadSkillsFromDir. It exists so phase 1 doesn't have
// to convert every plugin file at the same time.
func LoadCliSidecar(path string) (Capability, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("sidecar: read %s: %w", path, err)
	}
	var s cliSidecarSchema
	if err := yaml.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("sidecar: parse %s: %w", path, err)
	}
	if s.Name == "" {
		return nil, fmt.Errorf("sidecar: %s: name is required", path)
	}
	if s.Command == "" {
		return nil, fmt.Errorf("sidecar: %s: command is required", path)
	}
	return &scriptCapability{
		manifest: Manifest{
			Name:        s.Name,
			Description: s.Description,
			Category:    s.Category,
			Trigger:     Trigger{Mode: TriggerOnDemand},
			Sink:        Sink{Stream: true},
			Invocation: Invocation{
				Command: s.Command,
				Args:    s.Args,
				Parser:  ParserRaw,
			},
		},
	}, nil
}

// LoadCliSidecarsFromDir scans dir for *.yaml files and loads each. Per-file
// errors are returned alongside successfully loaded capabilities.
func LoadCliSidecarsFromDir(dir string) ([]Capability, []error) {
	matches, err := filepath.Glob(filepath.Join(dir, "*.yaml"))
	if err != nil {
		return nil, []error{err}
	}
	var caps []Capability
	var errs []error
	for _, m := range matches {
		c, err := LoadCliSidecar(m)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		caps = append(caps, c)
	}
	return caps, errs
}

// cliSidecarSchema is the subset of the existing executor.SidecarSchema that
// the capability adapter cares about. The full schema (Models, Mode, Signals,
// AgentLoop, etc.) carries TUI- and bus-specific fields that have no place in
// the unified runner — those concerns belong to whatever assistant front-end
// consumes the capability, not to the capability primitive itself.
type cliSidecarSchema struct {
	Name        string   `yaml:"name"`
	Description string   `yaml:"description"`
	Command     string   `yaml:"command"`
	Args        []string `yaml:"args"`
	Category    string   `yaml:"category"`
}
