package supervisor

import (
	"path/filepath"

	"github.com/8op-org/gl1tch/internal/picker"
	"github.com/8op-org/gl1tch/internal/providers"
)

// ResolvedModel holds the result of resolving a supervisor role to a concrete
// provider + model, along with optional cost metadata.
type ResolvedModel struct {
	ProviderID      string
	ModelID         string
	CostInputPer1M  float64
	CostOutputPer1M float64
}

// isLocal returns true when the given provider is considered "local" (i.e.
// Ollama, or any provider whose profile has no APIKeyEnv set).
func isLocal(id string, reg *providers.Registry) bool {
	if id == "ollama" {
		return true
	}
	if reg != nil {
		if p, ok := reg.Get(id); ok {
			return p.APIKeyEnv == ""
		}
	}
	return false
}

// ResolveModel looks up the named role in cfg, verifies the provider and model
// are available at runtime (via picker.BuildProviders()), and returns a usable
// ResolvedModel. It never returns an error — if the configured role cannot be
// satisfied it falls back to the first available local provider with any model.
func ResolveModel(cfg *SupervisorConfig, role string) ResolvedModel {
	var wantProvider, wantModel string
	if cfg != nil {
		if rc, ok := cfg.Roles[role]; ok {
			wantProvider = rc.Provider
			wantModel = rc.Model
		}
	}

	// Build the runtime provider list.
	available := picker.BuildProviders()

	// Build providers registry for cost metadata.
	var reg *providers.Registry
	if cfgDir := picker.GlitchConfigDir(); cfgDir != "" {
		reg, _ = providers.NewRegistry(filepath.Join(cfgDir, "providers"))
	}

	// Helper: fill cost metadata from registry.
	costFor := func(providerID, modelID string) (float64, float64) {
		if reg == nil {
			return 0, 0
		}
		p, ok := reg.Get(providerID)
		if !ok {
			return 0, 0
		}
		for _, m := range p.Models {
			if m.ID == modelID {
				return m.CostInputPer1M, m.CostOutputPer1M
			}
		}
		return 0, 0
	}

	// If a specific provider is configured, try to find it.
	if wantProvider != "" {
		for _, pd := range available {
			if pd.ID != wantProvider {
				continue
			}
			// Provider found. Now pick the model.
			modelID := wantModel
			if modelID == "" && len(pd.Models) > 0 {
				modelID = pd.Models[0].ID
			}
			if modelID != "" {
				in, out := costFor(pd.ID, modelID)
				return ResolvedModel{
					ProviderID:      pd.ID,
					ModelID:         modelID,
					CostInputPer1M:  in,
					CostOutputPer1M: out,
				}
			}
		}
	}

	// Fallback: first local provider with at least one model.
	for _, pd := range available {
		if !isLocal(pd.ID, reg) {
			continue
		}
		if len(pd.Models) == 0 {
			continue
		}
		modelID := pd.Models[0].ID
		in, out := costFor(pd.ID, modelID)
		return ResolvedModel{
			ProviderID:      pd.ID,
			ModelID:         modelID,
			CostInputPer1M:  in,
			CostOutputPer1M: out,
		}
	}

	// Last resort: first available provider with any model.
	for _, pd := range available {
		if len(pd.Models) == 0 {
			continue
		}
		modelID := pd.Models[0].ID
		in, out := costFor(pd.ID, modelID)
		return ResolvedModel{
			ProviderID:      pd.ID,
			ModelID:         modelID,
			CostInputPer1M:  in,
			CostOutputPer1M: out,
		}
	}

	// Absolute fallback — return ollama with no model (caller must handle empty ModelID gracefully).
	return ResolvedModel{ProviderID: "ollama"}
}
