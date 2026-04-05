// Package layout manages the glitch pane layout configuration.
//
// A layout.yaml file describes panes that should be created when a session
// is attached. Apply is now a no-op (tmux has been removed); the package is
// retained for LoadConfig compatibility with existing user config files.
package layout

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"

	"github.com/8op-org/gl1tch/internal/widgetdispatch"
	"gopkg.in/yaml.v3"
)

// Position values for pane placement.
type Position string

const (
	PositionLeft   Position = "left"
	PositionRight  Position = "right"
	PositionTop    Position = "top"
	PositionBottom Position = "bottom"
)

// Pane describes a single widget pane in the layout.
type Pane struct {
	Name     string   `yaml:"name"`
	Widget   string   `yaml:"widget"`
	Position Position `yaml:"position"`
	Size     string   `yaml:"size"`
}

// Config is the parsed layout.yaml.
type Config struct {
	Panes []Pane `yaml:"panes"`
}

// LoadConfig reads layout.yaml from path. Returns an empty Config (no panes) if
// the file does not exist — this is not an error.
func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return &Config{}, nil
		}
		return nil, fmt.Errorf("layout: read %s: %w", path, err)
	}
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("layout: parse %s: %w", path, err)
	}
	return &cfg, nil
}

// Apply is a no-op. The layout system previously created tmux panes; since
// tmux has been removed from gl1tch, this function does nothing.
func Apply(_ context.Context, _ *Config, _ widgetdispatch.Dispatcher) error {
	return nil
}
