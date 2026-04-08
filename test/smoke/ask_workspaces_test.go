//go:build smoke

// Package smoke runs end-to-end coverage for `glitch ask` against the user's
// real local repos and live Ollama + Elasticsearch. See README.md for details.
package smoke

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/8op-org/gl1tch/internal/brainrag"
	"github.com/8op-org/gl1tch/internal/executor"
	"github.com/8op-org/gl1tch/internal/pipeline"
)

// smokeModel is the local model used for the reasoning step. qwen2.5:7b is the
// hard default for gl1tch intelligence ops per user memory.
const smokeModel = "qwen2.5:7b"

// embedModel must match what the code-index capability uses so searches hit
// the same scope that indexing wrote into.
const embedModel = "nomic-embed-text"

// askScenario is one foundation scenario for a target repo. The real
// workspace smoke suite expands this to many prompts in a follow-up.
type askScenario struct {
	repoName   string
	repoPath   string
	extensions string // comma-separated, narrow scope to keep indexing cheap
	prompt     string // question for the reasoning step
}

// foundationScenarios returns the prompts the foundation suite runs. The
// public suite intentionally targets only the gl1tch repo itself so it stays
// reproducible for anyone with a local clone — no private/internal repo
// dependencies. Additional scenarios live in a developer-local file outside
// the repo and are loaded by name elsewhere.
func foundationScenarios() []askScenario {
	home, _ := os.UserHomeDir()
	return []askScenario{
		{
			repoName:   "gl1tch",
			repoPath:   filepath.Join(home, "Projects", "gl1tch"),
			extensions: ".md,.go",
			prompt:     "What is glitch ask? Answer in one sentence based on the indexed documentation.",
		},
	}
}

// TestSmokeAsk_Gl1tch is the foundation scenario against the gl1tch repo
// itself. It proves the full stack works end-to-end: code-index writes to
// glitch-vectors, a subsequent ask-style pipeline reads from it, and
// qwen2.5:7b produces a grounded answer. The case asserts the brainrag probe
// saw at least one vector read — the only evidence that "code index was
// actually consulted", not just configured.
func TestSmokeAsk_Gl1tch(t *testing.T) {
	requireOllama(t)
	requireModel(t, smokeModel)
	requireModel(t, embedModel)
	requireElasticsearch(t)

	stop := brainrag.EnableQueryProbe()
	defer stop()

	warmupOllama(t, smokeModel)

	mgr := buildSmokeManager(t)

	outDir := filepath.Join("out")
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		t.Fatalf("mkdir out: %v", err)
	}
	report := &strings.Builder{}
	fmt.Fprintf(report, "# smoke foundation report\n\nmodel: %s\nembed: %s\n\n", smokeModel, embedModel)

	for _, sc := range foundationScenarios() {
		t.Run(sc.repoName, func(t *testing.T) {
			if _, err := os.Stat(sc.repoPath); err != nil {
				t.Skipf("repo missing at %s: %v", sc.repoPath, err)
			}

			// builtin.index_code scopes the RAG store to os.Getwd() (see
			// internal/pipeline/action_index_code.go:90), not to its `path`
			// arg. Search does the same. If the two don't agree on cwd, the
			// index key and search key diverge and every query returns zero
			// chunks. Changing directory into the repo aligns both. t.Chdir
			// auto-restores on return.
			t.Chdir(sc.repoPath)

			// Per-case budget: qwen2.5:7b on a laptop can take 1-3 min for the
			// reasoning step, and the first cold index of a big docs tree can
			// take another 1-2 min. Six minutes each keeps the suite honest.
			ctx, cancel := context.WithTimeout(context.Background(), 6*time.Minute)
			defer cancel()

			hitsBefore := brainrag.QueryProbeHits()

			if err := runIndex(ctx, mgr, sc); err != nil {
				t.Fatalf("index_code: %v", err)
			}

			answer, err := runAsk(ctx, mgr, sc)
			if err != nil {
				t.Fatalf("ask pipeline: %v", err)
			}
			if strings.TrimSpace(answer) == "" {
				t.Fatal("expected non-empty answer")
			}

			hitsAfter := brainrag.QueryProbeHits()
			if hitsAfter <= hitsBefore {
				t.Fatalf("code index was not consulted: probe hits before=%d after=%d", hitsBefore, hitsAfter)
			}

			fmt.Fprintf(report, "## %s\n\n- repo: `%s`\n- extensions: `%s`\n- probe hits (delta): %d\n- prompt: %s\n\n**answer:**\n\n%s\n\n---\n\n",
				sc.repoName, sc.repoPath, sc.extensions, hitsAfter-hitsBefore, sc.prompt, strings.TrimSpace(answer))
		})
	}

	_ = os.WriteFile(filepath.Join(outDir, "foundation-report.md"), []byte(report.String()), 0o644)
}

// runIndex runs a one-step builtin.index_code pipeline against the repo,
// narrowing by extension so the cold run stays under a minute on a laptop.
func runIndex(ctx context.Context, mgr *executor.Manager, sc askScenario) error {
	// path "." so index_code walks the current directory — caller has
	// already chdir'd into sc.repoPath so Getwd() and path agree.
	p := &pipeline.Pipeline{
		Name:    "smoke-index-" + sc.repoName,
		Version: "1",
		Steps: []pipeline.Step{
			{
				ID:       "index",
				Executor: "builtin.index_code",
				Args: map[string]any{
					"path":       ".",
					"extensions": sc.extensions,
					"model":      embedModel,
					"chunk_size": "1500",
				},
			},
		},
	}
	_, err := pipeline.Run(ctx, p, mgr, "")
	return err
}

// runAsk runs the search → reasoning pipeline that mirrors what `glitch ask`
// will do once the workflow promotion path is wired to this same builtin pair.
// The cwd arg on builtin.search_code must match the repo path IndexTree saw,
// otherwise the RAGStore scope filters everything out.
func runAsk(ctx context.Context, mgr *executor.Manager, sc askScenario) (string, error) {
	p := &pipeline.Pipeline{
		Name:    "smoke-ask-" + sc.repoName,
		Version: "1",
		Steps: []pipeline.Step{
			{
				ID:       "search",
				Executor: "builtin.search_code",
				Args: map[string]any{
					"query": sc.prompt,
					"top_k": "6",
					"model": embedModel,
					// Scope must match whatever index_code wrote under. The
					// index path uses os.Getwd() unconditionally, which after
					// our t.Chdir is sc.repoPath — and search does NOT fall
					// back to Getwd when cwd is empty, it scopes to literal
					// "cwd:", so we have to pass it explicitly.
					"cwd": sc.repoPath,
				},
			},
			{
				ID:       "answer",
				Executor: "ollama",
				Model:    smokeModel,
				Needs:    []string{"search"},
				// {{get "..." .}} is the canonical in-memory pipeline template
				// form (see internal/pipeline/docs_improve_integration_test.go).
				// Plain {{step.x.data.value}} only resolves inside YAML files
				// that go through a preprocessor.
				Prompt: sc.prompt + "\n\n" +
					"Use ONLY the following search results as context. If the results don't cover it, say so.\n\n" +
					`---` + "\n" + `{{get "step.search.data.value" .}}` + "\n" + `---`,
			},
		},
	}
	// Smoke tests must not block on GLITCH_CLARIFY: the whole point is to
	// prove the code index read path works end-to-end. Clarification is
	// covered by its own test. Without this, qwen2.5:7b will sometimes
	// emit the marker in response to vague prompts and the runner will
	// then poll the DB for an answer that nobody writes.
	return pipeline.Run(ctx, p, mgr, "", pipeline.WithNoClarification())
}

// buildSmokeManager registers an HTTP-backed ollama stub — the same shape the
// existing internal/pipeline smoke tests use. Going through picker would
// require a configured glitch install on the machine, which CI doesn't have.
// The stub hits the local Ollama HTTP API directly so the test exercises the
// real model end-to-end without the sidecar config layer.
func buildSmokeManager(t *testing.T) *executor.Manager {
	t.Helper()
	mgr := executor.NewManager()
	if err := mgr.Register(ollamaGenerateStub(smokeModel)); err != nil {
		t.Fatalf("register ollama stub: %v", err)
	}
	return mgr
}

// ollamaGenerateStub posts directly to http://localhost:11434/api/generate,
// mirroring internal/pipeline/smoke_test.go.
func ollamaGenerateStub(model string) *executor.StubExecutor {
	return &executor.StubExecutor{
		ExecutorName: "ollama",
		ExecutorDesc: "ollama smoke stub",
		ExecuteFn: func(ctx context.Context, input string, _ map[string]string, w io.Writer) error {
			body, _ := json.Marshal(map[string]any{
				"model":  model,
				"prompt": input,
				"stream": false,
				// Pin the model in RAM for 30m so subsequent smoke runs in the
				// same shell only pay the ~20-30s qwen2.5:7b cold-load cost once.
				// Ollama default is 5m, which is too short when indexing alone
				// eats several minutes.
				"keep_alive": "30m",
			})
			req, _ := http.NewRequestWithContext(ctx, http.MethodPost, "http://localhost:11434/api/generate", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				return err
			}
			defer resp.Body.Close()
			var result struct {
				Response string `json:"response"`
			}
			if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
				return err
			}
			_, err = io.WriteString(w, result.Response)
			return err
		},
	}
}

// requireOllama skips when Ollama is not reachable locally.
func requireOllama(t *testing.T) {
	t.Helper()
	resp, err := http.Get("http://localhost:11434/api/tags")
	if err != nil {
		t.Skipf("ollama not reachable: %v", err)
	}
	_ = resp.Body.Close()
}

// requireModel skips when the given ollama model (tag-insensitive) is missing.
func requireModel(t *testing.T, model string) {
	t.Helper()
	out, err := exec.Command("ollama", "list").Output()
	if err != nil {
		t.Skipf("ollama list: %v", err)
	}
	base := model
	if idx := strings.Index(base, ":"); idx >= 0 {
		base = base[:idx]
	}
	for _, line := range strings.Split(string(out), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		name := fields[0]
		if idx := strings.Index(name, ":"); idx >= 0 {
			name = name[:idx]
		}
		if strings.EqualFold(name, base) {
			return
		}
	}
	t.Skipf("model not available: %s", model)
}

// warmupOllama sends a tiny keep_alive request so the model is resident in
// RAM before the table loop starts. Without this the first subtest pays the
// full 20-30s qwen2.5:7b cold-load cost on top of its own work, which has
// historically been the difference between a passing and a timing-out run.
// Best-effort — any error is logged and the test proceeds.
func warmupOllama(t *testing.T, model string) {
	t.Helper()
	body, _ := json.Marshal(map[string]any{
		"model":      model,
		"prompt":     "ok",
		"stream":     false,
		"keep_alive": "30m",
	})
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, "http://localhost:11434/api/generate", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	started := time.Now()
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Logf("warmup: %v (continuing — first case will pay cold-load cost)", err)
		return
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	t.Logf("warmup: %s resident after %s", model, time.Since(started).Truncate(time.Millisecond))
}

// requireElasticsearch skips when ES is not reachable. The code-index path
// writes directly to the glitch-vectors index, so the test is meaningless
// without a live cluster.
func requireElasticsearch(t *testing.T) {
	t.Helper()
	resp, err := http.Get("http://localhost:9200/_cluster/health")
	if err != nil {
		t.Skipf("elasticsearch not reachable: %v", err)
	}
	_ = resp.Body.Close()
}
