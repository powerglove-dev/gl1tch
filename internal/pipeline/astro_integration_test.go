//go:build integration

package pipeline_test

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/8op-org/gl1tch/internal/executor"
	"github.com/8op-org/gl1tch/internal/pipeline"
)

// astroBuildPrompt is sent to the local model to generate a pipeline YAML.
// TARGET_DIR is replaced at test time with the actual temp directory.
const astroBuildPrompt = `Output ONLY valid YAML. No markdown fences. No explanation. No commentary.

Generate a glitch pipeline YAML for building a minimal Astro website in the directory TARGET_DIR.

Rules:
- name: astro-site-build
- version: "1"
- All steps use executor: shell
- Shell steps pass the command in vars.cmd
- Step IDs: scaffold, install, build
- scaffold needs nothing; install needs [scaffold]; build needs [install]
- scaffold command: bun create astro@latest TARGET_DIR --template minimal --no-install --no-git --yes
- install command: bun install (set vars.cwd to TARGET_DIR)
- build command: bun run build (set vars.cwd to TARGET_DIR)

Begin YAML now:`

// buildManagerWithShell returns a manager with both AI providers and the shell sidecar.
func buildManagerWithShell(t *testing.T) *executor.Manager {
	t.Helper()

	mgr := buildManager()

	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("UserHomeDir: %v", err)
	}
	wrappersDir := filepath.Join(home, ".config", "glitch", "wrappers")
	// LoadWrappersFromDir returns errors for duplicates (ollama, claude already registered).
	// We only care that shell ends up registered — ignore duplicate errors.
	_ = mgr.LoadWrappersFromDir(wrappersDir)

	if _, ok := mgr.Get("shell"); !ok {
		t.Skip("shell executor not registered — check ~/.config/glitch/wrappers/shell.yaml")
	}
	return mgr
}

// stripFences removes ```yaml ... ``` or ``` ... ``` wrappers a model may produce
// despite being instructed not to.
func stripFences(s string) string {
	s = strings.TrimSpace(s)
	for _, prefix := range []string{"```yaml", "```yml", "```"} {
		if strings.HasPrefix(s, prefix) {
			s = strings.TrimPrefix(s, prefix)
			if idx := strings.LastIndex(s, "```"); idx >= 0 {
				s = s[:idx]
			}
			return strings.TrimSpace(s)
		}
	}
	return s
}

// TestAstroPipelineGenAndRun is an integration test that:
//  1. Asks a local ollama model to generate a pipeline YAML for building an Astro site.
//  2. Optionally validates the YAML with claude-haiku (teacher pass — skipped if unavailable).
//  3. Parses and executes the generated pipeline using real system tools (bun).
//  4. Asserts that dist/index.html was produced.
//
// Philosophy: local model (school child) does the generation work; haiku (teacher)
// checks correctness cheaply; bun/shell do the actual building for free.
func TestAstroPipelineGenAndRun(t *testing.T) {
	// Phase 0: pre-flight guards
	model := smokeModel()
	checkModelAvailable(t, smokeModelBase(model))

	if _, err := exec.LookPath("bun"); err != nil {
		t.Skip("bun not on PATH; skipping Astro build test")
	}

	// Phase 1: ask local model to generate pipeline YAML
	tmpDir := t.TempDir()
	prompt := strings.ReplaceAll(astroBuildPrompt, "TARGET_DIR", tmpDir)

	genPipeline := &pipeline.Pipeline{
		Name:    "ask-generate-astro",
		Version: "1",
		Steps: []pipeline.Step{
			{
				ID:       "generate",
				Executor: "ollama",
				Model:    model,
				Prompt:   prompt,
			},
		},
	}

	mgr := buildManager()
	pub := &collectPublisher{}

	genCtx, genCancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer genCancel()

	yamlOutput, err := pipeline.Run(genCtx, genPipeline, mgr, "", pipeline.WithEventPublisher(pub))
	if err != nil {
		t.Fatalf("generate step failed: %v", err)
	}
	if strings.TrimSpace(yamlOutput) == "" {
		t.Fatal("local model returned empty output")
	}

	// Phase 2: strip markdown fences (defensive)
	yamlOutput = stripFences(yamlOutput)
	t.Logf("generated YAML:\n%s", yamlOutput)

	// Phase 3: teacher validation (optional — skipped if orcai-claude not on PATH)
	if _, err := exec.LookPath("orcai-claude"); err == nil {
		validationPrompt := fmt.Sprintf(
			"Is this valid glitch pipeline YAML that would build an Astro site using only shell commands? "+
				"Reply YES or NO and one sentence.\n\n%s",
			yamlOutput,
		)
		teacherCtx, teacherCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer teacherCancel()

		cmd := exec.CommandContext(teacherCtx, "orcai-claude", "--print")
		cmd.Stdin = strings.NewReader(validationPrompt)
		var out bytes.Buffer
		cmd.Stdout = &out
		cmd.Stderr = &out

		if err := cmd.Run(); err != nil {
			t.Logf("teacher validation skipped (exec error): %v", err)
		} else {
			response := strings.TrimSpace(out.String())
			t.Logf("teacher says: %s", response)
			if !strings.HasPrefix(strings.ToUpper(response), "YES") {
				t.Errorf("teacher rejected generated YAML: %s", response)
			}
		}
	} else {
		t.Log("orcai-claude not on PATH — skipping teacher validation")
	}

	// Phase 4: parse the generated YAML
	p, err := pipeline.Load(strings.NewReader(yamlOutput))
	if err != nil {
		t.Fatalf("generated YAML failed to parse: %v\n\nYAML was:\n%s", err, yamlOutput)
	}

	// Phase 5: execute the build pipeline with the shell executor
	shellMgr := buildManagerWithShell(t)

	buildCtx, buildCancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer buildCancel()

	buildPub := &collectPublisher{}
	_, err = pipeline.Run(buildCtx, p, shellMgr, "", pipeline.WithEventPublisher(buildPub))
	if err != nil {
		t.Fatalf("generated pipeline execution failed: %v", err)
	}

	// Phase 6: assert dist/index.html was produced
	distDir := filepath.Join(tmpDir, "dist")
	if _, err := os.Stat(distDir); os.IsNotExist(err) {
		t.Fatalf("expected dist/ to exist at %s — Astro build may not have run", distDir)
	}

	indexHTML := filepath.Join(distDir, "index.html")
	info, err := os.Stat(indexHTML)
	if os.IsNotExist(err) {
		t.Fatalf("expected dist/index.html to exist at %s", indexHTML)
	}
	if err != nil {
		t.Fatalf("stat dist/index.html: %v", err)
	}
	if info.Size() == 0 {
		t.Error("dist/index.html exists but is empty")
	}

	t.Logf("Astro site built successfully: dist/index.html (%d bytes)", info.Size())
}
