package pipeline

import (
	"context"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/adam-stokes/orcai/internal/brainrag"
)

func init() {
	builtinRegistry["builtin.index_code"] = builtinIndexCode
}

// defaultCodeExtensions is the list of file extensions indexed by default.
var defaultCodeExtensions = []string{".go", ".ts", ".py", ".md"}

// skipDirs are directory names skipped during code indexing.
var skipDirs = map[string]bool{
	"vendor":       true,
	"node_modules": true,
	".git":         true,
}

// builtinIndexCode walks a path, chunks source files, embeds them with Ollama,
// and stores the results in the RAG vector store.
//
// Args:
//   - "path":       directory to walk (default ".")
//   - "extensions": comma-separated list (default ".go,.ts,.py,.md")
//   - "model":      embedding model (default "nomic-embed-text")
//   - "base_url":   Ollama base URL (default "http://localhost:11434")
//   - "chunk_size": max chars per chunk (default 1500)
//   - "store_path": RAG store file path (default ~/.local/share/orcai/brain.vectors.json)
func builtinIndexCode(ctx context.Context, args map[string]any, w io.Writer) (map[string]any, error) {
	root := toString(args["path"])
	if root == "" {
		root = "."
	}

	extStr := toString(args["extensions"])
	if extStr == "" {
		extStr = strings.Join(defaultCodeExtensions, ",")
	}
	exts := map[string]bool{}
	for _, e := range strings.Split(extStr, ",") {
		e = strings.TrimSpace(e)
		if e != "" {
			exts[e] = true
		}
	}

	model := toString(args["model"])
	if model == "" {
		model = brainrag.DefaultEmbedModel
	}

	baseURL := toString(args["base_url"])
	if baseURL == "" {
		baseURL = brainrag.DefaultBaseURL
	}

	chunkSize := 1500
	if cs := toString(args["chunk_size"]); cs != "" {
		_, _ = fmt.Sscanf(cs, "%d", &chunkSize)
	}
	if chunkSize <= 0 {
		chunkSize = 1500
	}

	storePath := toString(args["store_path"])
	if storePath == "" {
		home, err := os.UserHomeDir()
		if err == nil {
			storePath = filepath.Join(home, ".local", "share", "orcai", "brain.vectors.json")
		} else {
			storePath = "brain.vectors.json"
		}
	}

	rs, err := brainrag.NewRAGStore(storePath)
	if err != nil {
		return nil, fmt.Errorf("builtin.index_code: open rag store: %w", err)
	}

	fileCount := 0
	chunkCount := 0

	err = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if skipDirs[d.Name()] {
				return filepath.SkipDir
			}
			return nil
		}

		ext := filepath.Ext(d.Name())
		if !exts[ext] {
			return nil
		}

		data, readErr := os.ReadFile(path)
		if readErr != nil {
			fmt.Fprintf(os.Stderr, "[index_code] warn: read %q: %v\n", path, readErr)
			return nil
		}

		content := string(data)
		chunks := chunkText(content, chunkSize)

		for i, chunk := range chunks {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			// Metadata ID includes file path and approximate line range.
			lineStart := countLines(content[:chunkStart(content, chunkSize, i)])
			lineEnd := lineStart + countLines(chunk)
			noteID := fmt.Sprintf("file:%s:L%d-L%d", path, lineStart+1, lineEnd)

			if embedErr := rs.IndexNote(ctx, baseURL, model, noteID, chunk); embedErr != nil {
				fmt.Fprintf(os.Stderr, "[index_code] warn: embed %q chunk %d: %v\n", path, i, embedErr)
				continue
			}
			chunkCount++
		}
		fileCount++
		return nil
	})

	if err != nil && err != context.Canceled {
		return nil, fmt.Errorf("builtin.index_code: walk %q: %w", root, err)
	}

	msg := fmt.Sprintf("indexed %d files, %d chunks", fileCount, chunkCount)
	if w != nil {
		fmt.Fprintln(w, msg)
	}
	return map[string]any{"value": msg, "files": fileCount, "chunks": chunkCount}, nil
}

// chunkText splits text into chunks of at most chunkSize characters, with ~10% overlap.
func chunkText(text string, chunkSize int) []string {
	if len(text) == 0 {
		return nil
	}
	overlap := chunkSize / 10
	if overlap < 1 {
		overlap = 1
	}

	var chunks []string
	runes := []rune(text)
	step := chunkSize - overlap
	if step < 1 {
		step = 1
	}

	for start := 0; start < len(runes); start += step {
		end := start + chunkSize
		if end > len(runes) {
			end = len(runes)
		}
		chunks = append(chunks, string(runes[start:end]))
		if end == len(runes) {
			break
		}
	}
	return chunks
}

// chunkStart returns the byte offset into content where chunk i starts.
func chunkStart(content string, chunkSize, i int) int {
	overlap := chunkSize / 10
	if overlap < 1 {
		overlap = 1
	}
	step := chunkSize - overlap
	if step < 1 {
		step = 1
	}
	runes := []rune(content)
	start := i * step
	if start > len(runes) {
		start = len(runes)
	}
	return len(string(runes[:start]))
}

// countLines counts the number of newline characters in s.
func countLines(s string) int {
	return strings.Count(s, "\n")
}
