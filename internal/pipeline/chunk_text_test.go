package pipeline

import (
	"strings"
	"testing"
)

// TestChunkText is a pure unit test for the chunking helper. It lives
// here (not in action_index_code_test.go) because the rest of that
// file is gated behind the "integration" build tag for ES + Ollama
// end-to-end tests.
func TestChunkText(t *testing.T) {
	tests := []struct {
		name      string
		text      string
		chunkSize int
		wantMin   int
		wantMax   int
	}{
		{"empty", "", 100, 0, 0},
		{"shorter than chunk", "hello world", 100, 1, 1},
		{"exactly chunk size", strings.Repeat("a", 100), 100, 1, 1},
		{"two chunks with overlap", strings.Repeat("a", 150), 100, 2, 2},
		{"overlap preserved", strings.Repeat("x", 300), 100, 3, 4},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			chunks := chunkText(tt.text, tt.chunkSize)
			n := len(chunks)
			if n < tt.wantMin || n > tt.wantMax {
				t.Errorf("chunkText(%d chars, size=%d): want %d-%d chunks, got %d",
					len(tt.text), tt.chunkSize, tt.wantMin, tt.wantMax, n)
			}
		})
	}
}
