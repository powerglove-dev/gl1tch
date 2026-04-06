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

// WorkflowInfo is a discovered workflow, exported for the desktop app.
type WorkflowInfo struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Path        string `json:"path"`
	Workspace   string `json:"workspace"` // directory basename
}

// DiscoverWorkspaceWorkflows scans each workspace directory for workflow YAML
// files. Workflows live exclusively under <workspace>/.glitch/workflows/ and
// must use the .workflow.yaml extension.
func DiscoverWorkspaceWorkflows(dirs []string) []WorkflowInfo {
	var out []WorkflowInfo
	seen := map[string]bool{}
	for _, dir := range dirs {
		project := filepath.Base(dir)
		sd := filepath.Join(dir, ".glitch", "workflows")
		refs, _ := pipeline.DiscoverPipelines(sd)
		for _, r := range refs {
			if seen[r.Path] {
				continue
			}
			seen[r.Path] = true
			out = append(out, WorkflowInfo{
				Name: r.Name, Description: r.Description,
				Path: r.Path, Workspace: project,
			})
		}
	}
	return out
}

// RunWorkflow loads and executes a workflow YAML file, streaming output to tokenCh.
func RunWorkflow(ctx context.Context, workflowPath, input string, tokenCh chan<- string) error {
	defer close(tokenCh)

	f, err := os.Open(workflowPath)
	if err != nil {
		return fmt.Errorf("open workflow: %w", err)
	}
	defer f.Close()

	p, err := pipeline.Load(f)
	if err != nil {
		return fmt.Errorf("load workflow: %w", err)
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

// SaveWorkflow writes workflow YAML to a workspace's .glitch/workflows/ dir.
// Returns the full path of the saved file.
func SaveWorkflow(dir, name, yamlContent string) (string, error) {
	name = strings.TrimSuffix(name, ".workflow.yaml")
	filename := name + ".workflow.yaml"

	workflowsDir := filepath.Join(dir, ".glitch", "workflows")
	if err := os.MkdirAll(workflowsDir, 0o755); err != nil {
		return "", fmt.Errorf("create workflows dir: %w", err)
	}

	path := filepath.Join(workflowsDir, filename)
	if err := os.WriteFile(path, []byte(yamlContent), 0o644); err != nil {
		return "", fmt.Errorf("write workflow: %w", err)
	}
	return path, nil
}
