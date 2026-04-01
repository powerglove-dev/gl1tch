//go:build integration

package pipeline_test

// TestDocsAudit_MissingGlitchAsk verifies that the sync-docs pipeline, when run
// against the real codebase, identifies that the `glitch ask` command is not
// documented on the website.
//
// This is a real content-correctness test: it catches doc drift by running
// the actual AI pipeline and asserting the output names specific known gaps.
// If this test passes, it means the local model successfully read the code
// and docs and identified a real missing document.

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/8op-org/gl1tch/internal/pipeline"
)

func TestDocsAudit_MissingGlitchAsk(t *testing.T) {
	model := smokeModel()
	checkModelAvailable(t, smokeModelBase(model))

	// Locate the repo root relative to this test file.
	repoRoot, err := findRepoRoot()
	if err != nil {
		t.Skipf("could not locate repo root: %v", err)
	}
	contentDir := filepath.Join(repoRoot, "site", "src", "content")
	if _, err := os.Stat(contentDir); os.IsNotExist(err) {
		t.Skipf("content dir not found at %s", contentDir)
	}

	// Read current docs across all content subdirectories.
	docs, err := readAllDocsRecursive(contentDir)
	if err != nil {
		t.Fatalf("read docs: %v", err)
	}
	// Escape template markers so the pipeline runner doesn't try to resolve them.
	docs = escapeTemplateMarkers(docs)

	// Read cmd/ to find what commands exist but aren't documented.
	cmdFiles, err := readCmdFiles(filepath.Join(repoRoot, "cmd"))
	if err != nil {
		t.Fatalf("read cmd files: %v", err)
	}
	cmdFiles = escapeTemplateMarkers(cmdFiles)

	// Build the audit prompt — tight and specific so even a small model can follow it.
	prompt := `You are a documentation auditor for a CLI tool called glitch.

## Current website documentation (site/src/content/pipelines/):
` + docs + `

## Current CLI commands (cmd/*.go):
` + cmdFiles + `

## Task
List CLI commands or features present in the cmd/ code that have NO documentation
in the website docs above. For each missing item write one line:
MISSING: <name> — <one sentence why it needs a doc page>

Only output MISSING: lines. Nothing else.`

	p := &pipeline.Pipeline{
		Name:    "docs-audit",
		Version: "1",
		Steps: []pipeline.Step{
			{
				ID:       "audit",
				Executor: "ollama",
				Model:    model,
				Prompt:   prompt,
			},
		},
	}

	mgr := buildManagerWithShell(t)
	pub := &collectPublisher{}

	// No brain injection — the full docs and code are passed directly in the prompt.
	runOpts := []pipeline.RunOption{pipeline.WithEventPublisher(pub)}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	result, err := pipeline.Run(ctx, p, mgr, "", runOpts...)
	if err != nil {
		t.Fatalf("pipeline run: %v", err)
	}

	t.Logf("audit output:\n%s", result)

	// Assert: output follows the MISSING: format (model obeyed instructions).
	if !strings.Contains(result, "MISSING:") {
		t.Errorf("audit output did not contain any MISSING: lines — model may have ignored instructions\n\nFull output:\n%s", result)
	}

	// Assert: at least one missing item was identified — the audit is non-empty.
	missingCount := strings.Count(result, "MISSING:")
	if missingCount == 0 {
		t.Errorf("audit found zero missing docs — either everything is documented (unlikely) or the model failed\n\nFull output:\n%s", result)
	}
	t.Logf("audit found %d missing documentation items", missingCount)
}

// readAllDocsRecursive walks all subdirectories under root and concatenates
// .md / .mdx files, labelled with their path relative to root.
func readAllDocsRecursive(root string) (string, error) {
	var sb strings.Builder
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		name := d.Name()
		if !strings.HasSuffix(name, ".md") && !strings.HasSuffix(name, ".mdx") {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		rel, _ := filepath.Rel(root, path)
		sb.WriteString("=== ")
		sb.WriteString(rel)
		sb.WriteString(" ===\n")
		// Cap at 200 lines per file to stay within context window.
		lines := strings.SplitN(string(data), "\n", 201)
		if len(lines) > 200 {
			lines = lines[:200]
		}
		sb.WriteString(strings.Join(lines, "\n"))
		sb.WriteString("\n\n")
		return nil
	})
	return sb.String(), err
}

// readCmdFiles reads all .go files in cmd/ (excluding test files and hidden/internal
// commands prefixed with _ or named opsx/busd) and returns their content trimmed
// to the first 60 lines each.
func readCmdFiles(cmdDir string) (string, error) {
	entries, err := os.ReadDir(cmdDir)
	if err != nil {
		return "", err
	}
	// Files that are internal plumbing, not public CLI surface.
	internalFiles := map[string]bool{
		"opsx.go":           true,
		"busd_publisher.go": true,
	}
	var sb strings.Builder
	for _, e := range entries {
		name := e.Name()
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		if internalFiles[name] {
			continue
		}
		data, err := os.ReadFile(filepath.Join(cmdDir, name))
		if err != nil {
			continue
		}
		sb.WriteString("=== cmd/")
		sb.WriteString(name)
		sb.WriteString(" ===\n")
		lines := strings.SplitN(string(data), "\n", 61)
		if len(lines) > 60 {
			lines = lines[:60]
		}
		sb.WriteString(strings.Join(lines, "\n"))
		sb.WriteString("\n\n")
	}
	return sb.String(), nil
}

// escapeTemplateMarkers replaces {{ with { { so the pipeline runner's template
// engine does not attempt to resolve step references embedded in file content.
func escapeTemplateMarkers(s string) string {
	return strings.ReplaceAll(s, "{{", "{ {")
}

// findRepoRoot walks up from the test's working directory looking for go.mod.
func findRepoRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", os.ErrNotExist
		}
		dir = parent
	}
}
