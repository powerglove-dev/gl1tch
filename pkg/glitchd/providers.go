package glitchd

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"github.com/8op-org/gl1tch/internal/chatui"
	"github.com/8op-org/gl1tch/internal/executor"
	"github.com/8op-org/gl1tch/internal/picker"
	"github.com/8op-org/gl1tch/internal/pipeline"
)

// ProviderInfo is a provider with its available models, exported for the desktop app.
type ProviderInfo struct {
	ID     string      `json:"id"`
	Label  string      `json:"label"`
	Models []ModelInfo `json:"models"`
}

// ModelInfo is a single model option.
type ModelInfo struct {
	ID      string `json:"id"`
	Label   string `json:"label"`
	Default bool   `json:"default"`
}

// AgentInfo is a discovered agent or skill.
//
// Scope is a normalized "workspace" or "global" tag derived from
// chatui's IndexEntry.Source. The sidebar uses it to filter rows by
// where they live, without callers needing to know the full set of
// raw source labels (project, project:root, stok, cli:copilot, etc.).
//
// Path is the absolute on-disk location: a directory for skills (the
// SKILL.md lives at <Path>/SKILL.md) or a file for agents. The
// editor popup uses this when opening an entry as a draft.
type AgentInfo struct {
	Name        string `json:"name"`
	Kind        string `json:"kind"`
	Source      string `json:"source"`
	Scope       string `json:"scope"` // "workspace" | "global"
	Path        string `json:"path"`
	Description string `json:"description"`
	Invoke      string `json:"invoke"`
}

// ListProviders returns all available providers and their models.
func ListProviders() []ProviderInfo {
	defs := picker.BuildProviders()
	var out []ProviderInfo
	for _, d := range defs {
		p := ProviderInfo{ID: d.ID, Label: d.Label}
		for _, m := range d.Models {
			if m.Separator {
				continue
			}
			p.Models = append(p.Models, ModelInfo{ID: m.ID, Label: m.Label, Default: m.Default})
		}
		if len(p.Models) > 0 {
			out = append(out, p)
		}
	}
	return out
}

// ListAgents returns agents and skills for the given directories.
//
// Each entry is tagged with a normalized Scope ("workspace" or
// "global") derived from chatui's raw source string. The sidebar's
// scope filter reads this; it never has to parse source labels
// directly.
func ListAgents(dirs []string) []AgentInfo {
	home, _ := os.UserHomeDir()
	seen := map[string]bool{}
	var out []AgentInfo

	for _, dir := range dirs {
		entries := chatui.ScanIndex(dir, home)
		for _, e := range entries {
			key := e.Kind + ":" + e.Name
			if seen[key] {
				continue
			}
			seen[key] = true
			out = append(out, AgentInfo{
				Name:        e.Name,
				Kind:        e.Kind,
				Source:      e.Source,
				Scope:       normalizeAgentScope(e.Source),
				Path:        e.Path,
				Description: e.Description,
				Invoke:      e.Inject,
			})
		}
	}
	return out
}

// normalizeAgentScope buckets chatui's raw source labels into the two
// values the sidebar filter cares about.
//
// "project" and "project:root" come from <workspace_dir>/.claude/ or
// AGENTS.md → workspace. Everything else (global ~/.claude, ~/.stok,
// ~/.copilot, etc.) is global. We treat user-level CLI agents like
// cli:copilot as global because they're discovered from $HOME, not
// from the active workspace.
func normalizeAgentScope(source string) string {
	switch source {
	case "project", "project:root":
		return "workspace"
	default:
		return "global"
	}
}

// StreamPromptOpts holds options for a provider call.
type StreamPromptOpts struct {
	ProviderID string
	Model      string
	Prompt     string
	SystemCtx  string // glitch system context (injected)
	AgentPath  string // agent .md file path (optional)
	// Cwd pins the provider CLI's working directory so tool use (file reads,
	// grep, shell) happens inside the active workspace rather than
	// glitch-desktop's own launch directory.
	Cwd string
}

// StreamPrompt sends a prompt to a specific provider/model and streams the response.
// Injects glitch system context so the provider knows about pipelines, executors, etc.
func StreamPrompt(ctx context.Context, opts StreamPromptOpts, tokenCh chan<- string) error {
	defer close(tokenCh)

	mgr := buildManager()

	// Build the full prompt with context
	prompt := opts.Prompt
	if opts.AgentPath != "" {
		prompt = BuildAgentPrompt(opts.AgentPath, prompt)
	}

	// Prepend system context if available
	fullPrompt := prompt
	if opts.SystemCtx != "" {
		fullPrompt = opts.SystemCtx + "\n\n---\n\n" + prompt
	}

	step := pipeline.Step{
		ID:       "ask",
		Executor: opts.ProviderID,
		Model:    opts.Model,
		Prompt:   fullPrompt,
	}
	if opts.Cwd != "" {
		step.Vars = map[string]string{"cwd": opts.Cwd}
	}
	p := &pipeline.Pipeline{
		Name:    "desktop-ask",
		Version: "1",
		Steps:   []pipeline.Step{step},
	}

	w := &chanWriter{ch: tokenCh, ctx: ctx}
	// Synthetic single-step pipelines must run with literal prompts —
	// the prompt body is opaque user content (chat messages, refine
	// system prompts, workflow YAML being edited) that may legitimately
	// contain {{ steps.X.Y }} markers the runner would otherwise try
	// to resolve and error on. Real chains call pipeline.Run directly
	// without this option, so missing step references stay loud there.
	runOpts := []pipeline.RunOption{
		pipeline.WithStepWriter(w),
		pipeline.WithLiteralPrompts(),
	}

	_, err := pipeline.Run(ctx, p, mgr, "", runOpts...)
	return err
}

// BuildExecutorManager constructs an executor.Manager pre-loaded with
// every CLI provider and any user wrappers in ~/.config/glitch/wrappers.
// Exported so the desktop app's threaded chat host can build the same
// manager the existing chat path uses without duplicating the wiring.
func BuildExecutorManager() *executor.Manager { return buildManager() }

func buildManager() *executor.Manager {
	providers := picker.BuildProviders()
	mgr := executor.NewManager()
	for _, prov := range providers {
		if prov.SidecarPath != "" {
			continue
		}
		binary := prov.Command
		if binary == "" {
			binary = prov.ID
		}
		_ = mgr.Register(executor.NewCliAdapter(prov.ID, prov.Label+" CLI adapter", binary, prov.PipelineArgs...))
	}
	home, _ := os.UserHomeDir()
	if home != "" {
		_ = mgr.LoadWrappersFromDir(filepath.Join(home, ".config", "glitch", "wrappers"))
	}
	return mgr
}

// chanWriter adapts a chan<- string to io.Writer for pipeline streaming.
type chanWriter struct {
	ch  chan<- string
	ctx context.Context
}

func (w *chanWriter) Write(p []byte) (int, error) {
	select {
	case <-w.ctx.Done():
		return 0, w.ctx.Err()
	case w.ch <- string(p):
		return len(p), nil
	}
}

var _ io.Writer = (*chanWriter)(nil)

