package brainrag

import (
	"context"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// DefaultCodeExtensions is the list of file extensions indexed by default
// when callers don't override Extensions on IndexTreeOptions.
var DefaultCodeExtensions = []string{".go", ".ts", ".py", ".md"}

// SkipDirs are directory names skipped during code indexing. Sourced from
// github.com/github/gitignore for the 10 major languages plus common
// editor/OS patterns. All hidden directories (name starting with ".") are
// also skipped unconditionally via the walk condition in IndexTree — this
// handles the long tail of hidden tool dirs (.claude, .config, .local).
var SkipDirs = map[string]bool{
	// ── Version control ──────────────────────────────────────────────────────
	".git": true, ".svn": true, ".hg": true, ".bzr": true,

	// ── Dependency trees (Go, JS/TS, Ruby, Swift, PHP, Elixir) ──────────────
	"vendor":           true, // Go, PHP, Ruby
	"node_modules":     true, // JS/TS/Node
	"bower_components": true,
	"jspm_packages":    true,
	"web_modules":      true,
	"Pods":             true, // Swift/ObjC CocoaPods
	"Carthage":         true, // Swift/ObjC Carthage
	"deps":             true, // Elixir mix deps

	// ── Build / compiled output ──────────────────────────────────────────────
	"build":   true,
	"dist":    true,
	"target":  true,
	"out":     true,
	"output":  true,
	"bin":     true,
	"obj":     true,
	"pkg":     true,
	"_build":  true,
	"debug":   true,
	"release": true,
	"classes": true,

	// ── Framework / SSR build caches ─────────────────────────────────────────
	".next":       true,
	".nuxt":       true,
	".svelte-kit": true,
	".astro":      true,
	".output":     true,
	".vuepress":   true,
	".docusaurus": true,
	".serverless": true,
	".fusebox":    true,
	".firebase":   true,
	".swiftpm":    true,
	"fastlane":    true,

	// ── Python ────────────────────────────────────────────────────────────────
	"__pycache__":    true,
	"__pypackages__": true,
	".pytest_cache":  true,
	".mypy_cache":    true,
	".ruff_cache":    true,
	".tox":           true,
	".nox":           true,
	".hypothesis":    true,
	".pybuilder":     true,
	"venv":           true,
	".venv":          true,
	"env":            true,
	"site-packages":  true,
	"htmlcov":        true,
	".coverage":      true,
	"develop-eggs":   true,
	"eggs":           true,
	"sdist":          true,
	"wheels":         true,

	// ── Java / Kotlin / Android ───────────────────────────────────────────────
	".gradle":              true,
	".mvn":                 true,
	".kotlin":              true,
	".externalNativeBuild": true,
	".cxx":                 true,
	"captures":             true,

	// ── Test / coverage output ────────────────────────────────────────────────
	"coverage":      true,
	".nyc_output":   true,
	"test-results":  true,
	"testdata":      true,
	"__snapshots__": true,
	"snapshots":     true,
	"fixtures":      true,

	// ── Editor / IDE ──────────────────────────────────────────────────────────
	".idea":         true,
	".vscode":       true,
	".vs":           true,
	"xcuserdata":    true,
	".idea_modules": true,

	// ── OS artefacts ─────────────────────────────────────────────────────────
	"__MACOSX":                true,
	".AppleDouble":            true,
	".Spotlight-V100":         true,
	".TemporaryItems":         true,
	".Trashes":                true,
	".fseventsd":              true,
	".DocumentRevisions-V100": true,

	// ── Infra / cloud ─────────────────────────────────────────────────────────
	".terraform": true,
	".dynamodb":  true,

	// ── Generic temps / caches ───────────────────────────────────────────────
	"tmp":    true,
	"temp":   true,
	"cache":  true,
	".cache": true,
	"log":    true,
	"logs":   true,

	// ── glitch-specific non-source dirs ──────────────────────────────────────
	".worktrees":   true,
	"systemprompts": true,
}

// IndexTreeOptions configures a single IndexTree run.
type IndexTreeOptions struct {
	// Root is the directory to walk. Required.
	Root string
	// Extensions is the set of file extensions to index (e.g. ".go").
	// If nil, DefaultCodeExtensions is used.
	Extensions []string
	// ChunkSize is the max number of characters per chunk. Defaults to
	// 1500 when zero or negative.
	ChunkSize int
	// Embedder produces vectors for each chunk. Required.
	Embedder Embedder
	// Store receives the chunks via IndexNote. Required.
	Store *RAGStore
	// Progress, if non-nil, receives one human-readable line per warning
	// or notable event. Pass nil to silence.
	Progress io.Writer
}

// IndexTreeResult is the summary of one IndexTree run.
type IndexTreeResult struct {
	Files  int
	Chunks int
}

// IndexTree walks opts.Root, chunks every file matching opts.Extensions,
// embeds the chunks via opts.Embedder, and upserts them into opts.Store.
//
// The walk skips SkipDirs and any directory whose name begins with ".".
// Each chunk is keyed by "file:<path>:L<start>-L<end>" so the same file
// re-indexed at a different size still produces stable note ids per chunk
// boundary.
//
// IndexTree is safe to call repeatedly: RAGStore.IndexNote checks the
// stored hash before re-embedding, so unchanged files are nearly free.
func IndexTree(ctx context.Context, opts IndexTreeOptions) (IndexTreeResult, error) {
	var res IndexTreeResult

	if opts.Root == "" {
		return res, fmt.Errorf("brainrag: IndexTree: Root is required")
	}
	if opts.Embedder == nil {
		return res, fmt.Errorf("brainrag: IndexTree: Embedder is required")
	}
	if opts.Store == nil {
		return res, fmt.Errorf("brainrag: IndexTree: Store is required")
	}

	exts := map[string]bool{}
	src := opts.Extensions
	if len(src) == 0 {
		src = DefaultCodeExtensions
	}
	for _, e := range src {
		e = strings.TrimSpace(e)
		if e != "" {
			exts[e] = true
		}
	}

	chunkSize := opts.ChunkSize
	if chunkSize <= 0 {
		chunkSize = 1500
	}

	walkErr := filepath.WalkDir(opts.Root, func(path string, d fs.DirEntry, err error) error {
		// Bail out of the walk immediately on cancellation. Without
		// this, a `pod stop` issued during a multi-minute first-pass
		// embed against a huge tree could keep the goroutine alive
		// long after the rest of the supervisor had torn down.
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}
		if err != nil {
			return err
		}
		if d.IsDir() {
			name := d.Name()
			if SkipDirs[name] || (strings.HasPrefix(name, ".") && name != ".") {
				return filepath.SkipDir
			}
			return nil
		}
		if !exts[filepath.Ext(d.Name())] {
			return nil
		}

		data, readErr := os.ReadFile(path)
		if readErr != nil {
			if opts.Progress != nil {
				fmt.Fprintf(opts.Progress, "warn: read %q: %v\n", path, readErr)
			}
			return nil
		}

		content := string(data)
		chunks := ChunkText(content, chunkSize)
		for i, chunk := range chunks {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			lineStart := countLines(content[:chunkStart(content, chunkSize, i)])
			lineEnd := lineStart + countLines(chunk)
			noteID := fmt.Sprintf("file:%s:L%d-L%d", path, lineStart+1, lineEnd)

			if embedErr := opts.Store.IndexNote(ctx, opts.Embedder, noteID, chunk); embedErr != nil {
				if opts.Progress != nil {
					fmt.Fprintf(opts.Progress, "warn: embed %q chunk %d: %v\n", path, i, embedErr)
				}
				continue
			}
			res.Chunks++
		}
		res.Files++
		return nil
	})

	// Treat any context error (cancel + deadline) as a clean stop —
	// the caller asked us to bail, the partial result we accumulated
	// in `res` is still useful as a heartbeat number.
	if walkErr != nil && walkErr != context.Canceled && walkErr != context.DeadlineExceeded {
		return res, fmt.Errorf("brainrag: IndexTree: walk %q: %w", opts.Root, walkErr)
	}
	return res, nil
}

// CountIndexableFiles returns how many files under root match exts. Used
// by callers (e.g. the pipeline action's pre-scan) to recommend an embed
// model and warn on large corpora before any embedding work begins.
func CountIndexableFiles(root string, extensions []string) int {
	if root == "" {
		return 0
	}
	exts := map[string]bool{}
	src := extensions
	if len(src) == 0 {
		src = DefaultCodeExtensions
	}
	for _, e := range src {
		e = strings.TrimSpace(e)
		if e != "" {
			exts[e] = true
		}
	}

	count := 0
	_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			if d != nil && d.IsDir() {
				name := d.Name()
				if SkipDirs[name] || (strings.HasPrefix(name, ".") && name != ".") {
					return filepath.SkipDir
				}
			}
			return nil
		}
		if exts[filepath.Ext(d.Name())] {
			count++
		}
		return nil
	})
	return count
}

// ChunkText splits text into chunks of at most chunkSize characters with
// ~10% overlap between adjacent chunks. Exposed for callers (and tests)
// that need to reproduce the same chunk boundaries IndexTree uses.
func ChunkText(text string, chunkSize int) []string {
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

func countLines(s string) int {
	return strings.Count(s, "\n")
}
