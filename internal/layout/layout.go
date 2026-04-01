// Package layout manages the glitch pane layout configuration.
//
// A layout.yaml file describes panes that should be created when a session
// is attached. Apply creates any missing panes and launches their widgets.
package layout

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"os"
	"os/exec"
	"strings"

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

func validPosition(p Position) bool {
	switch p {
	case PositionLeft, PositionRight, PositionTop, PositionBottom:
		return true
	}
	return false
}

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

// Apply creates panes and launches widgets according to cfg. Panes that already
// exist (by name) in the current tmux window are skipped. Invalid positions
// are logged and skipped.
func Apply(ctx context.Context, cfg *Config, d widgetdispatch.Dispatcher) error {
	for _, pane := range cfg.Panes {
		if !validPosition(pane.Position) {
			log.Printf("layout: pane %q has invalid position %q, skipping", pane.Name, pane.Position)
			continue
		}
		if paneExists(pane.Name) {
			continue
		}
		if err := createPane(pane); err != nil {
			log.Printf("layout: create pane %q: %v", pane.Name, err)
			continue
		}
		if err := d.Dispatch(ctx, pane.Widget, widgetdispatch.Options{}); err != nil {
			log.Printf("layout: dispatch widget %q for pane %q: %v", pane.Widget, pane.Name, err)
		}
	}
	return nil
}

// paneExists returns true if a tmux pane named `name` exists in the current window.
func paneExists(name string) bool {
	out, err := exec.Command("tmux", "list-panes", "-F", "#{pane_title}").Output()
	if err != nil {
		return false
	}
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line == name {
			return true
		}
	}
	return false
}

// createPane creates a new tmux pane in the given position with the given size.
func createPane(pane Pane) error {
	splitFlag := "-h" // horizontal split → left/right
	if pane.Position == PositionTop || pane.Position == PositionBottom {
		splitFlag = "-v"
	}
	args := []string{"split-window", splitFlag, "-l", pane.Size, "-d"}
	if pane.Position == PositionLeft || pane.Position == PositionTop {
		args = append(args, "-b") // before current pane
	}
	args = append(args, "-P", "-F", "#{pane_id}") // print new pane ID
	cmd := exec.Command("tmux", args...)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("tmux split-window: %w", err)
	}
	return nil
}
