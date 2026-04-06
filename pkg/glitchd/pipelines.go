package glitchd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/8op-org/gl1tch/internal/pipeline"
)

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
	for _, dir := range dirs {
		project := filepath.Base(dir)

		// Scan root of directory
		refs, _ := pipeline.DiscoverPipelines(dir)
		for _, r := range refs {
			out = append(out, PipelineInfo{
				Name: r.Name, Description: r.Description,
				Path: r.Path, Project: project,
			})
		}

		// Scan .glitch/pipelines/ subdirectory
		refs, _ = pipeline.DiscoverPipelines(filepath.Join(dir, ".glitch", "pipelines"))
		for _, r := range refs {
			out = append(out, PipelineInfo{
				Name: r.Name, Description: r.Description,
				Path: r.Path, Project: project,
			})
		}

		// Scan pipelines/ subdirectory
		refs, _ = pipeline.DiscoverPipelines(filepath.Join(dir, "pipelines"))
		for _, r := range refs {
			out = append(out, PipelineInfo{
				Name: r.Name, Description: r.Description,
				Path: r.Path, Project: project,
			})
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
	w := &chanWriter{ch: tokenCh, ctx: ctx}
	opts := []pipeline.RunOption{pipeline.WithStepWriter(w)}

	_, err = pipeline.Run(ctx, p, mgr, input, opts...)
	return err
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
