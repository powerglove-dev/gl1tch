// Package systemprompts loads user-customizable system prompt files from
// ~/.config/glitch/prompts/, falling back to embedded defaults when absent.
// All four core prompts (brain-write, pipeline-generator, prompt-builder,
// clarify) are installed on first run so users can edit them in place.
package systemprompts

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sync"
)

var cache sync.Map // map[string]string — populated on first Load per name

// Load returns the system prompt named name (without the .md extension).
// It checks ~/.config/glitch/prompts/<name>.md first; if absent or unreadable,
// the embedded default is returned. The result is cached after first load.
func Load(name string) string {
	if v, ok := cache.Load(name); ok {
		return v.(string)
	}
	content := load(name)
	cache.Store(name, content)
	return content
}

func load(name string) string {
	home, err := os.UserHomeDir()
	if err == nil {
		userPath := filepath.Join(home, ".config", "glitch", "prompts", name+".md")
		if data, err := os.ReadFile(userPath); err == nil {
			return string(data)
		}
	}
	return embedded(name)
}

// embedded reads the named prompt from the embedded defaults FS.
func embedded(name string) string {
	data, err := defaultFS.ReadFile("defaults/" + name + ".md")
	if err != nil {
		fmt.Fprintf(os.Stderr, "[systemprompts] missing embedded default %q: %v\n", name, err)
		return ""
	}
	return string(data)
}

// EnsureInstalled copies all embedded default prompts to cfgDir/prompts/.
// Files that already exist are skipped (never overwritten). Errors are
// non-fatal: a warning is printed but the function continues.
func EnsureInstalled(cfgDir string) error {
	promptsDir := filepath.Join(cfgDir, "prompts")
	if err := os.MkdirAll(promptsDir, 0o755); err != nil {
		return fmt.Errorf("systemprompts: create prompts dir: %w", err)
	}

	entries, err := fs.ReadDir(defaultFS, "defaults")
	if err != nil {
		return fmt.Errorf("systemprompts: read embedded defaults: %w", err)
	}

	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		dst := filepath.Join(promptsDir, e.Name())
		// O_EXCL: skip if already exists — never clobber user edits.
		f, err := os.OpenFile(dst, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
		if err != nil {
			if os.IsExist(err) {
				continue
			}
			fmt.Fprintf(os.Stderr, "glitch: warning: install prompt %s: %v\n", e.Name(), err)
			continue
		}
		data, _ := defaultFS.ReadFile("defaults/" + e.Name())
		if _, err := f.Write(data); err != nil {
			fmt.Fprintf(os.Stderr, "glitch: warning: write prompt %s: %v\n", e.Name(), err)
		}
		f.Close()
	}
	return nil
}

// InvalidateCache clears the in-memory prompt cache. Useful in tests.
func InvalidateCache() {
	cache.Range(func(k, _ any) bool { cache.Delete(k); return true })
}

// names used by callers — kept as typed constants to avoid typos.
const (
	BrainWrite        = "brain-write"
	PipelineGenerator = "pipeline-generator"
	PromptBuilder     = "prompt-builder"
	Clarify           = "clarify"
)

