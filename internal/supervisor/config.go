// Package supervisor implements the gl1tch reactive supervisor — a background
// component that subscribes to busd events and dispatches intelligent handlers
// for error diagnosis, autonomous agent loops, and NL intent routing.
package supervisor

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// SupervisorConfig loaded from ~/.config/glitch/supervisor.yaml
type SupervisorConfig struct {
	Roles map[string]RoleConfig `yaml:"roles"`
}

// RoleConfig describes which provider and model should handle a named role.
type RoleConfig struct {
	Provider string `yaml:"provider"`
	Model    string `yaml:"model"`
}

// DefaultConfigPath returns the path to the supervisor config file.
func DefaultConfigPath(cfgDir string) string {
	return filepath.Join(cfgDir, "supervisor.yaml")
}

// LoadConfig reads <cfgDir>/supervisor.yaml and returns the parsed config.
// If the file does not exist, an empty (zero-value) config is returned — not an error.
func LoadConfig(cfgDir string) (*SupervisorConfig, error) {
	path := DefaultConfigPath(cfgDir)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &SupervisorConfig{Roles: make(map[string]RoleConfig)}, nil
		}
		return nil, fmt.Errorf("supervisor: read config %s: %w", path, err)
	}
	var cfg SupervisorConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("supervisor: parse config %s: %w", path, err)
	}
	if cfg.Roles == nil {
		cfg.Roles = make(map[string]RoleConfig)
	}
	return &cfg, nil
}
