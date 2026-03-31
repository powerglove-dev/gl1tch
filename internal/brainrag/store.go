package brainrag

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"math"
	"os"
	"sort"

	"github.com/adam-stokes/orcai/internal/store"
)

// VectorEntry is a stored embedding for a brain note.
type VectorEntry struct {
	NoteID string    `json:"note_id"`
	Text   string    `json:"text"`
	Vector []float32 `json:"vector"`
	Hash   string    `json:"hash"` // SHA256 of text
}

// RAGStore is a SQLite-backed vector store scoped to a working directory.
type RAGStore struct {
	db  *sql.DB
	cwd string
}

// NewRAGStore creates a RAGStore backed by db, scoped to cwd.
// The brain_vectors table must already exist (applied by store.Open/OpenAt).
func NewRAGStore(db *sql.DB, cwd string) *RAGStore {
	return &RAGStore{db: db, cwd: cwd}
}

// hashText returns the SHA256 hex hash of text.
func hashText(text string) string {
	h := sha256.Sum256([]byte(text))
	return hex.EncodeToString(h[:])
}

// encodeVector serializes a float32 slice as a little-endian IEEE 754 byte blob.
func encodeVector(v []float32) []byte {
	buf := make([]byte, len(v)*4)
	for i, f := range v {
		binary.LittleEndian.PutUint32(buf[i*4:], math.Float32bits(f))
	}
	return buf
}

// decodeVector deserializes a little-endian IEEE 754 byte blob to a float32 slice.
func decodeVector(b []byte) []float32 {
	v := make([]float32, len(b)/4)
	for i := range v {
		v[i] = math.Float32frombits(binary.LittleEndian.Uint32(b[i*4:]))
	}
	return v
}

// IndexNote computes an embedding for text and stores it under noteID scoped to r.cwd.
// If an entry already exists with the same hash, it is a no-op.
func (r *RAGStore) IndexNote(ctx context.Context, baseURL, model, noteID, text string) error {
	h := hashText(text)

	var existingHash string
	err := r.db.QueryRowContext(ctx,
		`SELECT hash FROM brain_vectors WHERE cwd = ? AND note_id = ?`,
		r.cwd, noteID,
	).Scan(&existingHash)
	if err == nil && existingHash == h {
		return nil // already up-to-date
	}
	if err != nil && err != sql.ErrNoRows {
		return fmt.Errorf("brainrag: check existing vector: %w", err)
	}

	vec, err := Embed(ctx, baseURL, model, text)
	if err != nil {
		return fmt.Errorf("brainrag: index note %q: %w", noteID, err)
	}

	blob := encodeVector(vec)
	_, err = r.db.ExecContext(ctx,
		`INSERT INTO brain_vectors (cwd, note_id, text, vector, hash)
		 VALUES (?, ?, ?, ?, ?)
		 ON CONFLICT(cwd, note_id) DO UPDATE SET
		   text       = excluded.text,
		   vector     = excluded.vector,
		   hash       = excluded.hash,
		   indexed_at = CURRENT_TIMESTAMP`,
		r.cwd, noteID, text, blob, h,
	)
	if err != nil {
		return fmt.Errorf("brainrag: upsert vector for %q: %w", noteID, err)
	}
	return nil
}

// RefreshStale re-embeds notes whose SHA256(body) differs from the stored hash.
// Ollama unavailability is handled gracefully: a warning is printed and stale
// entries are skipped.
func (r *RAGStore) RefreshStale(ctx context.Context, baseURL, model string, notes []store.BrainNote) error {
	for _, n := range notes {
		id := fmt.Sprintf("%d", n.ID)
		h := hashText(n.Body)

		var existingHash string
		err := r.db.QueryRowContext(ctx,
			`SELECT hash FROM brain_vectors WHERE cwd = ? AND note_id = ?`,
			r.cwd, id,
		).Scan(&existingHash)
		if err == nil && existingHash == h {
			continue
		}
		if err != nil && err != sql.ErrNoRows {
			fmt.Fprintf(os.Stderr, "[brainrag] warn: check stale %s: %v\n", id, err)
			continue
		}

		vec, err := Embed(ctx, baseURL, model, n.Body)
		if err != nil {
			fmt.Fprintf(os.Stderr, "[brainrag] warn: could not embed note %s: %v\n", id, err)
			continue
		}

		blob := encodeVector(vec)
		if _, err = r.db.ExecContext(ctx,
			`INSERT INTO brain_vectors (cwd, note_id, text, vector, hash)
			 VALUES (?, ?, ?, ?, ?)
			 ON CONFLICT(cwd, note_id) DO UPDATE SET
			   text       = excluded.text,
			   vector     = excluded.vector,
			   hash       = excluded.hash,
			   indexed_at = CURRENT_TIMESTAMP`,
			r.cwd, id, n.Body, blob, h,
		); err != nil {
			fmt.Fprintf(os.Stderr, "[brainrag] warn: upsert stale vector %s: %v\n", id, err)
		}
	}
	return nil
}

// Query embeds the query text and returns the top-K most similar note IDs for r.cwd.
// If filter is non-empty, only entries whose NoteID is in filter are considered.
// Returns an empty slice (not an error) if Ollama is unavailable.
func (r *RAGStore) Query(ctx context.Context, baseURL, model, q string, topK int, filter []string) ([]string, error) {
	qVec, err := Embed(ctx, baseURL, model, q)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[brainrag] warn: query embed failed: %v\n", err)
		return nil, nil
	}

	rows, err := r.db.QueryContext(ctx,
		`SELECT note_id, vector FROM brain_vectors WHERE cwd = ?`,
		r.cwd,
	)
	if err != nil {
		return nil, fmt.Errorf("brainrag: query vectors: %w", err)
	}
	defer rows.Close()

	filterSet := make(map[string]struct{}, len(filter))
	for _, id := range filter {
		filterSet[id] = struct{}{}
	}

	type scoredEntry struct {
		noteID string
		score  float32
	}
	var scored []scoredEntry

	for rows.Next() {
		var noteID string
		var blob []byte
		if err := rows.Scan(&noteID, &blob); err != nil {
			continue
		}
		if len(filter) > 0 {
			if _, ok := filterSet[noteID]; !ok {
				continue
			}
		}
		s := CosineSimilarity(qVec, decodeVector(blob))
		scored = append(scored, scoredEntry{noteID: noteID, score: s})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("brainrag: query rows: %w", err)
	}

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
