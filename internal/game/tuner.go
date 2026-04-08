package game

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"

	"github.com/8op-org/gl1tch/internal/store"
	"gopkg.in/yaml.v3"
)

// streakMilestones lists the streak-day values that trigger an auto-tune.
var streakMilestones = []int{3, 7, 14, 30, 60, 90}

// TunerState persists the last-tuned timestamp and run counter to a flat JSON
// file at ~/.local/share/glitch/game-tuner-state.json.
type TunerState struct {
	LastTunedAt   time.Time `json:"last_tuned_at"`
	RunsSinceTune int       `json:"runs_since_tune"`
}

// Tuner drives the self-evolving game pack cycle. It reads behavioral stats
// from SQLite, calls local Ollama twice (analyze → generate), validates the
// output, and writes the evolved pack to the tuned-pack directory.
type Tuner struct {
	st         *store.Store
	engine     *GameEngine
	packLoader WorldPackLoader
	agentPath  string
	statePath  string
}

// NewTuner creates a Tuner that writes to
// ~/.local/share/glitch/agents/game-world-tuned.agent.md.
func NewTuner(st *store.Store, engine *GameEngine, loader WorldPackLoader) *Tuner {
	home, _ := os.UserHomeDir()
	glitchDir := filepath.Join(home, ".local", "share", "glitch")
	return &Tuner{
		st:         st,
		engine:     engine,
		packLoader: loader,
		agentPath:  filepath.Join(glitchDir, "agents", "game-world-tuned.agent.md"),
		statePath:  filepath.Join(glitchDir, "game-tuner-state.json"),
	}
}

// LoadState reads the tuner state file. Returns zero-value on any error.
func (t *Tuner) LoadState() TunerState {
	data, err := os.ReadFile(t.statePath)
	if err != nil {
		return TunerState{}
	}
	var s TunerState
	if err := json.Unmarshal(data, &s); err != nil {
		return TunerState{}
	}
	return s
}

// SaveState writes the tuner state file.
func (t *Tuner) SaveState(s TunerState) error {
	data, err := json.Marshal(s)
	if err != nil {
		return fmt.Errorf("tuner: marshal state: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(t.statePath), 0o755); err != nil {
		return fmt.Errorf("tuner: mkdir state dir: %w", err)
	}
	return os.WriteFile(t.statePath, data, 0o644)
}

// IncrementRuns increments the runs_since_tune counter in the state file.
func (t *Tuner) IncrementRuns() {
	s := t.LoadState()
	s.RunsSinceTune++
	_ = t.SaveState(s)
}

// ShouldTune returns true when at least one trigger condition is met and the
// 1-day cooldown has not fired.
//
// Trigger conditions:
//   - level-up (prevLevel < newLevel)
//   - streak milestone (newStreak in {3,7,14,30,60,90})
//   - new achievement unlocked (len(newAchievements) > 0)
//   - 7-day floor: ≥7 days since last tune AND ≥5 runs since last tune
func (t *Tuner) ShouldTune(
	now time.Time,
	state TunerState,
	prevLevel, newLevel int,
	prevStreak, newStreak int,
	newAchievements []string,
) bool {
	// 1-day cooldown: don't tune if already tuned today.
	if !state.LastTunedAt.IsZero() {
		lastDate := state.LastTunedAt.Truncate(24 * time.Hour)
		nowDate := now.Truncate(24 * time.Hour)
		if lastDate.Equal(nowDate) {
			return false
		}
	}

	// Level-up trigger.
	if newLevel > prevLevel {
		return true
	}

	// Streak milestone trigger.
	for _, m := range streakMilestones {
		if newStreak == m {
			return true
		}
	}

	// New achievement trigger.
	if len(newAchievements) > 0 {
		return true
	}

	// 7-day floor trigger.
	if !state.LastTunedAt.IsZero() {
		daysSince := now.Sub(state.LastTunedAt).Hours() / 24
		if daysSince >= 7 && state.RunsSinceTune >= 5 {
			return true
		}
	} else if state.RunsSinceTune >= 5 {
		// Never tuned before — 5+ runs is enough.
		return true
	}

	return false
}

// analysisResult is the structured JSON returned by the first Ollama call.
type analysisResult struct {
	Calibrations      []calibration         `json:"calibrations"`
	NewAchievements   []newAchievement      `json:"new_achievements"`
	ICERules          iceRules              `json:"ice_rules"`
	QuestRules        []string              `json:"quest_rules"`
	NarratorNotes     string                `json:"narrator_notes"`
	WeightSuggestions map[string]any        `json:"weight_suggestions"`
	BountyContracts   []analysisContract    `json:"bounty_contracts"`
}

type analysisContract struct {
	ID             string  `json:"id"`
	Description    string  `json:"description"`
	ObjectiveType  string  `json:"objective_type"`
	ObjectiveValue float64 `json:"objective_value"`
	XPReward       int     `json:"xp_reward"`
	RoomID         string  `json:"room_id"`
	ValidDays      int     `json:"valid_days"`
}

type calibration struct {
	ID                 string  `json:"id"`
	Issue              string  `json:"issue"`
	SuggestedThreshold float64 `json:"suggested_threshold"`
}

type newAchievement struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Rule string `json:"rule"`
}

type iceRules struct {
	TraceICE string `json:"trace_ice"`
	DataICE  string `json:"data_ice"`
}

// buildAnalysisPrompt constructs the first Ollama prompt.
func (t *Tuner) buildAnalysisPrompt(stats store.GameStats, pack GameWorldPack) string {
	statsJSON, _ := json.MarshalIndent(stats, "", "  ")
	return fmt.Sprintf(`You are a game designer analyzing a player's usage patterns for a cyberpunk text MUD called The Gibson.

Current game rules:
%s

Player behavioral stats (last 30 days):
%s

Analyze the current rules against the stats and return ONLY a valid JSON object with these keys:
- calibrations: array of {id, issue, suggested_threshold} for achievements that need threshold adjustments
- new_achievements: array of {id, name, rule} for new achievements worth adding (based on observed patterns)
- ice_rules: {trace_ice: "condition string", data_ice: "condition string"} with calibrated thresholds
- quest_rules: array of quest event trigger condition strings
- narrator_notes: string describing how narrator should evolve given player arc
- weight_suggestions: object with any of {base_multiplier, cache_bonus_rate, speed_bonus_cap, speed_bonus_scale, retry_penalty, streak_multiplier} that should change
- bounty_contracts: array of 3-5 {id, description, objective_type, objective_value, xp_reward, room_id, valid_days} contracts
  - id: kebab-case unique string (e.g. "cache-blitz-7d")
  - description: 1-sentence player-facing objective
  - objective_type: one of "cache_ratio", "speed_ms", "run_streak", "cost_zero", "output_ratio"
  - objective_value: numeric threshold the player must hit
  - xp_reward: integer bonus XP (100-2000, scaled to difficulty)
  - room_id: one of "mainframe", "bazaar", "cryovault", "ghost-net", "ice-wall"
  - valid_days: how many days until contract expires (1-14)

Return ONLY valid JSON. No explanation. No markdown.`,
		pack.GameRules, string(statsJSON))
}

// buildGenerationPrompt constructs the second Ollama prompt.
func (t *Tuner) buildGenerationPrompt(analysisJSON string, pack GameWorldPack) string {
	weightsJSON, _ := json.MarshalIndent(pack.Weights, "", "  ")
	return fmt.Sprintf(`You are a game designer for a cyberpunk text MUD called The Gibson (gl1tch).
Evolve the existing game pack based on the analysis. Output a complete YAML frontmatter block.

Analysis:
%s

Current game_rules (PRESERVE ALL existing achievement IDs — only add, never remove):
%s

Current weights:
%s

CRITICAL YAML RULES — violating these will break the parser:
1. game_rules and narrator_style MUST use the | block scalar (plain indented text, NOT nested YAML).
2. game_rules content is a plain text prompt. Achievement definitions are plain text lines, not YAML keys.
3. weights fields are plain numbers — no nested keys except provider_weights.
4. provider_weights maps provider names to float multipliers ONLY.
   Valid keys: providers.claude, providers.ollama, providers.codex
   Valid values: floats like 1.2
   Use provider_weights: {} if nothing to add.
   NEVER put achievement IDs or rules in provider_weights.

Instructions:
- KEEP every existing achievement ID from current game_rules above — do not drop any
- Add 1-2 new achievements from the analysis new_achievements list
- Apply calibrated thresholds from analysis calibrations
- Keep trace-ice (cost_usd threshold) and data-ice (input_tokens threshold) rules
- Evolve narrator_style using the analysis narrator_notes
- Apply weight_suggestions; keep multiplier values in range [0.1, 5.0]
- Include bounty_contracts from the analysis as a YAML list under the key bounty_contracts.
  Each contract: id, description, objective_type, objective_value, xp_reward, room_id, valid_until (ISO8601 date)
  Compute valid_until by adding valid_days to today's date.

Output ONLY the YAML frontmatter, starting with --- and ending with ---. No explanation. No markdown fences.`,
		analysisJSON, pack.GameRules, string(weightsJSON))
}

// inferTrigger returns the primary reason ShouldTune fired, for observability.
func inferTrigger(score GameRunScoredPayload) string {
	if score.Level > score.PrevLevel {
		return "level_up"
	}
	for _, m := range streakMilestones {
		if score.StreakDays == m {
			return "streak_milestone"
		}
	}
	if len(score.Achievements) > 0 {
		return "achievement"
	}
	return "floor"
}

// Tune runs the full analyze → generate → validate → install cycle.
// It is safe to call from a goroutine; all errors are logged, not returned.
func (t *Tuner) Tune(ctx context.Context, stats store.GameStats, score GameRunScoredPayload) error {
	trigger := inferTrigger(score)
	RecordTunerInvoked(ctx, trigger)

	ctx, span := otel.Tracer("gl1tch/game").Start(ctx, "game.tuner")
	span.SetAttributes(
		attribute.String("game.trigger", trigger),
		attribute.Int("game.level", score.Level),
		attribute.Int("game.streak_days", score.StreakDays),
	)
	defer span.End()

	pack := t.packLoader.ActivePack()

	// ── Step 1: analysis call ─────────────────────────────────────────────────
	analysisPrompt := t.buildAnalysisPrompt(stats, pack)
	analysisRaw := t.engine.Respond(ctx, analysisPrompt, "Analyze and return JSON.")
	if analysisRaw == "" {
		return fmt.Errorf("tuner: analysis call returned empty response")
	}

	// Parse analysis JSON; retry once with stricter prompt on failure.
	var analysis analysisResult
	if err := parseJSON(analysisRaw, &analysis); err != nil {
		strictMsg := "Return ONLY valid JSON. No explanation. No markdown. No code fences."
		analysisRaw2 := t.engine.Respond(ctx, analysisPrompt, strictMsg)
		if err2 := parseJSON(analysisRaw2, &analysis); err2 != nil {
			return fmt.Errorf("tuner: analysis parse failed after retry: %w", err2)
		}
		analysisRaw = analysisRaw2
	}

	// ── Step 2: generation call ───────────────────────────────────────────────
	genPrompt := t.buildGenerationPrompt(analysisRaw, pack)
	genRaw := t.engine.Respond(ctx, genPrompt, "Generate the evolved pack YAML frontmatter.")
	if genRaw == "" {
		return fmt.Errorf("tuner: generation call returned empty response")
	}

	// Extract YAML frontmatter from the response.
	yamlBytes, err := extractFrontmatter(genRaw)
	if err != nil {
		return fmt.Errorf("tuner: extract frontmatter: %w", err)
	}

	// ── Step 3: validate ──────────────────────────────────────────────────────
	if err := t.validate(yamlBytes); err != nil {
		log.Printf("[DEBUG] tuner: validation failed: %v\nraw YAML:\n%s", err, string(yamlBytes))
		return fmt.Errorf("tuner: validate: %w", err)
	}

	// ── Step 4: write agent file ──────────────────────────────────────────────
	agentContent := "---\n" + string(yamlBytes) + "\n---\n"
	if err := os.MkdirAll(filepath.Dir(t.agentPath), 0o755); err != nil {
		return fmt.Errorf("tuner: mkdir agents: %w", err)
	}
	if err := os.WriteFile(t.agentPath, []byte(agentContent), 0o644); err != nil {
		return fmt.Errorf("tuner: write agent file: %w", err)
	}

	// ── Step 5: update state ──────────────────────────────────────────────────
	state := t.LoadState()
	state.LastTunedAt = time.Now()
	state.RunsSinceTune = 0
	_ = t.SaveState(state)

	log.Printf("[DEBUG] tuner: evolved pack written to %s", t.agentPath)
	return nil
}

// loosePack is a permissive struct for validating Ollama-generated YAML.
// provider_weights uses map[string]any because Ollama sometimes emits string
// values instead of floats; we sanitize rather than hard-fail.
type loosePack struct {
	Kind             string             `yaml:"kind"`
	GameRules        string             `yaml:"game_rules"`
	NarratorStyle    string             `yaml:"narrator_style"`
	Weights          looseWeights       `yaml:"weights"`
	BountyContracts  []looseBounty      `yaml:"bounty_contracts"`
}

type looseBounty struct {
	ID       string `yaml:"id"`
	XPReward int    `yaml:"xp_reward"`
	RoomID   string `yaml:"room_id"`
}

type looseWeights struct {
	BaseMultiplier  float64        `yaml:"base_multiplier"`
	CacheBonusRate  float64        `yaml:"cache_bonus_rate"`
	SpeedBonusCap   int64          `yaml:"speed_bonus_cap"`
	SpeedBonusScale float64        `yaml:"speed_bonus_scale"`
	RetryPenalty    int64          `yaml:"retry_penalty"`
	StreakMultiplier float64        `yaml:"streak_multiplier"`
	ProviderWeights map[string]any `yaml:"provider_weights"`
}

// validate checks the generated pack YAML meets minimum quality requirements.
func (t *Tuner) validate(yamlBytes []byte) error {
	var fm loosePack
	if err := yaml.Unmarshal(yamlBytes, &fm); err != nil {
		return fmt.Errorf("yaml parse failed: %w", err)
	}

	// Count achievements: lines starting with "- " and containing ":" in game_rules.
	achCount := 0
	for _, line := range strings.Split(fm.GameRules, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "- ") && strings.Contains(line, ":") {
			achCount++
		}
	}
	if achCount < 5 {
		return fmt.Errorf("too few achievements in game_rules: found %d, need ≥5", achCount)
	}

	// Validate streak_multiplier is in [0.1, 5.0] if set.
	if fm.Weights.StreakMultiplier != 0 {
		if err := checkRange("streak_multiplier", fm.Weights.StreakMultiplier); err != nil {
			return err
		}
	}

	// Validate bounty_contracts: any present entries must have id, positive xp_reward.
	for i, c := range fm.BountyContracts {
		if c.ID == "" {
			return fmt.Errorf("bounty_contracts[%d] missing id", i)
		}
		if c.XPReward <= 0 {
			return fmt.Errorf("bounty_contracts[%d] (%s) has non-positive xp_reward: %d", i, c.ID, c.XPReward)
		}
	}

	// Validate provider_weights: only entries with float values and provider-style
	// keys (containing ".") are valid; silently skip corrupt entries.
	for k, v := range fm.Weights.ProviderWeights {
		f, ok := toFloat64(v)
		if !ok {
			// Ollama stuffed a non-float here; skip it rather than fail.
			continue
		}
		if !strings.Contains(k, ".") {
			// Not a provider key (e.g. "providers.claude") — skip.
			continue
		}
		if err := checkRange("provider_weights."+k, f); err != nil {
			return err
		}
	}
	return nil
}

// toFloat64 coerces yaml-decoded numeric types to float64.
func toFloat64(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	default:
		return 0, false
	}
}

// checkRange returns an error if v is outside [0.1, 5.0].
func checkRange(name string, v float64) error {
	if v < 0.1 || v > 5.0 {
		return fmt.Errorf("weight %s=%f is outside [0.1, 5.0]", name, v)
	}
	return nil
}

// parseJSON extracts and unmarshals a JSON object from content, handling
// markdown code fences.
func parseJSON(content string, v any) error {
	content = strings.TrimSpace(content)
	// Strip markdown fences if present.
	if strings.HasPrefix(content, "```") {
		lines := strings.Split(content, "\n")
		var inner []string
		for i, line := range lines {
			if i == 0 && strings.HasPrefix(line, "```") {
				continue
			}
			if line == "```" {
				break
			}
			inner = append(inner, line)
		}
		content = strings.Join(inner, "\n")
	}
	start := strings.Index(content, "{")
	end := strings.LastIndex(content, "}")
	if start < 0 || end < start {
		return fmt.Errorf("no JSON object found in response")
	}
	return json.Unmarshal([]byte(content[start:end+1]), v)
}

// extractFrontmatter pulls the YAML content between the first --- markers.
// It handles responses that include the --- delimiters and those that don't.
func extractFrontmatter(content string) ([]byte, error) {
	content = strings.TrimSpace(content)

	// Strip markdown code fences if present.
	if strings.HasPrefix(content, "```") {
		lines := strings.Split(content, "\n")
		var inner []string
		for i, line := range lines {
			if i == 0 && strings.HasPrefix(line, "```") {
				continue
			}
			if line == "```" || line == "```yaml" {
				continue
			}
			inner = append(inner, line)
		}
		content = strings.Join(inner, "\n")
		content = strings.TrimSpace(content)
	}

	// Strip --- delimiters if present to get raw YAML.
	if strings.HasPrefix(content, "---") {
		content = content[3:]
		if idx := strings.Index(content, "\n---"); idx >= 0 {
			content = content[:idx]
		}
	}

	return []byte(strings.TrimSpace(content)), nil
}
