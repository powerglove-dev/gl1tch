// prompts_loader.go loads editable prompt templates from disk at
// runtime so operators can tune the LLM rubric without recompiling
// the glitchd binary. This is the practical expression of the
// "AI-first, nothing hardcoded" rule: the *judgment* lives in the
// prompt file, not in Go string literals.
//
// Why not //go:embed? Because the point is editability. Embedded
// prompts would require a rebuild to change, defeating the rule.
// Prompt files ship with the repo at pkg/glitchd/prompts/<name>.md
// and operators can copy them to a user-config override directory
// (~/.config/glitch/prompts/) to customize without touching the
// installed tree.
//
// Search order:
//  1. $GLITCH_PROMPTS_DIR/<name>.md  — explicit override (tests, dev)
//  2. ~/.config/glitch/prompts/<name>.md — per-user override
//  3. <repo>/pkg/glitchd/prompts/<name>.md — bundled default
//
// The loader resolves the bundled default relative to the running
// binary's path on macOS/Linux by walking upward looking for a
// `pkg/glitchd/prompts/` directory. If no default is found the
// caller gets an error — callers are expected to surface that as a
// "install is broken" condition, not silently substitute an empty
// string.
package glitchd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// promptCache memoizes loaded prompts by name. Prompts are small and
// rarely change during a single process lifetime, so re-reading them
// on every analyzer invocation would be wasted IO. Tests that want
// a fresh read can call ResetPromptCache.
var (
	promptCacheMu sync.Mutex
	promptCache   = map[string]string{}
)

// LoadPrompt returns the contents of the prompt template with the
// given short name (no extension, no directory). Returns an error
// when no override AND no bundled default can be found, so the
// caller can refuse to run rather than feed an empty prompt to the
// model.
//
// name is a short identifier like "activity_analyzer" or "judge";
// the loader appends ".md" and searches the resolved prompt
// directories in order.
func LoadPrompt(name string) (string, error) {
	promptCacheMu.Lock()
	if cached, ok := promptCache[name]; ok {
		promptCacheMu.Unlock()
		return cached, nil
	}
	promptCacheMu.Unlock()

	file := name + ".md"
	for _, dir := range promptSearchDirs() {
		path := filepath.Join(dir, file)
		b, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		content := strings.TrimSpace(string(b))
		if content == "" {
			continue
		}
		promptCacheMu.Lock()
		promptCache[name] = content
		promptCacheMu.Unlock()
		return content, nil
	}
	return "", fmt.Errorf("prompt %q not found in any search dir", name)
}

// ResetPromptCache drops the memoized prompt contents so subsequent
// LoadPrompt calls re-read from disk. Tests call this between cases
// that tweak the on-disk file; production callers should not.
func ResetPromptCache() {
	promptCacheMu.Lock()
	promptCache = map[string]string{}
	promptCacheMu.Unlock()
}

// RenderPrompt loads the named template and performs simple
// {{KEY}} substitution using the values map. Unknown placeholders
// are left in place so a template with a typo fails loudly when
// the model sees it rather than silently collapsing the prompt.
func RenderPrompt(name string, values map[string]string) (string, error) {
	tmpl, err := LoadPrompt(name)
	if err != nil {
		return "", err
	}
	out := tmpl
	for k, v := range values {
		out = strings.ReplaceAll(out, "{{"+k+"}}", v)
	}
	return out, nil
}

// promptSearchDirs returns the ordered list of directories to
// search for a named prompt template. First hit wins.
func promptSearchDirs() []string {
	var dirs []string

	// 1. Explicit override — GLITCH_PROMPTS_DIR. Wins over everything
	//    else so tests and dev sessions can point at a scratch
	//    directory without touching user config.
	if d := strings.TrimSpace(os.Getenv("GLITCH_PROMPTS_DIR")); d != "" {
		dirs = append(dirs, d)
	}

	// 2. Per-user override under ~/.config/glitch/prompts.
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		dirs = append(dirs, filepath.Join(home, ".config", "glitch", "prompts"))
	}

	// 3. Bundled default. Try the current working directory first —
	//    during `wails dev` and `go test ./...` the cwd is typically
	//    inside the repo so this lands cleanly. Then walk upward from
	//    the executable looking for `pkg/glitchd/prompts`.
	if cwd, err := os.Getwd(); err == nil {
		if d := findBundledPromptsDir(cwd); d != "" {
			dirs = append(dirs, d)
		}
	}
	if exe, err := os.Executable(); err == nil {
		if d := findBundledPromptsDir(filepath.Dir(exe)); d != "" {
			dirs = append(dirs, d)
		}
	}

	return dirs
}

// findBundledPromptsDir walks upward from start looking for
// `pkg/glitchd/prompts`. Returns the absolute path to that directory
// if found, empty string otherwise. Bounded at 6 levels of ascent
// so it can't run forever on a weird filesystem.
func findBundledPromptsDir(start string) string {
	cur := start
	for i := 0; i < 6; i++ {
		cand := filepath.Join(cur, "pkg", "glitchd", "prompts")
		if info, err := os.Stat(cand); err == nil && info.IsDir() {
			abs, err := filepath.Abs(cand)
			if err == nil {
				return abs
			}
			return cand
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			return ""
		}
		cur = parent
	}
	return ""
}
