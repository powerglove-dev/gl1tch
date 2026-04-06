package glitchd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/8op-org/gl1tch/internal/pipeline"
)

// noisePatterns are line prefixes/substrings in executor output that should be
// filtered from desktop chat streaming. These are stats/metadata lines emitted
// by CLI adapters (github-copilot, claude, etc.) that clutter the chat UI.
var noisePatterns = []string{
	"Total usage est:",
	"Total session time:",
	"Total code changes:",
	"Breakdown by AI model:",
	"API time spent:",
	"Premium requests)",
}

// filteringChanWriter wraps chanWriter to strip executor noise lines.
type filteringChanWriter struct {
	inner *chanWriter
	buf   string
}

func (w *filteringChanWriter) Write(p []byte) (int, error) {
	w.buf += string(p)

	// Process complete lines
	for {
		nl := strings.IndexByte(w.buf, '\n')
		if nl < 0 {
			break
		}
		line := w.buf[:nl+1]
		w.buf = w.buf[nl+1:]

		if isNoiseLine(line) {
			continue
		}
		if _, err := w.inner.Write([]byte(line)); err != nil {
			return len(p), err
		}
	}
	return len(p), nil
}

func (w *filteringChanWriter) Flush() {
	if w.buf != "" && !isNoiseLine(w.buf) {
		_, _ = w.inner.Write([]byte(w.buf))
	}
	w.buf = ""
}

func isNoiseLine(line string) bool {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" {
		return false
	}
	for _, pat := range noisePatterns {
		if strings.Contains(trimmed, pat) {
			return true
		}
	}
	return false
}

// PipelineInfo is a discovered pipeline, exported for the desktop app.
type PipelineInfo struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Path        string `json:"path"`
	Project     string `json:"project"` // directory basename
}

// DiscoverWorkspacePipelines scans all workspace directories for pipeline YAML files.
func DiscoverWorkspacePipelines(dirs []string) []PipelineInfo {
	var out []PipelineInfo
	seen := map[string]bool{}
	for _, dir := range dirs {
		project := filepath.Base(dir)

		// All directories that may contain workflow files.
		// Both .workflow.yaml (current) and .pipeline.yaml (legacy) are picked
		// up by pipeline.DiscoverPipelines.
		scanDirs := []string{
			dir,
			filepath.Join(dir, ".glitch", "workflows"),
			filepath.Join(dir, ".glitch", "pipelines"), // legacy
			filepath.Join(dir, "workflows"),
			filepath.Join(dir, "pipelines"), // legacy
		}

		for _, sd := range scanDirs {
			refs, _ := pipeline.DiscoverPipelines(sd)
			for _, r := range refs {
				if seen[r.Path] {
					continue
				}
				seen[r.Path] = true
				out = append(out, PipelineInfo{
					Name: r.Name, Description: r.Description,
					Path: r.Path, Project: project,
				})
			}
		}
	}
	return out
}

// RunPipeline loads and executes a pipeline YAML file, streaming output to tokenCh.
func RunPipeline(ctx context.Context, pipelinePath, input string, tokenCh chan<- string) error {
	defer close(tokenCh)

	f, err := os.Open(pipelinePath)
	if err != nil {
		return fmt.Errorf("open pipeline: %w", err)
	}
	defer f.Close()

	p, err := pipeline.Load(f)
	if err != nil {
		return fmt.Errorf("load pipeline: %w", err)
	}

	mgr := buildManager()
	fw := &filteringChanWriter{inner: &chanWriter{ch: tokenCh, ctx: ctx}}
	opts := []pipeline.RunOption{pipeline.WithStepWriter(fw)}

	_, err = pipeline.Run(ctx, p, mgr, input, opts...)
	fw.Flush()
	return err
}

// WorkflowStepInfo is a single step's preview metadata for the desktop step editor.
type WorkflowStepInfo struct {
	ID            string `json:"id"`
	Executor      string `json:"executor"`
	Model         string `json:"model"`
	PromptPreview string `json:"prompt_preview"`
	Needs         []string `json:"needs,omitempty"`
}

// WorkflowFileDetails is the full preview returned by GetWorkflowFileDetails.
type WorkflowFileDetails struct {
	Name        string             `json:"name"`
	Description string             `json:"description"`
	Path        string             `json:"path"`
	Steps       []WorkflowStepInfo `json:"steps"`
}

// GetWorkflowFileDetails loads a workflow YAML file and returns its metadata
// + step previews as JSON. Used by the desktop step editor to show users what
// a workflow does before they run it.
func GetWorkflowFileDetails(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return "{}"
	}
	defer f.Close()

	p, err := pipeline.Load(f)
	if err != nil {
		return "{}"
	}

	out := WorkflowFileDetails{
		Name:        p.Name,
		Description: p.Description,
		Path:        path,
	}
	for _, s := range p.Steps {
		preview := s.Prompt
		if preview == "" {
			preview = s.Input
		}
		// Trim multi-line prompts to a single-line preview
		if len(preview) > 140 {
			preview = preview[:137] + "..."
		}
		preview = strings.TrimSpace(strings.ReplaceAll(preview, "\n", " "))

		executor := s.Executor
		if executor == "" {
			executor = s.Type
		}

		out.Steps = append(out.Steps, WorkflowStepInfo{
			ID:            s.ID,
			Executor:      executor,
			Model:         s.Model,
			PromptPreview: preview,
			Needs:         s.Needs,
		})
	}

	b, _ := json.Marshal(out)
	return string(b)
}

// SavePipeline writes pipeline YAML to a project directory.
// Returns the full path of the saved file.
func SavePipeline(dir, name, yamlContent string) (string, error) {
	// Normalize name
	name = strings.TrimSuffix(name, ".pipeline.yaml")
	filename := name + ".pipeline.yaml"

	// Save to .glitch/pipelines/ in the project
	pipelinesDir := filepath.Join(dir, ".glitch", "pipelines")
	if err := os.MkdirAll(pipelinesDir, 0o755); err != nil {
		return "", fmt.Errorf("create pipelines dir: %w", err)
	}

	path := filepath.Join(pipelinesDir, filename)
	if err := os.WriteFile(path, []byte(yamlContent), 0o644); err != nil {
		return "", fmt.Errorf("write pipeline: %w", err)
	}
	return path, nil
}
