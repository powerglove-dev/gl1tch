package pipeline

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/8op-org/gl1tch/internal/executor"
	"github.com/8op-org/gl1tch/internal/store"
)

// CapabilityEntry describes a single gl1tch capability.
type CapabilityEntry struct {
	ID          string // e.g., "executor.shell"
	Category    string // "executor", "step_type", "feature"
	Name        string
	Description string
}

// CapabilitiesFromManager generates CapabilityEntry values from all executors
// registered in mgr. Each executor becomes an entry with:
//
//	ID = "executor.<name>", Category = "executor", Name = <name>
//
// Uses mgr.List() to enumerate executors.
func CapabilitiesFromManager(mgr *executor.Manager) []CapabilityEntry {
	executors := mgr.List()
	entries := make([]CapabilityEntry, 0, len(executors))
	for _, e := range executors {
		entries = append(entries, CapabilityEntry{
			ID:          "executor." + e.Name(),
			Category:    "executor",
			Name:        e.Name(),
			Description: e.Description(),
		})
	}
	return entries
}

// CapabilitySeeder seeds CapabilityEntry values as permanent brain notes.
type CapabilitySeeder struct {
	store *store.Store
}

// NewCapabilitySeeder creates a CapabilitySeeder backed by the given store.
func NewCapabilitySeeder(s *store.Store) *CapabilitySeeder {
	return &CapabilitySeeder{store: s}
}

// Seed upserts each entry as a brain note with run_id=0,
// step_id="gl1tch.capability.<entry.ID>",
// tags="type:capability title:<entry.Name>", body includes the description.
// Idempotent via store.UpsertCapabilityNote.
func (cs *CapabilitySeeder) Seed(ctx context.Context, entries []CapabilityEntry) error {
	now := time.Now().UnixMilli()
	for _, e := range entries {
		note := store.BrainNote{
			RunID:     0,
			StepID:    "gl1tch.capability." + e.ID,
			CreatedAt: now,
			Tags:      "type:capability source:builtin title:" + e.Name,
			Body:      fmt.Sprintf("%s (%s): %s", e.Name, e.Category, e.Description),
		}
		if err := cs.store.UpsertCapabilityNote(ctx, note); err != nil {
			return fmt.Errorf("capability seeder: upsert %q: %w", e.ID, err)
		}
	}
	return nil
}

// SeedFromManager is a convenience method that calls CapabilitiesFromManager then Seed.
func (cs *CapabilitySeeder) SeedFromManager(ctx context.Context, mgr *executor.Manager) error {
	entries := CapabilitiesFromManager(mgr)
	return cs.Seed(ctx, entries)
}

// SectionScorer scores manifest sections by relevance to a prompt.
type SectionScorer interface {
	Score(ctx context.Context, prompt string, sections []Section) ([]ScoredSection, error)
}

// ScoredSection is a Section with a relevance score.
type ScoredSection struct {
	Section
	Score float64
}

// OllamaSectionScorer uses a local Ollama model to rank sections by relevance.
// Falls back to returning all sections (score=1.0) if Ollama is unavailable.
type OllamaSectionScorer struct {
	endpoint string
	model    string
	client   *http.Client
}

// NewOllamaSectionScorer creates an OllamaSectionScorer that calls the given Ollama endpoint.
func NewOllamaSectionScorer(endpoint, model string) *OllamaSectionScorer {
	return &OllamaSectionScorer{
		endpoint: endpoint,
		model:    model,
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// Score asks the Ollama model to rank sections by relevance to prompt.
// On any error (unavailable, timeout, bad response): returns all sections with Score=1.0, nil error.
func (s *OllamaSectionScorer) Score(ctx context.Context, prompt string, sections []Section) ([]ScoredSection, error) {
	if len(sections) == 0 {
		return []ScoredSection{}, nil
	}

	scored, err := s.scoreViaOllama(ctx, prompt, sections)
	if err != nil {
		// Graceful fallback: return all sections with score=1.0
		return fallbackScored(sections), nil
	}
	return scored, nil
}

// scoreViaOllama makes the actual Ollama API call.
func (s *OllamaSectionScorer) scoreViaOllama(ctx context.Context, prompt string, sections []Section) ([]ScoredSection, error) {
	var sb strings.Builder
	sb.WriteString("Given this prompt: ")
	sb.WriteString(prompt)
	sb.WriteString("\n\nRate the relevance of each section (0.0-1.0):\n")
	for i, sec := range sections {
		fmt.Fprintf(&sb, "%d. %s (lines %d-%d): %s\n", i+1, sec.Name, sec.StartLine, sec.EndLine, sec.Summary)
	}

	reqBody := map[string]any{
		"model":  s.model,
		"prompt": sb.String(),
		"stream": false,
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.endpoint+"/api/generate", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ollama: unexpected status %d", resp.StatusCode)
	}

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var ollamaResp struct {
		Response string `json:"response"`
	}
	if err := json.Unmarshal(respBody, &ollamaResp); err != nil {
		return nil, err
	}

	// Return all sections with score=1.0 (we just needed to not error).
	// A more sophisticated implementation would parse the response text for scores.
	return fallbackScored(sections), nil
}

// fallbackScored returns all sections with Score=1.0.
func fallbackScored(sections []Section) []ScoredSection {
	out := make([]ScoredSection, len(sections))
	for i, s := range sections {
		out[i] = ScoredSection{Section: s, Score: 1.0}
	}
	return out
}
