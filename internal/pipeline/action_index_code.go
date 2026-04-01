package pipeline

import (
	"context"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/8op-org/gl1tch/internal/brainrag"
	"github.com/8op-org/gl1tch/internal/store"
)

func init() {
	builtinRegistry["builtin.index_code"] = builtinIndexCode
}

// defaultCodeExtensions is the list of file extensions indexed by default.
var defaultCodeExtensions = []string{".go", ".ts", ".py", ".md"}

// recommendEmbedModel returns the recommended Ollama embedding model and a
// human-readable rationale based on the number of source files to be indexed.
//
// Tiers:
//   - ≤500 files:   nomic-embed-text  — fast, low memory, sufficient for small repos
//   - ≤5 000 files: nomic-embed-text  — still fine; note indexing will take a minute or two
//   - ≤20 000 files: mxbai-embed-large — better recall justifies the extra time
//   - >20 000 files: mxbai-embed-large — warn user to narrow the path or increase chunk_size
func recommendEmbedModel(fileCount int) (model, rationale string) {
	switch {
	case fileCount <= 500:
		return "nomic-embed-text", "small repo — nomic-embed-text is fast and sufficient"
	case fileCount <= 5000:
		return "nomic-embed-text", fmt.Sprintf("%d files — nomic-embed-text works well; expect 1-3 min", fileCount)
	case fileCount <= 20000:
		return "mxbai-embed-large", fmt.Sprintf("%d files — mxbai-embed-large recommended for better recall at this scale", fileCount)
	default:
		return "mxbai-embed-large", fmt.Sprintf("%d files is a large corpus — consider narrowing 'path' to a subdirectory, or raising chunk_size to reduce chunk count. mxbai-embed-large recommended.", fileCount)
	}
}

// skipDirs are directory names skipped during code indexing.
// Sourced from github.com/github/gitignore for the 10 major languages plus
// common editor/OS patterns. All hidden directories (name starting with ".")
// are also skipped unconditionally via the walk condition below — this handles
// the long tail of hidden tool dirs (.claude, .config, .local, etc.).
var skipDirs = map[string]bool{
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
	"apm_modules":      true, // glitch Agent Package Manager

	// ── Build / compiled output ──────────────────────────────────────────────
	"build":   true, // Go, Java, C/C++, JS
	"dist":    true, // JS/TS, Python
	"target":  true, // Rust, Java/Maven, Kotlin
	"out":     true, // Java, Go
	"output":  true,
	"bin":     true, // Go, C/C++
	"obj":     true, // C/C++
	"pkg":     true, // Go pkg cache
	"_build":  true, // Elixir
	"debug":   true, // Rust/C debug builds
	"release": true, // Rust release builds
	"classes": true, // Java compiled classes

	// ── Framework / SSR build caches ─────────────────────────────────────────
	".next":        true, // Next.js
	".nuxt":        true, // Nuxt.js
	".svelte-kit":  true, // SvelteKit
	".astro":       true, // Astro
	".output":      true, // Nuxt/Nitro
	".vuepress":    true,
	".docusaurus":  true,
	".serverless":  true,
	".fusebox":     true,
	".firebase":    true,
	".swiftpm":     true, // Swift Package Manager
	"fastlane":     true, // iOS/Android CI

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
	"captures":             true, // Android profiler

	// ── Rust ──────────────────────────────────────────────────────────────────
	// (target/ covered above)

	// ── Test / coverage output ────────────────────────────────────────────────
	"coverage":     true,
	".nyc_output":  true,
	"test-results": true,
	"testdata":     true, // Go testdata dirs (golden files, fixtures)
	"__snapshots__": true, // Jest snapshots
	"snapshots":    true,
	"fixtures":     true,

	// ── Editor / IDE ──────────────────────────────────────────────────────────
	".idea":        true, // JetBrains
	".vscode":      true, // VS Code
	".vs":          true, // Visual Studio
	"xcuserdata":   true, // Xcode
	".idea_modules": true,

	// ── OS artefacts ─────────────────────────────────────────────────────────
	"__MACOSX":              true,
	".AppleDouble":          true,
	".Spotlight-V100":       true,
	".TemporaryItems":       true,
	".Trashes":              true,
	".fseventsd":            true,
	".DocumentRevisions-V100": true,

	// ── Infra / cloud ─────────────────────────────────────────────────────────
	".terraform": true,
	".dynamodb":  true, // local DynamoDB data

	// ── Generic temps / caches ───────────────────────────────────────────────
	"tmp":    true,
	"temp":   true,
	"cache":  true,
	".cache": true,
	"log":    true,
	"logs":   true,

	// ── glitch-specific non-source dirs ────────────────────────────────────────
	".worktrees": true, // git worktrees
}

// builtinIndexCode walks a path, chunks source files, embeds them with Ollama,
// and stores the results in the RAG vector store (brain_vectors table in glitch.db).
//
// Args:
//   - "path":       directory to walk (default ".")
//   - "extensions": comma-separated list (default ".go,.ts,.py,.md")
//   - "model":      embedding model (default "nomic-embed-text")
//   - "base_url":   Ollama base URL (default "http://localhost:11434")
//   - "chunk_size": max chars per chunk (default 1500)
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

	cwd, err := os.Getwd()
	if err != nil {
		cwd = root
	}

	// Pre-scan: count eligible files so we can recommend a model and warn on large repos.
	prescanCount := 0
	_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			if d != nil && d.IsDir() {
				name := d.Name()
				if skipDirs[name] || (strings.HasPrefix(name, ".") && name != ".") {
					return filepath.SkipDir
				}
			}
			return nil
		}
		if exts[filepath.Ext(d.Name())] {
			prescanCount++
		}
		return nil
	})

	recModel, recRationale := recommendEmbedModel(prescanCount)
	if w != nil {
		fmt.Fprintf(w, "pre-scan: %d source files found\n", prescanCount)
		fmt.Fprintf(w, "recommended model: %s (%s)\n", recModel, recRationale)
		if model != recModel {
			fmt.Fprintf(w, "note: using %q as specified; consider switching to %q for better results\n", model, recModel)
		}
		if prescanCount > 20000 {
			fmt.Fprintf(w, "warning: large corpus — consider narrowing 'path' to a subdirectory or increasing chunk_size to reduce indexing time\n")
		}
	}

	s, err := store.Open()
	if err != nil {
		return nil, fmt.Errorf("builtin.index_code: open store: %w", err)
	}
	defer s.Close()

	rs := brainrag.NewRAGStore(s.DB(), cwd)

	fileCount := 0
	chunkCount := 0

	err = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			name := d.Name()
			if skipDirs[name] || (strings.HasPrefix(name, ".") && name != ".") {
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
