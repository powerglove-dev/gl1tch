package research

import (
	_ "embed"
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

//go:embed smoke_pack.yaml
var embeddedSmokePackYAML []byte

// SmokeFixture is one curated question the smoke runner exercises
// against a target. Each fixture has the question, the picks the
// planner SHOULD return for it, and an optional list of substrings
// that must appear in the gathered evidence (for repo-specific
// fixtures with known-stable identifiers).
type SmokeFixture struct {
	Name                   string   `yaml:"name"`
	Question               string   `yaml:"question"`
	ExpectedPicks          []string `yaml:"expected_picks"`
	ExpectedEvidenceSubstr []string `yaml:"expected_evidence_substr,omitempty"`
}

// smokePackDoc is the on-disk YAML shape.
type smokePackDoc struct {
	Fixtures []SmokeFixture `yaml:"fixtures"`
}

// LoadSmokePack returns the canonical fixture list. Resolution:
//
//   1. ~/.config/glitch/smoke_pack.yaml (user override)
//   2. internal/research/smoke_pack.yaml (embedded default)
//
// Re-reads on every call so adding a fixture is just a YAML edit.
func LoadSmokePack() ([]SmokeFixture, error) {
	if fixtures, err := loadSmokePackFromDisk(); err == nil && len(fixtures) > 0 {
		return fixtures, nil
	}
	return parseSmokePackYAML(embeddedSmokePackYAML)
}

func loadSmokePackFromDisk() ([]SmokeFixture, error) {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return nil, os.ErrNotExist
	}
	path := filepath.Join(home, ".config", "glitch", "smoke_pack.yaml")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return parseSmokePackYAML(data)
}

func parseSmokePackYAML(data []byte) ([]SmokeFixture, error) {
	var doc smokePackDoc
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("research: parse smoke pack yaml: %w", err)
	}
	return doc.Fixtures, nil
}

// SmokePackOverridePath returns the absolute path the user override
// file would live at.
func SmokePackOverridePath() string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return ""
	}
	return filepath.Join(home, ".config", "glitch", "smoke_pack.yaml")
}

// EmbeddedSmokePackDefault returns the embedded default YAML body.
func EmbeddedSmokePackDefault() []byte {
	return append([]byte(nil), embeddedSmokePackYAML...)
}
