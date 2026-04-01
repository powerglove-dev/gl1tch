//go:build integration

package pipeline_test

// TestDocsImprove_ProducesCommit is a full end-to-end integration test that:
//   1. Copies the real docs into an isolated temp git repo
//   2. Copies a snapshot of cmd/ source for the auditor to read
//   3. Runs the docs-improve pipeline against the temp repo
//   4. Asserts that exactly one doc file was modified and committed
//
// It uses the real ollama + claude-haiku stack.
// Run with: make test-integration (or go test -tags=integration -run TestDocsImprove ./internal/pipeline/...)

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/8op-org/gl1tch/internal/pipeline"
)


func TestDocsImprove_ProducesCommit(t *testing.T) {
	model := smokeModel()
	checkModelAvailable(t, smokeModelBase(model))

	repoRoot, err := findRepoRoot()
	if err != nil {
		t.Skipf("could not locate repo root: %v", err)
	}

	// ── Set up isolated temp git repo ─────────────────────────────────────────

	tmpRepo := t.TempDir()

	// Init git repo with a default branch name.
	runGit(t, tmpRepo, "init", "-b", "main")
	runGit(t, tmpRepo, "config", "user.email", "test@glitch.dev")
	runGit(t, tmpRepo, "config", "user.name", "glitch-test")

	// Copy docs into the temp repo.
	srcDocs := filepath.Join(repoRoot, "site", "src", "content", "pipelines")
	dstDocs := filepath.Join(tmpRepo, "site", "src", "content", "pipelines")
	if err := copyDir(srcDocs, dstDocs); err != nil {
		t.Fatalf("copy docs: %v", err)
	}

	// Copy cmd/ source so the auditor can read it.
	srcCmd := filepath.Join(repoRoot, "cmd")
	dstCmd := filepath.Join(tmpRepo, "cmd")
	if err := copyDir(srcCmd, dstCmd); err != nil {
		t.Fatalf("copy cmd: %v", err)
	}

	// Initial commit so git diff has a baseline.
	runGit(t, tmpRepo, "add", ".")
	runGit(t, tmpRepo, "commit", "-m", "chore: initial docs snapshot")

	// ── Run the docs-improve pipeline ────────────────────────────────────────

	p := buildDocsImprovePipeline(tmpRepo, model)
	mgr := buildManagerWithShell(t)
	pub := &collectPublisher{}
	runOpts := []pipeline.RunOption{pipeline.WithEventPublisher(pub)}

	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Minute)
	defer cancel()

	result, err := pipeline.Run(ctx, p, mgr, "", runOpts...)
	if err != nil {
		t.Fatalf("pipeline run: %v\n\nPublished events:\n%s", err, dumpEvents(pub))
	}

	t.Logf("pipeline result:\n%s", result)

	// ── Assert: a file was actually changed ───────────────────────────────────

	diffOut := runGitOutput(t, tmpRepo, "diff", "--stat", "HEAD~1", "HEAD")
	if diffOut == "" {
		t.Error("expected a git commit with file changes, but diff vs HEAD~1 is empty")
	}
	t.Logf("git diff --stat:\n%s", diffOut)

	// Assert: the changed file is inside site/
	if !strings.Contains(diffOut, "site/") {
		t.Errorf("expected change in site/ directory, got:\n%s", diffOut)
	}

	// Assert: commit message mentions "docs"
	logOut := runGitOutput(t, tmpRepo, "log", "--oneline", "-1")
	if !strings.Contains(strings.ToLower(logOut), "docs") {
		t.Errorf("expected commit message to contain 'docs', got: %s", logOut)
	}
	t.Logf("commit: %s", logOut)
}

// TestDocsImprove_PickProducesStructuredOutput verifies that the pick step
// produces a FILE/ISSUE/FIX formatted response given real docs and code.
// Faster than the full pipeline test — only runs up to the pick step.
func TestDocsImprove_PickProducesStructuredOutput(t *testing.T) {
	model := smokeModel()
	checkModelAvailable(t, smokeModelBase(model))

	repoRoot, err := findRepoRoot()
	if err != nil {
		t.Skipf("could not locate repo root: %v", err)
	}

	docs, err := readAllDocsRecursive(filepath.Join(repoRoot, "site", "src", "content"))
	if err != nil {
		t.Fatalf("read docs: %v", err)
	}
	docs = escapeTemplateMarkers(docs)

	code, err := readCmdFiles(filepath.Join(repoRoot, "cmd"))
	if err != nil {
		t.Fatalf("read cmd: %v", err)
	}
	code = escapeTemplateMarkers(code)

	prompt := `You are a documentation auditor for a CLI tool called glitch.

## Code (cmd/ files):
` + code + `

## Current documentation files:
` + docs + `

## Task
Identify the SINGLE highest-value documentation gap or error to fix right now.

Choose one of these files: yaml-reference.md, cli-reference.md, brain.md, executors.md, examples.md, quickstart.md

Output EXACTLY this format — three lines, nothing else:
FILE: <filename>
ISSUE: <one sentence: what is missing or wrong>
FIX: <one sentence: what to add or change>`

	p := &pipeline.Pipeline{
		Name:    "test-pick",
		Version: "1",
		Steps: []pipeline.Step{
			{ID: "pick", Executor: "ollama", Model: model, Prompt: prompt},
		},
	}

	mgr := buildManagerWithShell(t)
	pub := &collectPublisher{}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	result, err := pipeline.Run(ctx, p, mgr, "", pipeline.WithEventPublisher(pub))
	if err != nil {
		t.Fatalf("pick step: %v", err)
	}
	t.Logf("pick output:\n%s", result)

	// Assert structured output format.
	if !strings.Contains(result, "FILE:") {
		t.Errorf("expected FILE: line in output, got:\n%s", result)
	}
	if !strings.Contains(result, "ISSUE:") {
		t.Errorf("expected ISSUE: line in output, got:\n%s", result)
	}
	if !strings.Contains(result, "FIX:") {
		t.Errorf("expected FIX: line in output, got:\n%s", result)
	}

	// Assert FILE: names a known doc file.
	knownFiles := []string{
		"yaml-reference.md", "cli-reference.md", "brain.md",
		"executors.md", "examples.md", "quickstart.md",
	}
	found := false
	for _, f := range knownFiles {
		if strings.Contains(result, f) {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("FILE: line did not name a known doc file; known: %v\noutput:\n%s", knownFiles, result)
	}
}

// ── helpers ──────────────────────────────────────────────────────────────────

// buildDocsImprovePipeline constructs an in-memory version of docs-improve.pipeline.yaml
// targeting tmpRepo instead of the real repository.
// The index_code and semantic_search steps are omitted here to keep the test fast;
// the pipeline is validated end-to-end without the vector index dependency.
func buildDocsImprovePipeline(tmpRepo, model string) *pipeline.Pipeline {
	docsDir := filepath.Join(tmpRepo, "site", "src", "content", "pipelines")
	cmdDir := filepath.Join(tmpRepo, "cmd")

	scanCodeCmd := fmt.Sprintf(`for f in %s/*.go; do [ -f "$f" ] || continue; case "$f" in *_test.go) continue;; esac; echo "=== $f ==="; head -80 "$f"; echo; done`, cmdDir)
	scanDocsCmd := fmt.Sprintf(`for f in %s/*.md %s/*.mdx; do [ -f "$f" ] || continue; echo "=== $(basename $f) ==="; cat "$f"; echo; done`, docsDir, docsDir)

	return &pipeline.Pipeline{
		Name:    "docs-improve-test",
		Version: "1",
		Steps: []pipeline.Step{
			{
				ID:       "scan_code",
				Executor: "shell",
				Vars:     map[string]any{"cmd": scanCodeCmd},
			},
			{
				ID:       "scan_docs",
				Executor: "shell",
				Vars:     map[string]any{"cmd": scanDocsCmd},
			},
			{
				ID:       "git_log",
				Executor: "shell",
				Vars:     map[string]any{"cmd": fmt.Sprintf(`cd %q && git log --oneline -10`, tmpRepo)},
			},
			{
				ID:       "pick",
				Executor: "ollama",
				Model:    model,
				Needs:    []string{"scan_code", "scan_docs", "git_log"},
				Prompt: `You are a documentation auditor for a CLI tool called glitch.

## Code:
{{get "step.scan_code.data.value" .}}

## Documentation:
{{get "step.scan_docs.data.value" .}}

## Task
Pick the SINGLE highest-value documentation gap. Output EXACTLY:
FILE: <filename from: yaml-reference.md cli-reference.md brain.md executors.md examples.md quickstart.md>
ISSUE: <one sentence>
FIX: <one sentence>`,
			},
			{
				ID:       "filename",
				Executor: "ollama",
				Model:    model,
				Needs:    []string{"pick"},
				Prompt:   `Output ONLY the filename (e.g. "yaml-reference.md") from this improvement request. Nothing else.\n\n{{get "step.pick.data.value" .}}`,
			},
			{
				ID:       "read_target",
				Executor: "shell",
				Needs:    []string{"filename"},
				Vars: map[string]any{
					"cmd":      fmt.Sprintf(`TARGET=$(echo "$GLITCH_FILENAME" | tr -d '[:space:]'); cat %q/"$TARGET" 2>/dev/null || echo "file not found"`, docsDir),
					"filename": `{{get "step.filename.data.value" .}}`,
				},
			},
			{
				ID:       "rewrite",
				Executor: "ollama",
				Model:    model,
				Needs:    []string{"pick", "read_target", "scan_code"},
				Prompt: `You are a technical writer. Apply this improvement to the documentation file.

Improvement:
{{get "step.pick.data.value" .}}

Current file:
{{get "step.read_target.data.value" .}}

Output the COMPLETE improved markdown file. Preserve frontmatter. No fences.`,
			},
			{
				ID:     "polish",
				Executor: "claude",
				Model:    "claude-haiku-4-5-20251001",
				Needs:    []string{"rewrite", "pick", "scan_code"},
				Prompt: `You are an expert technical writer for CLI tool documentation.

## What was improved and why:
{{get "step.pick.data.value" .}}

## Supporting code:
{{get "step.scan_code.data.value" .}}

## Draft from local model:
{{get "step.rewrite.data.value" .}}

Produce the final version. Strengthen weak sections, add examples where helpful, fix inaccuracies. Do NOT change frontmatter. Output raw markdown only.`,
			},
			{
				ID:       "write_file",
				Executor: "shell",
				Needs:    []string{"polish", "filename"},
				Input:    `{{get "step.polish.data.value" .}}`,
				Vars: map[string]any{
					"cmd":      fmt.Sprintf(`TARGET=$(echo "$GLITCH_FILENAME" | tr -d '[:space:]'); cat > %q/"$TARGET" && echo "wrote $TARGET"`, docsDir),
					"filename": `{{get "step.filename.data.value" .}}`,
				},
			},
			{
				ID:       "commit",
				Executor: "shell",
				Needs:    []string{"write_file"},
				Vars: map[string]any{
					"cmd": fmt.Sprintf(`
cd %q
if [ -z "$(git diff --stat site/)" ]; then
  echo "no changes"
  exit 0
fi
git add site/
git commit -m "docs: automated improvement by docs-improve pipeline" -m "Co-Authored-By: gl1tch <nomoresecrets+noreply@8op.org>"
echo "committed"
`, tmpRepo),
				},
			},
		},
	}
}

// copyDir recursively copies src into dst, creating dst if needed.
func copyDir(src, dst string) error {
	return filepath.WalkDir(src, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, 0o644)
	})
}

// runGit runs a git command in dir, failing the test on error.
func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

// runGitOutput runs a git command in dir and returns stdout.
func runGitOutput(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		t.Logf("git %v: %v", args, err)
		return ""
	}
	return strings.TrimSpace(string(out))
}

// dumpEvents formats collected pipeline events for test output.
func dumpEvents(pub *collectPublisher) string {
	return strings.Join(pub.lines, "\n")
}
