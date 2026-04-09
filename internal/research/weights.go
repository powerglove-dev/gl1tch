package research

import (
	_ "embed"
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// weights.go is the externalized loader for the composite scoring
// weights. The Composite() function used to use hard-coded equal
// weights baked into Go; users who wanted to favour evidence_coverage
// over judge_score had to recompile. Now the weights live as YAML —
// embedded for first-impression defaults, overridable on disk for
// per-user tuning, re-read on every Composite() call so a tweaked
// file takes effect on the next research run with no restart.
//
// Same pattern as PromptStore (internal/research/prompt_store.go) and
// the researcher menu loader (internal/research/defaults.go) — embed,
// override, re-read. The brain stats engine eventually writes learned
// weights back to this file based on which signals predicted accept,
// closing the same self-improvement loop the prompts and researchers
// already have.

//go:embed weights.yaml
var embeddedWeightsYAML []byte

// Weights holds the per-signal weights the Composite function uses.
// Each weight is relative — the renderer normalizes against the sum
// of present weights so the values don't have to add up to 1.0. A
// weight of 0 means the signal is computed but not counted; missing
// signals (nil pointers in Score) are skipped entirely.
type Weights struct {
	CrossCapabilityAgreement float64 `yaml:"cross_capability_agreement"`
	EvidenceCoverage         float64 `yaml:"evidence_coverage"`
	JudgeScore               float64 `yaml:"judge_score"`
	SelfConsistency          float64 `yaml:"self_consistency"`
}

// weightsDoc is the on-disk YAML shape. Kept private — callers
// consume the flat Weights struct LoadWeights returns.
type weightsDoc struct {
	Weights Weights `yaml:"weights"`
}

// LoadWeights returns the canonical Weights struct. Resolution order:
//
//   1. ~/.config/glitch/weights.yaml (user override)
//   2. internal/research/weights.yaml (embedded default)
//
// Re-reads on every call (no caching) so a tuning loop is `vim
// ~/.config/glitch/weights.yaml` → `glitch threads new` → see the
// composite math change. Errors collapse to the embedded default so
// a malformed user file never breaks the loop.
func LoadWeights() (Weights, error) {
	if w, _, err := loadWeightsFromDisk(); err == nil {
		return w, nil
	}
	return parseWeightsYAML(embeddedWeightsYAML)
}

// loadWeightsFromDisk probes the user override path. Returns the
// parsed weights + the source path, or err when no override exists.
func loadWeightsFromDisk() (Weights, string, error) {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return Weights{}, "", os.ErrNotExist
	}
	path := filepath.Join(home, ".config", "glitch", "weights.yaml")
	data, err := os.ReadFile(path)
	if err != nil {
		return Weights{}, "", err
	}
	w, err := parseWeightsYAML(data)
	if err != nil {
		return Weights{}, path, err
	}
	return w, path, nil
}

// parseWeightsYAML decodes the embedded or override YAML body.
func parseWeightsYAML(data []byte) (Weights, error) {
	var doc weightsDoc
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return Weights{}, fmt.Errorf("research: parse weights yaml: %w", err)
	}
	return doc.Weights, nil
}

// WeightsOverridePath returns the absolute path the user override
// would live at. Used by `glitch weights edit` to figure out where
// to seed the file.
func WeightsOverridePath() string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return ""
	}
	return filepath.Join(home, ".config", "glitch", "weights.yaml")
}

// EmbeddedWeightsDefault returns the embedded default YAML
// regardless of any disk override. Used by `glitch weights edit`
// (which seeds the user override from this) and `glitch weights
// diff` (which compares against this).
func EmbeddedWeightsDefault() []byte {
	return append([]byte(nil), embeddedWeightsYAML...)
}

// ApplyWeights folds the per-signal Score values into a composite
// using the supplied weights. Missing signals (nil pointers) are
// skipped entirely — they contribute neither to the numerator nor
// the denominator. Weights of 0 are computed but not counted, which
// is the "turn off this signal but still log it" path.
//
// Composite() in score.go is the production caller and reads
// weights via LoadWeights on every invocation; tests can call
// ApplyWeights directly with a fixed Weights value for determinism.
func ApplyWeights(s Score, w Weights) float64 {
	var sum, totalWeight float64
	if s.SelfConsistency != nil && w.SelfConsistency > 0 {
		sum += *s.SelfConsistency * w.SelfConsistency
		totalWeight += w.SelfConsistency
	}
	if s.EvidenceCoverage != nil && w.EvidenceCoverage > 0 {
		sum += *s.EvidenceCoverage * w.EvidenceCoverage
		totalWeight += w.EvidenceCoverage
	}
	if s.CrossCapabilityAgree != nil && w.CrossCapabilityAgreement > 0 {
		sum += *s.CrossCapabilityAgree * w.CrossCapabilityAgreement
		totalWeight += w.CrossCapabilityAgreement
	}
	if s.JudgeScore != nil && w.JudgeScore > 0 {
		sum += *s.JudgeScore * w.JudgeScore
		totalWeight += w.JudgeScore
	}
	if totalWeight == 0 {
		return 0
	}
	return sum / totalWeight
}
