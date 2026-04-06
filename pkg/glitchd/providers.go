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
type AgentInfo struct {
	Name        string `json:"name"`
	Kind        string `json:"kind"`
	Source      string `json:"source"`
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
				Description: e.Description,
				Invoke:      e.Inject,
			})
		}
	}
	return out
}

// StreamPromptOpts holds options for a provider call.
type StreamPromptOpts struct {
	ProviderID   string
	Model        string
	Prompt       string
	SystemCtx    string // glitch system context (injected)
	AgentPath    string // agent .md file path (optional)
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

	p := &pipeline.Pipeline{
		Name:    "desktop-ask",
		Version: "1",
		Steps: []pipeline.Step{
			{
				ID:       "ask",
				Executor: opts.ProviderID,
				Model:    opts.Model,
				Prompt:   fullPrompt,
			},
		},
	}

	w := &chanWriter{ch: tokenCh, ctx: ctx}
	runOpts := []pipeline.RunOption{pipeline.WithStepWriter(w)}

	_, err := pipeline.Run(ctx, p, mgr, "", runOpts...)
	return err
}

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

