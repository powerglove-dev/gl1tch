package brainrag

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"

	"github.com/adam-stokes/orcai/internal/store"
)

// VectorEntry is a stored embedding for a brain note.
type VectorEntry struct {
	NoteID string    `json:"note_id"`
	Text   string    `json:"text"`
	Vector []float32 `json:"vector"`
	Hash   string    `json:"hash"` // SHA256 of text
}

// RAGStore is a flat-file vector store for brain note embeddings.
type RAGStore struct {
	path    string // e.g. ~/.local/share/orcai/brain.vectors.json
	entries []VectorEntry
	mu      sync.RWMutex
}

// NewRAGStore opens or creates the vector store at the given path.
func NewRAGStore(path string) (*RAGStore, error) {
	r := &RAGStore{path: path}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("brainrag: mkdir %q: %w", filepath.Dir(path), err)
	}
	data, err := os.ReadFile(path)
	if err == nil {
		_ = json.Unmarshal(data, &r.entries)
	} else if !os.IsNotExist(err) {
		return nil, fmt.Errorf("brainrag: read store %q: %w", path, err)
	}
	return r, nil
}

// hashText returns the SHA256 hex hash of text.
func hashText(text string) string {
	h := sha256.Sum256([]byte(text))
	return hex.EncodeToString(h[:])
}

// IndexNote computes an embedding for text and stores it under noteID.
// If the entry already exists and the hash matches, it is a no-op.
func (r *RAGStore) IndexNote(ctx context.Context, baseURL, model, noteID, text string) error {
	h := hashText(text)

	r.mu.RLock()
	for _, e := range r.entries {
		if e.NoteID == noteID && e.Hash == h {
			r.mu.RUnlock()
			return nil // already up-to-date
		}
	}
	r.mu.RUnlock()

	vec, err := Embed(ctx, baseURL, model, text)
	if err != nil {
		return fmt.Errorf("brainrag: index note %q: %w", noteID, err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	// Update existing or append.
	for i, e := range r.entries {
		if e.NoteID == noteID {
			r.entries[i] = VectorEntry{NoteID: noteID, Text: text, Vector: vec, Hash: h}
			return r.save()
		}
	}
	r.entries = append(r.entries, VectorEntry{NoteID: noteID, Text: text, Vector: vec, Hash: h})
	return r.save()
}

// RefreshStale re-embeds notes whose SHA256(body) differs from the stored hash.
// Ollama unavailability is handled gracefully: a warning is printed and stale
// entries are skipped.
func (r *RAGStore) RefreshStale(ctx context.Context, baseURL, model string, notes []store.BrainNote) error {
	for _, n := range notes {
		id := fmt.Sprintf("%d", n.ID)
		h := hashText(n.Body)

		r.mu.RLock()
		stale := true
		for _, e := range r.entries {
			if e.NoteID == id && e.Hash == h {
				stale = false
				break
			}
		}
		r.mu.RUnlock()

		if !stale {
			continue
		}

		vec, err := Embed(ctx, baseURL, model, n.Body)
		if err != nil {
			// Degrade gracefully: log and skip this note.
			fmt.Fprintf(os.Stderr, "[brainrag] warn: could not embed note %s: %v\n", id, err)
			continue
		}

		r.mu.Lock()
		updated := false
		for i, e := range r.entries {
			if e.NoteID == id {
				r.entries[i] = VectorEntry{NoteID: id, Text: n.Body, Vector: vec, Hash: h}
				updated = true
				break
			}
		}
		if !updated {
			r.entries = append(r.entries, VectorEntry{NoteID: id, Text: n.Body, Vector: vec, Hash: h})
		}
		_ = r.save()
		r.mu.Unlock()
	}
	return nil
}

// scoredEntry is used for sorting query results.
type scoredEntry struct {
	noteID string
	score  float32
}

// Query embeds the query text and returns the top-K most similar note IDs.
// If filter is non-empty, only entries whose NoteID is in the filter are considered.
// Returns an empty slice (not an error) if Ollama is unavailable.
func (r *RAGStore) Query(ctx context.Context, baseURL, model, q string, topK int, filter []string) ([]string, error) {
	qVec, err := Embed(ctx, baseURL, model, q)
	if err != nil {
		// Degrade gracefully.
		fmt.Fprintf(os.Stderr, "[brainrag] warn: query embed failed: %v\n", err)
		return nil, nil
	}

	filterSet := make(map[string]struct{}, len(filter))
	for _, id := range filter {
		filterSet[id] = struct{}{}
	}

	r.mu.RLock()
	scored := make([]scoredEntry, 0, len(r.entries))
	for _, e := range r.entries {
		if len(filter) > 0 {
			if _, ok := filterSet[e.NoteID]; !ok {
				continue
			}
		}
		s := CosineSimilarity(qVec, e.Vector)
		scored = append(scored, scoredEntry{noteID: e.NoteID, score: s})
	}
	r.mu.RUnlock()

	sort.Slice(scored, func(i, j int) bool {
		return scored[i].score > scored[j].score
	})

	if topK <= 0 {
		topK = 5
	}
	if topK > len(scored) {
		topK = len(scored)
	}

	ids := make([]string, topK)
	for i := range topK {
		ids[i] = scored[i].noteID
	}
	return ids, nil
}

// save writes all entries to disk. Must be called with r.mu held.
func (r *RAGStore) save() error {
	data, err := json.MarshalIndent(r.entries, "", "  ")
	if err != nil {
		return fmt.Errorf("brainrag: marshal store: %w", err)
	}
	if err := os.WriteFile(r.path, data, 0o600); err != nil {
		return fmt.Errorf("brainrag: write store %q: %w", r.path, err)
	}
	return nil
}
