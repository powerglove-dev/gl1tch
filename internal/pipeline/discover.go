package pipeline

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// PipelineRef is a lightweight reference to a discovered pipeline file.
type PipelineRef struct {
	Name           string
	Description    string
	Path           string
	// TriggerPhrases holds example imperative invocation phrases from the pipeline YAML.
	// When non-empty, the intent router embeds these instead of the description.
	TriggerPhrases []string
}

// pipelineMeta is the minimal struct used for partial YAML unmarshal during discovery.
// Using a minimal struct (not pipeline.Load) keeps discovery fast and tolerant of
// partially-authored or future-schema pipeline files.
type pipelineMeta struct {
	Name           string   `yaml:"name"`
	Description    string   `yaml:"description"`
	TriggerPhrases []string `yaml:"trigger_phrases"`
}

// DiscoverPipelines scans dir for *.pipeline.yaml files and returns a PipelineRef
// for each. Invalid or unreadable files are silently skipped.
// Description is extracted using the fallback chain:
//  1. `description:` YAML field
//  2. First `# comment` line in the file
//  3. `name:` field
func DiscoverPipelines(dir string) ([]PipelineRef, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var refs []PipelineRef
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		// Accept both legacy .pipeline.yaml and current .workflow.yaml extensions.
		// They share the same YAML schema; the rename is UI-level only.
		if !strings.HasSuffix(e.Name(), ".pipeline.yaml") && !strings.HasSuffix(e.Name(), ".workflow.yaml") {
			continue
		}
		path := filepath.Join(dir, e.Name())
		ref, ok := loadPipelineRef(path)
		if !ok {
			continue
		}
		refs = append(refs, ref)
	}
	return refs, nil
}

// loadPipelineRef reads the minimal metadata needed for routing from a single file.
// Returns (ref, false) if the file cannot be read or has no usable name.
func loadPipelineRef(path string) (PipelineRef, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return PipelineRef{}, false
	}

	// Extract leading comment before YAML parse (YAML parsers strip comments).
	leadingComment := extractLeadingComment(data)

	var meta pipelineMeta
	if err := yaml.Unmarshal(data, &meta); err != nil || meta.Name == "" {
		return PipelineRef{}, false
	}

	desc := meta.Description
	if desc == "" {
		desc = leadingComment
	}
	if desc == "" {
		desc = meta.Name
	}

	return PipelineRef{
		Name:           meta.Name,
		Description:    desc,
		Path:           path,
		TriggerPhrases: meta.TriggerPhrases,
	}, true
}

// extractLeadingComment returns the text of the first `# ...` line in data,
// with the `# ` prefix stripped. Returns "" if no leading comment is found.
func extractLeadingComment(data []byte) string {
	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		if text, ok := strings.CutPrefix(line, "#"); ok {
			return strings.TrimSpace(text)
		}
		// First non-blank, non-comment line — stop looking.
		break
	}
	return ""
}
