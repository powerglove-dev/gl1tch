package cron

import (
	"errors"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Entry defines a single scheduled pipeline or agent run.
type Entry struct {
	// Name is a human-readable label for this schedule entry.
	Name string `yaml:"name"`
	// Schedule is a standard 5-field cron expression (minute hour dom month dow).
	Schedule string `yaml:"schedule"`
	// Kind is either "pipeline" or "agent".
	Kind string `yaml:"kind"`
	// Target is a file path (for pipeline) or agent name (for agent).
	Target string `yaml:"target"`
	// Args are optional key-value arguments passed to the target.
	Args map[string]any `yaml:"args"`
	// Timeout is an optional duration string, e.g. "5m". Zero means no timeout.
	Timeout string `yaml:"timeout"`
	// WorkingDir sets the working directory for the spawned subprocess.
	// When empty the subprocess inherits the daemon's working directory.
	WorkingDir string `yaml:"working_dir"`
}

// cronConfig is the top-level structure of cron.yaml.
type cronConfig struct {
	Entries []Entry `yaml:"entries"`
}

// LoadConfig reads ~/.config/orcai/cron.yaml and returns the configured entries.
// It returns an empty slice (and no error) if the file does not exist.
func LoadConfig() ([]Entry, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	return LoadConfigFrom(filepath.Join(home, ".config", "orcai", "cron.yaml"))
}

// LoadConfigFrom reads a cron config from the specified path and returns the
// configured entries. It returns an empty slice (and no error) if the file
// does not exist.
func LoadConfigFrom(path string) ([]Entry, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return []Entry{}, nil
		}
		return nil, err
	}

	var cfg cronConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}

	if cfg.Entries == nil {
		return []Entry{}, nil
	}
	return cfg.Entries, nil
}
