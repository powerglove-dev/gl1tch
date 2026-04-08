// research_prompt.go loads a workspace's research prompt — the
// free-form markdown file where the user tells gl1tch what counts as
// "needs my attention" in this workspace and what artifact to
// produce for high-attention events.
//
// The research prompt is the single source of truth for:
//
//   1. The attention classifier (pkg/glitchd/attention.go), which
//      feeds this prompt to qwen2.5:7b when deciding whether an
//      event should be flagged `high`, `normal`, or `low`.
//
//   2. The deep-analysis artifact-mode rubric
//      (pkg/glitchd/prompts/deep_analysis_artifact.md), which
//      substitutes this prompt into its template so the tool-using
//      agent knows exactly what artifact to produce for the user.
//
// Storage: `~/.config/glitch/workspaces/<id>/research.md`, next to
// the workspace's collectors.yaml. The global fallback location
// `~/.config/glitch/research.md` is consulted when the workspace id
// is empty or the per-workspace file is missing. Both locations
// fall through to the bundled default at
// `pkg/glitchd/prompts/research_default.md` so a fresh install has
// something sensible to feed the classifier on the first tick.
//
// This file contains NO judgement about what matters — it only
// handles path resolution, file IO, and seeding. The actual rules
// live in the markdown prompt itself, which is the point: the
// judgement belongs to the LLM, not to Go string literals.
package glitchd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ResearchPromptPath returns the absolute path of the research
// prompt file for the given workspace id. Returns the global
// fallback path `~/.config/glitch/research.md` when workspaceID is
// empty. The file itself is NOT created — use EnsureResearchPrompt
// for that.
func ResearchPromptPath(workspaceID string) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(workspaceID) == "" {
		return filepath.Join(home, ".config", "glitch", "research.md"), nil
	}
	return filepath.Join(home, ".config", "glitch", "workspaces",
		workspaceID, "research.md"), nil
}

// LoadResearchPrompt returns the text of the research prompt that
// applies to the given workspace. Resolution order:
//
//  1. Per-workspace file:
//     ~/.config/glitch/workspaces/<id>/research.md
//  2. Global fallback file:
//     ~/.config/glitch/research.md
//  3. Bundled default:
//     pkg/glitchd/prompts/research_default.md
//
// Missing files fall through to the next step. An empty workspace
// id skips step 1. Returns an error only when *all three* sources
// are unreadable (broken install) — callers should treat that as
// fatal for the classifier, since running without a research prompt
// would hardcode judgement back into Go.
func LoadResearchPrompt(workspaceID string) (string, error) {
	// Step 1: per-workspace file.
	if strings.TrimSpace(workspaceID) != "" {
		if p, err := ResearchPromptPath(workspaceID); err == nil {
			if b, err := os.ReadFile(p); err == nil {
				if s := strings.TrimSpace(string(b)); s != "" {
					return s, nil
				}
			}
		}
	}

	// Step 2: global fallback file.
	if p, err := ResearchPromptPath(""); err == nil {
		if b, err := os.ReadFile(p); err == nil {
			if s := strings.TrimSpace(string(b)); s != "" {
				return s, nil
			}
		}
	}

	// Step 3: bundled default via the prompts loader. This is the
	// same mechanism the activity analyzer uses for its rubric, so
	// the default ships with the repo and honors GLITCH_PROMPTS_DIR
	// overrides in tests.
	if s, err := LoadPrompt("research_default"); err == nil {
		if s = strings.TrimSpace(s); s != "" {
			return s, nil
		}
	}

	return "", fmt.Errorf("research prompt unavailable: no per-workspace file, no global file, and no bundled default")
}

// EnsureResearchPrompt writes the bundled default into the
// per-workspace file if the file does not already exist. Used by
// the desktop's "configure workspace" flow so a newly-created
// workspace lands on disk with a seed the user can edit.
//
// Idempotent: existing files are left alone. Empty workspaceID is
// an error — the global fallback is intentionally not auto-seeded
// because it would pre-empt every future workspace's per-workspace
// file.
func EnsureResearchPrompt(workspaceID string) error {
	if strings.TrimSpace(workspaceID) == "" {
		return fmt.Errorf("workspace id required")
	}
	path, err := ResearchPromptPath(workspaceID)
	if err != nil {
		return err
	}
	if _, err := os.Stat(path); err == nil {
		return nil
	}
	seed, err := LoadPrompt("research_default")
	if err != nil {
		return fmt.Errorf("load bundled default: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(seed), 0o644)
}

// WriteResearchPrompt replaces a workspace's research prompt with
// the given content. Used by the desktop editor. No schema — the
// content is whatever markdown the user (or LLM) produces; the
// classifier and artifact-mode rubric are the consumers and they
// get a single free-form string.
//
// Fails fast on an empty workspace id to prevent accidental writes
// to the global fallback through the editor path; callers that
// genuinely want to edit the global file should write it directly.
func WriteResearchPrompt(workspaceID, content string) error {
	if strings.TrimSpace(workspaceID) == "" {
		return fmt.Errorf("workspace id required")
	}
	path, err := ResearchPromptPath(workspaceID)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(content), 0o644)
}

// ResearchEscalationMode is a convenience extractor that reads the
// `escalate:` directive out of a loaded research prompt. Recognized
// values are "on-request" (default — paid polish only when the user
// clicks Escalate) and "auto-high" (paid polish runs automatically
// on every high-attention event).
//
// Unknown or missing values return "on-request" so an empty /
// partial research prompt degrades to the safer default.
//
// This is the one piece of lightweight structured parsing we do
// against the research prompt. Everything else is handed to the LLM
// as free text — the escalation directive is parsed in Go because
// it gates a paid-provider call, and we don't want the decision to
// live inside a prompt the model might hallucinate.
func ResearchEscalationMode(prompt string) string {
	for _, line := range strings.Split(prompt, "\n") {
		line = strings.TrimSpace(line)
		low := strings.ToLower(line)
		if !strings.HasPrefix(low, "escalate:") {
			continue
		}
		val := strings.TrimSpace(strings.TrimPrefix(low, "escalate:"))
		switch val {
		case "auto-high", "auto":
			return "auto-high"
		case "on-request", "manual", "off":
			return "on-request"
		}
	}
	return "on-request"
}
