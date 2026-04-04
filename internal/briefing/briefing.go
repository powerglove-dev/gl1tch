// Package briefing provides the morning-briefing pipeline template and
// the enable/disable helpers that wire it into the cron scheduler.
package briefing

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/8op-org/gl1tch/internal/cron"
)

const pipelineYAML = `name: morning-briefing
version: "1"
description: "Daily briefing — brain digest, weather, and GitHub activity"
use_brain: true

steps:

  # ── 1. Brain digest ────────────────────────────────────────────────────────
  - id: brain-digest
    executor: ollama
    use_brain: true
    prompt: |
      You have access to brain notes from recent sessions.
      Write a 1-2 sentence digest of what has been happening recently —
      projects, themes, or recurring topics. Be specific, not generic.
      If brain notes are sparse, say so briefly.

  # ── 2. Weather ─────────────────────────────────────────────────────────────
  - id: weather
    executor: shell
    vars:
      cmd: >-
        glitch-weather 2>/dev/null | head -3
        || echo "weather unavailable"

  # ── 3. GitHub notifications ─────────────────────────────────────────────────
  - id: github-activity
    executor: shell
    vars:
      cmd: >-
        gh api /notifications -q '.[].subject.title' 2>/dev/null | head -5
        || echo "no github activity"

  # ── 4. Format the briefing ──────────────────────────────────────────────────
  - id: format
    executor: ollama
    needs: [brain-digest, weather, github-activity]
    prompt: |
      Write a brief morning briefing (4-6 sentences, lowercase, no drama).
      Weave together what you know from:

      Brain digest: {{steps.brain-digest.output}}
      Weather: {{steps.weather.output}}
      GitHub: {{steps.github-activity.output}}

      Start with the most relevant item. End with one actionable suggestion
      for the day based on what's in the brain.

  # ── 5. Output ───────────────────────────────────────────────────────────────
  - id: output
    executor: builtin.log
    needs: [format]
    vars:
      message: "morning briefing:\n\n{{steps.format.output}}"
`

const cronName = "morning-briefing"
const cronSchedule = "0 8 * * *" // 8 AM daily

// PipelinePath returns the path where the morning-briefing pipeline YAML is installed.
func PipelinePath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "glitch", "pipelines", "morning-briefing.pipeline.yaml"), nil
}

// Enable installs the pipeline file (if absent) and upserts a cron entry.
// Returns the pipeline path and the updated cron entry count.
func Enable() (string, int, error) {
	ppath, err := PipelinePath()
	if err != nil {
		return "", 0, fmt.Errorf("briefing: resolve pipeline path: %w", err)
	}

	// Write pipeline YAML only if not already present — never clobber edits.
	if _, err := os.Stat(ppath); os.IsNotExist(err) {
		if err := os.MkdirAll(filepath.Dir(ppath), 0o755); err != nil {
			return "", 0, fmt.Errorf("briefing: create pipelines dir: %w", err)
		}
		if err := os.WriteFile(ppath, []byte(pipelineYAML), 0o644); err != nil {
			return "", 0, fmt.Errorf("briefing: write pipeline: %w", err)
		}
	}

	// Load, upsert, and save the cron config.
	home, _ := os.UserHomeDir()
	cronPath := filepath.Join(home, ".config", "glitch", "cron.yaml")
	entries, err := cron.LoadConfigFrom(cronPath)
	if err != nil {
		return "", 0, fmt.Errorf("briefing: load cron config: %w", err)
	}
	entries = cron.UpsertEntry(entries, cron.Entry{
		Name:     cronName,
		Schedule: cronSchedule,
		Kind:     "pipeline",
		Target:   ppath,
	})
	if err := cron.SaveConfigTo(cronPath, entries); err != nil {
		return "", 0, fmt.Errorf("briefing: save cron config: %w", err)
	}
	return ppath, len(entries), nil
}

// Disable removes the morning-briefing cron entry. The pipeline file is kept.
func Disable() (int, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return 0, fmt.Errorf("briefing: home dir: %w", err)
	}
	cronPath := filepath.Join(home, ".config", "glitch", "cron.yaml")
	entries, err := cron.LoadConfigFrom(cronPath)
	if err != nil {
		return 0, fmt.Errorf("briefing: load cron config: %w", err)
	}
	var kept []cron.Entry
	for _, e := range entries {
		if e.Name != cronName {
			kept = append(kept, e)
		}
	}
	if len(kept) == len(entries) {
		return len(kept), nil // not found, no-op
	}
	if err := cron.SaveConfigTo(cronPath, kept); err != nil {
		return 0, fmt.Errorf("briefing: save cron config: %w", err)
	}
	return len(kept), nil
}

// IsEnabled reports whether the morning-briefing cron entry exists.
func IsEnabled() bool {
	entries, err := cron.LoadConfig()
	if err != nil {
		return false
	}
	for _, e := range entries {
		if e.Name == cronName {
			return true
		}
	}
	return false
}
