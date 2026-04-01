package game

import (
	_ "embed"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// GameWorldPack holds the prompts that drive the game engine.
type GameWorldPack struct {
	Name          string
	GameRules     string
	NarratorStyle string
}

// WorldPackLoader resolves the active game world pack.
type WorldPackLoader interface {
	ActivePack() GameWorldPack
}

//go:embed packs/cyberspace/pack.yaml
var defaultPackData []byte

// DefaultWorldPackLoader returns the embedded cyberspace pack.
type DefaultWorldPackLoader struct{}

// ActivePack parses and returns the embedded default pack.
func (DefaultWorldPackLoader) ActivePack() GameWorldPack {
	var raw struct {
		Name          string `yaml:"name"`
		GameRules     string `yaml:"game_rules"`
		NarratorStyle string `yaml:"narrator_style"`
	}
	if err := yaml.Unmarshal(defaultPackData, &raw); err != nil {
		return GameWorldPack{Name: "default"}
	}
	return GameWorldPack{
		Name:          raw.Name,
		GameRules:     raw.GameRules,
		NarratorStyle: raw.NarratorStyle,
	}
}

// APMWorldPackLoader scans the APM agent directory for a kind: game-world
// agent. Falls back to DefaultWorldPackLoader if none is found.
type APMWorldPackLoader struct{}

// ActivePack returns the first installed game-world pack, or the default.
func (APMWorldPackLoader) ActivePack() GameWorldPack {
	home, err := os.UserHomeDir()
	if err != nil {
		return DefaultWorldPackLoader{}.ActivePack()
	}
	agentDir := filepath.Join(home, ".local", "share", "glitch", "agents")
	entries, err := os.ReadDir(agentDir)
	if err != nil {
		return DefaultWorldPackLoader{}.ActivePack()
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".agent.md") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(agentDir, entry.Name()))
		if err != nil {
			continue
		}
		pack, ok := parseGameWorldAgent(data)
		if ok {
			return pack
		}
	}
	return DefaultWorldPackLoader{}.ActivePack()
}

// parseGameWorldAgent extracts a GameWorldPack from an .agent.md file if its
// YAML frontmatter contains "kind: game-world".
func parseGameWorldAgent(data []byte) (GameWorldPack, bool) {
	content := string(data)
	// Strip leading BOM or whitespace.
	content = strings.TrimSpace(content)
	if !strings.HasPrefix(content, "---") {
		return GameWorldPack{}, false
	}
	// Extract frontmatter between first and second "---".
	rest := content[3:]
	idx := strings.Index(rest, "\n---")
	if idx < 0 {
		return GameWorldPack{}, false
	}
	frontmatter := rest[:idx]

	var fm struct {
		Kind          string `yaml:"kind"`
		Name          string `yaml:"name"`
		GameRules     string `yaml:"game_rules"`
		NarratorStyle string `yaml:"narrator_style"`
	}
	if err := yaml.Unmarshal([]byte(frontmatter), &fm); err != nil {
		return GameWorldPack{}, false
	}
	if fm.Kind != "game-world" {
		return GameWorldPack{}, false
	}
	return GameWorldPack{
		Name:          fm.Name,
		GameRules:     fm.GameRules,
		NarratorStyle: fm.NarratorStyle,
	}, true
}
