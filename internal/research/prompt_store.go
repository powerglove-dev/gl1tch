package research

import (
	"bytes"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"text/template"
)

// prompt_store.go is the externalized template loader for the research
// loop. The whole point: stop baking prompt strings into Go so the user
// can `vim ~/.config/glitch/prompts/plan.tmpl` and have the next
// research call pick it up without recompiling. Embedded defaults ship
// with the binary so a fresh install works out of the box; disk
// overrides win on every render.
//
// Why this exists at all: the brain is supposed to learn from itself.
// Hand-tuning planner prompts in Go strings forced a recompile every
// time we wanted to test a copy change, which makes iterative tuning
// (the kind a learning loop NEEDS) impossible to do quickly. With the
// loader, the planner prompt is data — the brain hints reader (next
// commit) will inject `{{.Hints}}` into the same template the user
// can edit, and the system improves without anyone touching Go.
//
// Lookup order per render call:
//
//   1. ~/.config/glitch/prompts/<name>.tmpl       (user override)
//   2. <workspaceDir>/.glitch/prompts/<name>.tmpl (workspace override,
//                                                   not yet wired but
//                                                   reserved in the
//                                                   resolver)
//   3. internal/research/prompts/<name>.tmpl      (embedded default)
//
// Lower numbers win. The default is always available so a misspelled
// override never breaks the loop — it just falls through.

//go:embed prompts/*.tmpl
var embeddedPrompts embed.FS

// PromptName is one of the canonical prompt slots the loop knows about.
// Adding a new slot is a Go change (the loop has to call it); changing
// the body of an existing slot is just a file edit.
type PromptName string

const (
	PromptNamePlan            PromptName = "plan"
	PromptNameDraft           PromptName = "draft"
	PromptNameCritique        PromptName = "critique"
	PromptNameJudge           PromptName = "judge"
	PromptNameSelfConsistency PromptName = "self_consistency"
	PromptNameVerify          PromptName = "verify"
)

// AllPromptNames is the canonical list the `glitch prompts list`
// command iterates. Keep in sync with the consts above.
var AllPromptNames = []PromptName{
	PromptNamePlan,
	PromptNameDraft,
	PromptNameCritique,
	PromptNameJudge,
	PromptNameSelfConsistency,
	PromptNameVerify,
}

// PromptSource describes where a resolved prompt came from. Used by
// the CLI's `glitch prompts list` to show the user whether a slot is
// running on the embedded default or an override.
type PromptSource string

const (
	PromptSourceEmbedded  PromptSource = "embedded"
	PromptSourceUser      PromptSource = "user"
	PromptSourceWorkspace PromptSource = "workspace"
)

// PromptStore is the loader. It is concurrency-safe and stateless
// across calls — it does NOT cache disk reads, because caching would
// defeat the "edit and re-run" loop the externalization exists to
// enable. The cost is one stat + one open per render call, both cheap
// against the latency of the LLM call that follows.
type PromptStore struct {
	// userDir is the per-user override path
	// (~/.config/glitch/prompts) cached at construction so the
	// resolver doesn't re-call os.UserHomeDir on every render.
	userDir string
	// workspaceDir is the optional workspace-scoped override root.
	// Empty when not set; the resolver skips that level entirely.
	workspaceDir string
	// funcMap is the text/template.FuncMap every render gets, so a
	// template can call {{add 1 .Index}} or {{indent 4 .Body}}
	// without each caller having to register the helpers.
	funcMap template.FuncMap
	// mu guards in-memory parsing of overrides — nothing else.
	mu sync.Mutex
}

// NewPromptStore constructs the loader. workspaceDir may be empty (the
// resolver then skips the workspace-scoped override path); production
// callers usually pass the active workspace's primary directory so a
// repo can ship its own prompts under .glitch/prompts/.
func NewPromptStore(workspaceDir string) *PromptStore {
	home, _ := os.UserHomeDir()
	userDir := ""
	if home != "" {
		userDir = filepath.Join(home, ".config", "glitch", "prompts")
	}
	return &PromptStore{
		userDir:      userDir,
		workspaceDir: workspaceDir,
		funcMap:      defaultPromptFuncs(),
	}
}

// Render renders the named prompt with the supplied data. Errors when
// the slot is not in AllPromptNames or when the resolved template
// fails to parse / execute. The error wraps the source so the caller
// can tell whether it was the embedded default or a user override
// that broke (the more common cause once externalization lands).
func (s *PromptStore) Render(name PromptName, data any) (string, error) {
	body, source, err := s.Resolve(name)
	if err != nil {
		return "", err
	}
	s.mu.Lock()
	tmpl, parseErr := template.New(string(name)).Funcs(s.funcMap).Parse(body)
	s.mu.Unlock()
	if parseErr != nil {
		return "", fmt.Errorf("research: prompt %q (%s) parse: %w", name, source, parseErr)
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("research: prompt %q (%s) execute: %w", name, source, err)
	}
	return buf.String(), nil
}

// Resolve walks the lookup order and returns the first match plus the
// source it came from. Exposed (rather than just used internally by
// Render) so the `glitch prompts show` CLI can print the resolved
// body without going through a render call.
func (s *PromptStore) Resolve(name PromptName) (body string, source PromptSource, err error) {
	if !s.known(name) {
		return "", "", fmt.Errorf("research: unknown prompt name %q", name)
	}
	filename := string(name) + ".tmpl"

	// 1. user override
	if s.userDir != "" {
		if data, err := os.ReadFile(filepath.Join(s.userDir, filename)); err == nil {
			return string(data), PromptSourceUser, nil
		}
	}
	// 2. workspace override
	if s.workspaceDir != "" {
		if data, err := os.ReadFile(filepath.Join(s.workspaceDir, ".glitch", "prompts", filename)); err == nil {
			return string(data), PromptSourceWorkspace, nil
		}
	}
	// 3. embedded default
	data, err := fs.ReadFile(embeddedPrompts, "prompts/"+filename)
	if err != nil {
		return "", "", fmt.Errorf("research: embedded prompt %q missing: %w", name, err)
	}
	return string(data), PromptSourceEmbedded, nil
}

// EmbeddedDefault returns the embedded default body for a prompt,
// regardless of any disk overrides. Used by `glitch prompts diff`
// and `glitch prompts edit` (which seeds the override file from the
// default before opening $EDITOR).
func (s *PromptStore) EmbeddedDefault(name PromptName) (string, error) {
	if !s.known(name) {
		return "", fmt.Errorf("research: unknown prompt name %q", name)
	}
	data, err := fs.ReadFile(embeddedPrompts, "prompts/"+string(name)+".tmpl")
	if err != nil {
		return "", fmt.Errorf("research: embedded prompt %q missing: %w", name, err)
	}
	return string(data), nil
}

// UserOverridePath returns the absolute path the user override would
// live at, even if the file does not yet exist. Used by `glitch
// prompts edit` to figure out where to write the seed copy.
func (s *PromptStore) UserOverridePath(name PromptName) string {
	if s.userDir == "" {
		return ""
	}
	return filepath.Join(s.userDir, string(name)+".tmpl")
}

// HasUserOverride reports whether a user override exists on disk for
// name. Used by `glitch prompts list` to render the source column.
func (s *PromptStore) HasUserOverride(name PromptName) bool {
	if s.userDir == "" {
		return false
	}
	_, err := os.Stat(filepath.Join(s.userDir, string(name)+".tmpl"))
	return err == nil
}

func (s *PromptStore) known(name PromptName) bool {
	for _, n := range AllPromptNames {
		if n == name {
			return true
		}
	}
	return false
}

// defaultPromptFuncs is the FuncMap every embedded template can rely
// on. The set is intentionally small: anything more elaborate (e.g.
// markdown rendering, RAG snippet inclusion) belongs in a Go helper
// the loop calls before passing data to Render, not in template
// helpers — keeping the surface tiny means a user-edited template
// never crashes on a missing function the upstream binary forgot to
// register.
func defaultPromptFuncs() template.FuncMap {
	return template.FuncMap{
		"add": func(a, b int) int { return a + b },
		"join": func(parts []string, sep string) string {
			return strings.Join(parts, sep)
		},
		"indent": func(spaces int, s string) string {
			pad := strings.Repeat(" ", spaces)
			lines := strings.Split(s, "\n")
			for i, line := range lines {
				lines[i] = pad + line
			}
			return strings.Join(lines, "\n")
		},
		"trim": strings.TrimSpace,
	}
}

// ErrPromptNotFound is returned by Resolve when the slot is unknown.
// Defined as a sentinel so future tests can match on it.
var ErrPromptNotFound = errors.New("research: prompt not found")
