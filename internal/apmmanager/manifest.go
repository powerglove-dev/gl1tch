package apmmanager

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// ApmDependency describes a single entry in the apm.yml dependencies.apm list.
// Supports both plain string form ("org/agent") and object form
// ("id: org/agent\npipeline: |...").
type ApmDependency struct {
	// ID is the APM dependency string (e.g. "8op-org/gl1tch").
	ID string
	// Pipeline is an optional pipeline YAML fragment. When non-empty, it is
	// materialized to ~/.config/glitch/pipelines/apm.<name>.pipeline.yaml on install.
	Pipeline string
}

// UnmarshalYAML implements yaml.Unmarshaler to handle both string and map forms.
func (d *ApmDependency) UnmarshalYAML(value *yaml.Node) error {
	switch value.Kind {
	case yaml.ScalarNode:
		d.ID = value.Value
		return nil
	case yaml.MappingNode:
		var obj struct {
			ID       string `yaml:"id"`
			Pipeline string `yaml:"pipeline"`
		}
		if err := value.Decode(&obj); err != nil {
			return fmt.Errorf("apm dependency: %w", err)
		}
		d.ID = obj.ID
		d.Pipeline = obj.Pipeline
		return nil
	default:
		return fmt.Errorf("apm dependency: unexpected YAML node kind %v", value.Kind)
	}
}

// ApmManifest is the parsed representation of apm.yml.
type ApmManifest struct {
	Name         string          `yaml:"name"`
	Dependencies apmDependencies `yaml:"dependencies"`
}

// apmDependencies holds the apm and mcp subsections.
type apmDependencies struct {
	APM []ApmDependency `yaml:"apm"`
}

// LoadApmManifest reads and parses the apm.yml file at projectRoot.
// Returns an empty manifest (no error) if the file does not exist.
func LoadApmManifest(projectRoot string) (*ApmManifest, error) {
	path := filepath.Join(projectRoot, "apm.yml")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &ApmManifest{}, nil
		}
		return nil, fmt.Errorf("load apm manifest: read %s: %w", path, err)
	}
	var m ApmManifest
	if err := yaml.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("load apm manifest: parse %s: %w", path, err)
	}
	return &m, nil
}

// PipelineStanza returns the pipeline YAML fragment for the given agent ID,
// or "" if no stanza is declared.
func (m *ApmManifest) PipelineStanza(agentID string) string {
	for _, dep := range m.Dependencies.APM {
		if dep.ID == agentID {
			return dep.Pipeline
		}
	}
	return ""
}
