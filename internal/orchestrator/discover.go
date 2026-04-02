package orchestrator

import (
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/8op-org/gl1tch/internal/pipeline"
)

// workflowMeta is the minimal struct used for partial YAML unmarshal during discovery.
type workflowMeta struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
}

// DiscoverWorkflows scans dir for *.workflow.yaml files and returns a PipelineRef
// for each, with the Name field prefixed by "workflow:" so the router can
// distinguish them from pipelines without a schema change.
// Invalid or unreadable files are silently skipped.
func DiscoverWorkflows(dir string) ([]pipeline.PipelineRef, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var refs []pipeline.PipelineRef
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".workflow.yaml") {
			continue
		}
		path := filepath.Join(dir, e.Name())
		ref, ok := loadWorkflowRef(path)
		if !ok {
			continue
		}
		refs = append(refs, ref)
	}
	return refs, nil
}

// loadWorkflowRef reads the minimal metadata needed for routing from a single workflow file.
func loadWorkflowRef(path string) (pipeline.PipelineRef, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return pipeline.PipelineRef{}, false
	}

	var meta workflowMeta
	if err := yaml.Unmarshal(data, &meta); err != nil || meta.Name == "" {
		return pipeline.PipelineRef{}, false
	}

	desc := meta.Description
	if desc == "" {
		desc = meta.Name
	}

	return pipeline.PipelineRef{
		Name:        "workflow:" + meta.Name,
		Description: desc,
		Path:        path,
	}, true
}
