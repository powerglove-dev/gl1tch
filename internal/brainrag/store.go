// Package brainrag provides vector embedding + retrieval-augmented
// generation for brain notes and indexed code chunks.
//
// As of the ES migration, the store is backed by Elasticsearch's
// dense_vector + kNN search instead of the previous SQLite blob
// approach. This consolidates the vector store onto the same backend
// the observer already uses (glitch-events, glitch-summaries, …) so
// the brain has a single memory substrate. Kibana can now visualize
// the same vectors the runtime queries against.
package brainrag

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"time"

	"github.com/8op-org/gl1tch/internal/esearch"
	"github.com/8op-org/gl1tch/internal/store"
)

// VectorEntry is a stored embedding for a brain note or code chunk.
// Kept for backward compatibility with consumers that read the
// QueryWithText results — only the public fields are used now (the
// raw vector blob lives only in ES).
type VectorEntry struct {
	NoteID string    `json:"note_id"`
	Text   string    `json:"text"`
	Vector []float32 `json:"vector,omitempty"`
	Hash   string    `json:"hash,omitempty"`
}

// RAGStore is an Elasticsearch-backed vector store scoped to a single
// "scope" string (typically a working directory or workspace id). All
// reads and writes are filtered by scope so multiple workspaces can
// share the same backing index without bleeding into each other.
type RAGStore struct {
	es    *esearch.Client
	scope string
}

// NewRAGStore returns an ES-backed store scoped to scope.
//
// The scope namespacing convention:
//   - "cwd:/abs/path"     for indexed code chunks
//   - "workspace:<id>"    for workspace brain notes
//
// Callers that previously passed a working directory should keep doing
// so — NewRAGStoreForCWD is a thin wrapper that adds the prefix.
func NewRAGStore(es *esearch.Client, scope string) *RAGStore {
	return &RAGStore{es: es, scope: scope}
}

// NewRAGStoreForCWD returns a store scoped to a working-directory
// path. The "cwd:" prefix is added automatically so callers don't
// have to think about the scope discriminator.
func NewRAGStoreForCWD(es *esearch.Client, cwd string) *RAGStore {
	return &RAGStore{es: es, scope: "cwd:" + cwd}
}

// hashText returns the SHA256 hex hash of text.
func hashText(text string) string {
	h := sha256.Sum256([]byte(text))
	return hex.EncodeToString(h[:])
}

// vectorDoc is the wire shape of a doc in IndexVectors. Mirrors the
// fields declared in vectorsMapping (see internal/esearch/mappings.go).
type vectorDoc struct {
	Scope     string    `json:"scope"`
	NoteID    string    `json:"note_id"`
	Text      string    `json:"text"`
	Vector    []float32 `json:"vector"`
	Hash      string    `json:"hash"`
	EmbedID   string    `json:"embed_id"`
	IndexedAt string    `json:"indexed_at,omitempty"`
}

// docID is the ES document _id we use for upserts. We hash scope +
// note_id because note_id often contains slashes (file paths) and
// colons that ES would otherwise interpret as URL path segments. The
// raw scope and note_id are still stored as fields on the doc body
// for filtering and display.
func docID(scope, noteID string) string {
	h := sha256.Sum256([]byte(scope + "\x00" + noteID))
	return hex.EncodeToString(h[:])
}

// IndexNote upserts an embedding for text under noteID. If the same
// content is already indexed under the same embedder, this is a no-op
// (we read-back the existing doc by id and compare hash + embed_id).
func (r *RAGStore) IndexNote(ctx context.Context, embedder Embedder, noteID, text string) error {
	if r == nil || r.es == nil {
		return nil
	}
	h := hashText(text)
	embedID := embedder.ID()

	// Existence/freshness check: query by id+filter to avoid re-embedding
	// content the user hasn't touched. Cheaper than always re-running
	// the embedder, especially for OpenAI/Voyage where embeddings cost
	// real money.
	if r.alreadyFresh(ctx, noteID, h, embedID) {
		return nil
	}

	vec, err := embedder.Embed(ctx, text)
	if err != nil {
		return fmt.Errorf("brainrag: index note %q: %w", noteID, err)
	}

	doc := vectorDoc{
		Scope:   r.scope,
		NoteID:  noteID,
		Text:    text,
		Vector:  vec,
		Hash:    h,
		EmbedID: embedID,
		// Stamp the actual write time so QueryCodeIndexActivityScoped
		// can return last_seen_ms and the brain popover's code-index
		// row can render "last seen Xs ago" instead of always-zero.
		// vectorsMapping declares indexed_at as a `date` field; ES
		// parses RFC3339Nano cleanly without a custom format.
		IndexedAt: time.Now().UTC().Format(time.RFC3339Nano),
	}
	if err := r.es.Index(ctx, esearch.IndexVectors, docID(r.scope, noteID), doc); err != nil {
		return fmt.Errorf("brainrag: upsert vector for %q: %w", noteID, err)
	}
	return nil
}

// alreadyFresh returns true if a doc with the given id already exists
// in ES with the same hash and embed_id, meaning re-embedding would
// be wasted work.
func (r *RAGStore) alreadyFresh(ctx context.Context, noteID, hash, embedID string) bool {
	q := map[string]any{
		"size": 1,
		"_source": []string{"hash", "embed_id"},
		"query": map[string]any{
			"bool": map[string]any{
				"filter": []map[string]any{
					{"term": map[string]any{"scope": r.scope}},
					{"term": map[string]any{"note_id": noteID}},
					{"term": map[string]any{"hash": hash}},
					{"term": map[string]any{"embed_id": embedID}},
				},
			},
		},
	}
	resp, err := r.es.Search(ctx, []string{esearch.IndexVectors}, q)
	if err != nil {
		return false
	}
	return resp.Total > 0
}

// RefreshStale re-embeds notes whose SHA256(body) differs from the
// stored hash. Embedder unavailability is handled gracefully — the
// caller can keep going on best-effort.
func (r *RAGStore) RefreshStale(ctx context.Context, embedder Embedder, notes []store.BrainNote) error {
	if r == nil || r.es == nil {
		return nil
	}
	for _, n := range notes {
		id := fmt.Sprintf("%d", n.ID)
		if err := r.IndexNote(ctx, embedder, id, n.Body); err != nil {
			fmt.Fprintf(os.Stderr, "[brainrag] warn: refresh %s: %v\n", id, err)
		}
	}
	return nil
}

// Query embeds q and returns the top-K most similar note IDs scoped
// to r.scope. If filter is non-empty, only notes whose note_id is in
// filter are considered (used to scope brain RAG to workspace-linked
// notes).
//
// Returns an empty slice (not an error) on embedder failure so callers
// can degrade gracefully when Ollama is offline.
func (r *RAGStore) Query(ctx context.Context, embedder Embedder, q string, topK int, filter []string) ([]string, error) {
	if r == nil || r.es == nil {
		return nil, nil
	}
	recordQueryHit()
	qVec, err := embedder.Embed(ctx, q)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[brainrag] warn: query embed failed: %v\n", err)
		return nil, nil
	}
	hits, err := r.es.VectorSearch(ctx, r.scope, embedder.ID(), qVec, topK, filter)
	if err != nil {
		return nil, fmt.Errorf("brainrag: vector search: %w", err)
	}
	out := make([]string, 0, len(hits))
	for _, h := range hits {
		out = append(out, h.NoteID)
	}
	return out, nil
}

// QueryWithText runs the same kNN search but returns hits with their
// original text inline, so callers (e.g. the brain injector) don't
// have to round-trip back to SQLite to fetch the body.
func (r *RAGStore) QueryWithText(ctx context.Context, embedder Embedder, q string, topK int) ([]VectorEntry, error) {
	if r == nil || r.es == nil {
		return nil, nil
	}
	recordQueryHit()
	qVec, err := embedder.Embed(ctx, q)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[brainrag] warn: query embed failed: %v\n", err)
		return nil, nil
	}
	hits, err := r.es.VectorSearch(ctx, r.scope, embedder.ID(), qVec, topK, nil)
	if err != nil {
		return nil, fmt.Errorf("brainrag: vector search: %w", err)
	}
	out := make([]VectorEntry, 0, len(hits))
	for _, h := range hits {
		out = append(out, VectorEntry{
			NoteID: h.NoteID,
			Text:   h.Text,
		})
	}
	return out, nil
}
