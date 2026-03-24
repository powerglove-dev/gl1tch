package picker

import (
	"os"
	"path/filepath"

	"github.com/sahilm/fuzzy"

	"github.com/adam-stokes/orcai/internal/chatui"
	"github.com/adam-stokes/orcai/internal/discovery"
)

// PickerItem is a single selectable row in the fuzzy picker.
type PickerItem struct {
	Kind         string // "session" | "pipeline" | "skill" | "agent" | "provider"
	Name         string
	Description  string
	SourceTag    string // "[global]" "[project]" "[copilot]" — empty for providers/sessions
	ProviderID   string // for kind=provider; also set after skill/agent picks a CLI
	ModelID      string // for kind=provider with a pre-selected model
	InjectText   string // for kind=skill|agent — sent to CLI after launch via tmux send-keys
	PipelineFile string // for kind=pipeline
	SessionIndex string // for kind=session — tmux window index to focus
	// internal — populated by ApplyFuzzy
	matchIndexes []int
}

// Filter returns the string used for fuzzy matching.
func (p PickerItem) Filter() string {
	if p.Description == "" {
		return p.Name
	}
	return p.Name + " " + p.Description
}

// SetMatchIndexes stores which character positions were matched by the fuzzy algorithm.
func (p *PickerItem) SetMatchIndexes(indexes []int) { p.matchIndexes = indexes }

// MatchIndexes returns the stored fuzzy match positions (nil when no filter active).
func (p PickerItem) MatchIndexes() []int { return p.matchIndexes }

// itemsSource implements fuzzy.Source over a []PickerItem.
type itemsSource []PickerItem

func (s itemsSource) Len() int            { return len(s) }
func (s itemsSource) String(i int) string { return s[i].Filter() }

// ApplyFuzzy filters items using sahilm/fuzzy.
// Returns all items (group order preserved) when query is empty.
// Returns matched items sorted by score when query is non-empty.
func ApplyFuzzy(query string, items []PickerItem) []PickerItem {
	if query == "" {
		out := make([]PickerItem, len(items))
		for i, item := range items {
			item.matchIndexes = nil
			out[i] = item
		}
		return out
	}
	matches := fuzzy.FindFrom(query, itemsSource(items))
	out := make([]PickerItem, len(matches))
	for i, m := range matches {
		item := items[m.Index]
		item.matchIndexes = m.MatchedIndexes
		out[i] = item
	}
	return out
}

// orcaiConfigDir returns ~/.config/orcai, or "" on error.
func orcaiConfigDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".config", "orcai")
}

// BuildPickerItems assembles all session-starter items in display group order:
// sessions → pipelines → skills → agents → providers.
// cwd and homeDir are passed to chatui.ScanIndex to locate skills and agents.
func BuildPickerItems(sessions []WindowEntry, providers []ProviderDef, cwd, homeDir string) []PickerItem {
	var items []PickerItem

	// ── sessions ──
	for _, s := range sessions {
		items = append(items, PickerItem{
			Kind:         "session",
			Name:         s.Name,
			Description:  "existing session",
			SessionIndex: s.Index,
		})
	}

	// ── pipelines ── (TypePipeline only — native/CLI-wrapper entries overlap providers)
	if configDir := orcaiConfigDir(); configDir != "" {
		if plugins, err := discovery.Discover(configDir); err == nil {
			for _, p := range plugins {
				if p.Type != discovery.TypePipeline {
					continue
				}
				items = append(items, PickerItem{
					Kind:         "pipeline",
					Name:         p.Name,
					Description:  "pipeline",
					PipelineFile: p.PipelineFile,
				})
			}
		}
	}

	// ── skills + agents ──
	index := chatui.ScanIndex(cwd, homeDir)
	for _, e := range index {
		if e.Kind != "skill" && e.Kind != "agent" {
			continue
		}
		items = append(items, PickerItem{
			Kind:        e.Kind,
			Name:        e.Name,
			Description: e.Description,
			SourceTag:   chatui.SourceLabel(e.Source),
			InjectText:  e.Inject,
		})
	}

	// ── providers ──
	for _, p := range providers {
		desc := ""
		if len(selectableModels(p)) > 0 {
			desc = "select model"
		}
		items = append(items, PickerItem{
			Kind:        "provider",
			Name:        p.Label,
			Description: desc,
			ProviderID:  p.ID,
		})
	}

	return items
}
