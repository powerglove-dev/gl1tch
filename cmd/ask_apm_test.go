//go:build !integration

package cmd

import (
	"strings"
	"testing"

	"github.com/8op-org/gl1tch/internal/executor"
	"github.com/8op-org/gl1tch/internal/pipeline"
)

// buildAPMRefs replicates the synthetic ref construction from the routing section
// of ask.go. Extracted here so it can be tested independently.
func buildAPMRefs(mgr *executor.Manager, existingNames map[string]bool) []pipeline.PipelineRef {
	var refs []pipeline.PipelineRef
	for _, ex := range mgr.List() {
		name := ex.Name()
		if !strings.HasPrefix(name, "apm.") {
			continue
		}
		if existingNames[name] {
			continue
		}
		refs = append(refs, pipeline.PipelineRef{
			Name:        name,
			Description: "[apm] " + name + ": " + ex.Description(),
			Path:        "",
		})
	}
	return refs
}

func TestAPMRefsFromManager_APMExecutorsIncluded(t *testing.T) {
	mgr := executor.NewManager()
	_ = mgr.Register(executor.NewCliAdapter("apm.glab-issue", "Triage GitLab issues", "claude", "--print"))
	_ = mgr.Register(executor.NewCliAdapter("apm.glab-mr", "Manage GitLab MRs", "claude", "--print"))
	_ = mgr.Register(executor.NewCliAdapter("claude", "Claude provider", "claude"))

	refs := buildAPMRefs(mgr, map[string]bool{})

	if len(refs) != 2 {
		t.Fatalf("expected 2 APM refs, got %d", len(refs))
	}
	for _, ref := range refs {
		if !strings.HasPrefix(ref.Name, "apm.") {
			t.Errorf("expected apm. prefix on ref name, got %q", ref.Name)
		}
		if !strings.Contains(ref.Description, "[apm]") {
			t.Errorf("expected [apm] prefix in description, got %q", ref.Description)
		}
		if ref.Path != "" {
			t.Errorf("expected empty Path for synthetic APM ref, got %q", ref.Path)
		}
	}
}

func TestAPMRefsFromManager_NoAPMExecutors(t *testing.T) {
	mgr := executor.NewManager()
	_ = mgr.Register(executor.NewCliAdapter("claude", "Claude provider", "claude"))

	refs := buildAPMRefs(mgr, map[string]bool{})
	if len(refs) != 0 {
		t.Errorf("expected 0 APM refs when no apm. executors registered, got %d", len(refs))
	}
}

func TestAPMRefsFromManager_SkipsAlreadyMaterialized(t *testing.T) {
	mgr := executor.NewManager()
	_ = mgr.Register(executor.NewCliAdapter("apm.glab-issue", "Triage GitLab issues", "claude", "--print"))

	// Mark as already having a pipeline file.
	existingNames := map[string]bool{"apm.glab-issue": true}
	refs := buildAPMRefs(mgr, existingNames)

	if len(refs) != 0 {
		t.Errorf("expected 0 synthetic refs when pipeline already materialized, got %d", len(refs))
	}
}

func TestSyntheticAPMRef_DetectedByPath(t *testing.T) {
	// Verify the condition used in the routing dispatch: empty Path + apm. prefix.
	ref := pipeline.PipelineRef{
		Name:        "apm.glab-issue",
		Description: "[apm] apm.glab-issue: Triage GitLab issues",
		Path:        "",
	}

	isSyntheticAPM := ref.Path == "" && strings.HasPrefix(ref.Name, "apm.")
	if !isSyntheticAPM {
		t.Error("expected synthetic APM ref to be detected by empty path + apm. prefix")
	}

	// Real pipeline ref should not be detected as synthetic APM.
	real := pipeline.PipelineRef{
		Name:        "apm.glab-issue",
		Description: "...",
		Path:        "/home/user/Projects/gl1tch/.glitch/workflows/apm.glab-issue.workflow.yaml",
	}
	isRealSyntheticAPM := real.Path == "" && strings.HasPrefix(real.Name, "apm.")
	if isRealSyntheticAPM {
		t.Error("real pipeline ref with path should not be detected as synthetic APM")
	}
}
