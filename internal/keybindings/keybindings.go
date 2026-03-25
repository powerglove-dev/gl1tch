// Package keybindings manages the orcai tmux keybinding configuration.
//
// A keybindings.yaml file maps key sequences to orcai action names. Apply
// binds each key to the resolved tmux command for that action.
package keybindings

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"

	"gopkg.in/yaml.v3"
)

// actionMap maps action names to tmux command args.
var actionMap = map[string][]string{
	"launch-session-picker": {"display-popup", "-E", "-w", "120", "-h", "40", "orcai-picker"},
	"open-sysop":            {"display-popup", "-E", "-w", "120", "-h", "40", "orcai-sysop"},
	"open-welcome":          {"new-window", "orcai-welcome"},
	"open-prompt-builder":   {"display-popup", "-E", "-w", "120", "-h", "40", "orcai", "pipeline-builder"},
	// Window management
	"new-window":  {"new-window"},
	"prev-window": {"previous-window"},
	"next-window": {"next-window"},
	// Pane splitting
	"split-pane-right": {"split-window", "-h"},
	"split-pane-down":  {"split-window", "-v"},
	"kill-pane":        {"kill-pane"},
	"kill-window":      {"kill-window"},
	// Pane navigation
	"select-pane-left":  {"select-pane", "-L"},
	"select-pane-right": {"select-pane", "-R"},
	"select-pane-up":    {"select-pane", "-U"},
	"select-pane-down":  {"select-pane", "-D"},
	// Pane resizing
	"resize-pane-left":  {"resize-pane", "-L", "5"},
	"resize-pane-right": {"resize-pane", "-R", "5"},
	"resize-pane-up":    {"resize-pane", "-U", "5"},
	"resize-pane-down":  {"resize-pane", "-D", "5"},
}

// Binding pairs a tmux key with an orcai action name.
type Binding struct {
	Key    string `yaml:"key"`
	Action string `yaml:"action"`
}

// Config is the parsed keybindings.yaml.
type Config struct {
	Bindings []Binding `yaml:"bindings"`
}

// LoadConfig reads keybindings.yaml from path. Returns empty config if file absent.
func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return &Config{}, nil
		}
		return nil, fmt.Errorf("keybindings: read %s: %w", path, err)
	}
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("keybindings: parse %s: %w", path, err)
	}
	return &cfg, nil
}

// Apply binds each key in cfg to its resolved tmux command. Unknown actions
// are logged to stdout and skipped.
func Apply(cfg *Config) error {
	for _, b := range cfg.Bindings {
		tmuxArgs, ok := actionMap[b.Action]
		if !ok {
			fmt.Printf("keybindings: unknown action %q for key %q, skipping\n", b.Action, b.Key)
			continue
		}
		args := append([]string{"bind-key", b.Key}, tmuxArgs...)
		if err := exec.Command("tmux", args...).Run(); err != nil {
			return fmt.Errorf("keybindings: bind %s → %s: %w", b.Key, b.Action, err)
		}
	}
	return nil
}
