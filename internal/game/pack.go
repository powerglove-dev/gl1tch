package game

import (
	_ "embed"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// PackWeights holds the tuneable coefficients for the XP formula.
// DefaultPackWeights reproduces the original hard-coded formula exactly.
type PackWeights struct {
	BaseMultiplier  float64            `yaml:"base_multiplier"`
	CacheBonusRate  float64            `yaml:"cache_bonus_rate"`
	SpeedBonusCap   int64              `yaml:"speed_bonus_cap"`
	SpeedBonusScale float64            `yaml:"speed_bonus_scale"`
	RetryPenalty    int64              `yaml:"retry_penalty"`
	StreakMultiplier float64            `yaml:"streak_multiplier"`
	ProviderWeights map[string]float64 `yaml:"provider_weights"`
	MUDXPEvents     map[string]int     `yaml:"mud_xp_events"`
}

// BountyContract is a time-limited challenge placed in a MUD room by the tuner.
type BountyContract struct {
	ID             string    `yaml:"id"`
	Description    string    `yaml:"description"`
	ObjectiveType  string    `yaml:"objective_type"`
	ObjectiveValue float64   `yaml:"objective_value"`
	XPReward       int       `yaml:"xp_reward"`
	RoomID         string    `yaml:"room_id"`
	ValidUntil     time.Time `yaml:"valid_until"`
}

// ReputationDecayConfig controls how MUD faction reputation decays during inactive days.
type ReputationDecayConfig struct {
	DecayPerDay   int `yaml:"decay_per_day"`
	Floor         int `yaml:"floor"`
	MaxDecayDays  int `yaml:"max_decay_days"`
}

// ICEEncounterConfig controls ICE encounter timeout behaviour.
type ICEEncounterConfig struct {
	TimeoutHours int `yaml:"timeout_hours"`
}

// DefaultReputationDecay returns sensible default decay settings.
func DefaultReputationDecay() ReputationDecayConfig {
	return ReputationDecayConfig{DecayPerDay: 2, Floor: 10, MaxDecayDays: 7}
}

// DefaultICEEncounterConfig returns a 24-hour timeout suitable for unattended runs.
func DefaultICEEncounterConfig() ICEEncounterConfig {
	return ICEEncounterConfig{TimeoutHours: 24}
}

// DefaultPackWeights returns coefficients that reproduce the original
// ComputeXP formula exactly:
//
//	base = outputTokens * (output/input ratio) * 10
//	cacheBonus = cacheReadTokens / 2
//	speedBonus = max(0, 1000 - durationMS/100)
//	retryPenalty = retryCount * 50
func DefaultPackWeights() PackWeights {
	return PackWeights{
		BaseMultiplier:  10.0,
		CacheBonusRate:  0.5,
		SpeedBonusCap:   1000,
		SpeedBonusScale: 0.01,
		RetryPenalty:    50,
		StreakMultiplier: 1.0,
		ProviderWeights: map[string]float64{},
	}
}

// GameWorldPack holds the prompts that drive the game engine.
type GameWorldPack struct {
	Name             string
	GameRules        string
	NarratorStyle    string
	Weights          PackWeights
	BountyContracts  []BountyContract
	ReputationDecay  ReputationDecayConfig
	ICEEncounter     ICEEncounterConfig
}

// ActiveBountyContracts returns contracts that have not yet expired.
func (p GameWorldPack) ActiveBountyContracts() []BountyContract {
	now := time.Now()
	var active []BountyContract
	for _, c := range p.BountyContracts {
		if c.ValidUntil.IsZero() || c.ValidUntil.After(now) {
			active = append(active, c)
		}
	}
	return active
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
		Name             string                `yaml:"name"`
		GameRules        string                `yaml:"game_rules"`
		NarratorStyle    string                `yaml:"narrator_style"`
		Weights          PackWeights           `yaml:"weights"`
		BountyContracts  []BountyContract      `yaml:"bounty_contracts"`
		ReputationDecay  ReputationDecayConfig `yaml:"reputation_decay"`
		ICEEncounter     ICEEncounterConfig    `yaml:"ice_encounter"`
	}
	if err := yaml.Unmarshal(defaultPackData, &raw); err != nil {
		return GameWorldPack{Name: "default", Weights: DefaultPackWeights(), ReputationDecay: DefaultReputationDecay(), ICEEncounter: DefaultICEEncounterConfig()}
	}
	w := raw.Weights
	if w.BaseMultiplier == 0 {
		w = DefaultPackWeights()
	} else if w.ProviderWeights == nil {
		w.ProviderWeights = map[string]float64{}
	}
	if w.MUDXPEvents == nil {
		w.MUDXPEvents = map[string]int{}
	}
	rd := raw.ReputationDecay
	if rd.DecayPerDay == 0 {
		rd = DefaultReputationDecay()
	}
	ice := raw.ICEEncounter
	if ice.TimeoutHours == 0 {
		ice = DefaultICEEncounterConfig()
	}
	pack := GameWorldPack{
		Name:            raw.Name,
		GameRules:       raw.GameRules,
		NarratorStyle:   raw.NarratorStyle,
		Weights:         w,
		BountyContracts: raw.BountyContracts,
		ReputationDecay: rd,
		ICEEncounter:    ice,
	}
	return pack
}

// TunedWorldPackLoader scans the local tuned-pack directory for a
// kind: game-world pack written by the tuner. Falls back to
// DefaultWorldPackLoader if none is found.
type TunedWorldPackLoader struct{}

// ActivePack returns the first tuned game-world pack on disk, or the default.
func (TunedWorldPackLoader) ActivePack() GameWorldPack {
	home, err := os.UserHomeDir()
	if err != nil {
		return DefaultWorldPackLoader{}.ActivePack()
	}
	packDir := filepath.Join(home, ".local", "share", "glitch", "agents")
	entries, err := os.ReadDir(packDir)
	if err != nil {
		return DefaultWorldPackLoader{}.ActivePack()
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".agent.md") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(packDir, entry.Name()))
		if err != nil {
			continue
		}
		pack, ok := parseGameWorldPack(data)
		if ok {
			return pack
		}
	}
	return DefaultWorldPackLoader{}.ActivePack()
}

// parseGameWorldPack extracts a GameWorldPack from a tuned pack file if its
// YAML frontmatter contains "kind: game-world".
func parseGameWorldPack(data []byte) (GameWorldPack, bool) {
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
		Kind             string                `yaml:"kind"`
		Name             string                `yaml:"name"`
		GameRules        string                `yaml:"game_rules"`
		NarratorStyle    string                `yaml:"narrator_style"`
		Weights          PackWeights           `yaml:"weights"`
		BountyContracts  []BountyContract      `yaml:"bounty_contracts"`
		ReputationDecay  ReputationDecayConfig `yaml:"reputation_decay"`
		ICEEncounter     ICEEncounterConfig    `yaml:"ice_encounter"`
	}
	if err := yaml.Unmarshal([]byte(frontmatter), &fm); err != nil {
		return GameWorldPack{}, false
	}
	if fm.Kind != "game-world" {
		return GameWorldPack{}, false
	}
	w := fm.Weights
	if w.BaseMultiplier == 0 {
		w = DefaultPackWeights()
	} else if w.ProviderWeights == nil {
		w.ProviderWeights = map[string]float64{}
	}
	if w.MUDXPEvents == nil {
		w.MUDXPEvents = map[string]int{}
	}
	rd := fm.ReputationDecay
	if rd.DecayPerDay == 0 {
		rd = DefaultReputationDecay()
	}
	ice := fm.ICEEncounter
	if ice.TimeoutHours == 0 {
		ice = DefaultICEEncounterConfig()
	}
	return GameWorldPack{
		Name:            fm.Name,
		GameRules:       fm.GameRules,
		NarratorStyle:   fm.NarratorStyle,
		Weights:         w,
		BountyContracts: fm.BountyContracts,
		ReputationDecay: rd,
		ICEEncounter:    ice,
	}, true
}
