package picker

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/adam-stokes/orcai/internal/discovery"
	"github.com/adam-stokes/orcai/internal/plugin"
	"github.com/adam-stokes/orcai/internal/providers"
)

// ModelOption is a selectable model within a provider.
type ModelOption struct {
	ID        string
	Label     string
	Separator bool // visual divider — not selectable
}

// ProviderDef describes one AI provider and its available models.
type ProviderDef struct {
	ID          string
	Label       string
	Models      []ModelOption
	Command     string // actual binary path/name to invoke
	SidecarPath string // path to wrappers YAML; non-empty for sidecar-backed providers
}

// Providers is the base list of built-in providers. All AI providers are
// discovered at runtime via sidecar YAML files in ~/.config/orcai/wrappers/.
var Providers = []ProviderDef{
	{ID: "ollama", Label: "Ollama"},
	{ID: "shell", Label: "Shell"},
}

// queryOllamaModels calls the local Ollama API and returns model names.
// Returns nil if Ollama is not running or has no models.
func queryOllamaModels() []string {
	client := &http.Client{Timeout: 500 * time.Millisecond}
	resp, err := client.Get("http://localhost:11434/api/tags")
	if err != nil {
		return nil
	}
	defer resp.Body.Close()

	var result struct {
		Models []struct {
			Name string `json:"name"`
		} `json:"models"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil
	}
	names := make([]string, 0, len(result.Models))
	for _, m := range result.Models {
		names = append(names, m.Name)
	}
	return names
}

// sidecarMeta holds both model options and kind metadata for a sidecar plugin.
type sidecarMeta struct {
	Models  []ModelOption
	Kind    string
	Command string
}

// loadSidecarMeta scans configDir/wrappers/ and returns a map from plugin
// name to sidecarMeta, capturing both models and kind from each sidecar YAML.
func loadSidecarMeta(configDir string) map[string]sidecarMeta {
	result := make(map[string]sidecarMeta)
	if configDir == "" {
		return result
	}
	wrappersDir := filepath.Join(configDir, "wrappers")
	entries, err := os.ReadDir(wrappersDir)
	if err != nil {
		return result
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".yaml") {
			continue
		}
		adapter, err := plugin.NewCliAdapterFromSidecar(filepath.Join(wrappersDir, e.Name()))
		if err != nil {
			continue
		}
		models := make([]ModelOption, 0, len(adapter.Models()))
		for _, m := range adapter.Models() {
			models = append(models, ModelOption{ID: m.ID, Label: m.Label})
		}
		cmd := adapter.Command()
		// If no models declared in sidecar, try --list-models autodetect.
		if len(models) == 0 && cmd != "" {
			models = autodetectModels(cmd)
		}
		result[adapter.Name()] = sidecarMeta{Models: models, Kind: adapter.Kind(), Command: cmd}
	}
	return result
}

// autodetectModels runs `<cmd> --list-models` with a 2-second timeout and
// parses stdout as JSON: [{"id":"...","label":"..."}].
// Returns nil (not an error) on any failure so startup is never blocked.
func autodetectModels(cmd string) []ModelOption {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, cmd, "--list-models").Output()
	if err != nil {
		return nil
	}
	out = bytes.TrimSpace(out)
	if len(out) == 0 {
		return nil
	}
	var raw []struct {
		ID    string `json:"id"`
		Label string `json:"label"`
	}
	if err := json.Unmarshal(out, &raw); err != nil {
		return nil
	}
	models := make([]ModelOption, 0, len(raw))
	for _, r := range raw {
		lbl := r.Label
		if lbl == "" {
			lbl = r.ID
		}
		models = append(models, ModelOption{ID: r.ID, Label: lbl})
	}
	return models
}

// buildProviders returns a filtered, runtime-enriched provider list driven by
// the plugin Manager/discovery layer:
//   - only includes providers found via discovery (native plugins + CLI wrappers)
//   - shell is always included
//   - reads model metadata from ~/.config/orcai/wrappers/<name>.yaml sidecars
//   - falls back to runtime Ollama query if the ollama sidecar declares no models
//   - appends any discovered plugins not in the static Providers list
func buildProviders() []ProviderDef {
	configDir := orcaiConfigDir()
	ollamaModels := queryOllamaModels()
	sidecarData := loadSidecarMeta(configDir)

	// Discover available plugins (native gRPC plugins + CLI wrappers; skip pipelines).
	discovered := make(map[string]bool)
	type extraEntry struct {
		name        string
		sidecarPath string
		command     string
	}
	var extras []extraEntry

	if configDir != "" {
		if plugins, err := discovery.Discover(configDir); err == nil {
			for _, p := range plugins {
				if p.Type == discovery.TypePipeline {
					continue
				}
				discovered[p.Name] = true
				if p.Type == discovery.TypeNative || p.Type == discovery.TypeCLIWrapper {
					extras = append(extras, extraEntry{name: p.Name, sidecarPath: p.SidecarPath, command: p.Command})
				}
			}
		}
	}

	// Build a name→extraEntry lookup so static providers can inherit SidecarPath.
	extrasByName := make(map[string]extraEntry, len(extras))
	for _, e := range extras {
		extrasByName[e.name] = e
	}

	var out []ProviderDef
	for _, p := range Providers {
		switch p.ID {
		case "shell":
			// always available — no binary to check
		case "ollama":
			if !discovered[p.ID] {
				continue
			}
			// Prefer sidecar-declared models; fall back to runtime query.
			if models := sidecarData[p.ID].Models; len(models) > 0 {
				p.Models = models
			} else {
				p = injectOllamaModels(p, ollamaModels)
			}
			if cmd := sidecarData[p.ID].Command; cmd != "" {
				p.Command = cmd
			}
		default:
			if !discovered[p.ID] {
				continue
			}
			if cmd := sidecarData[p.ID].Command; cmd != "" {
				p.Command = cmd
			}
		}
		// Propagate SidecarPath from the discovery layer so that the
		// pipelineRunCmd sidecar-skip guard fires correctly and avoids
		// "already registered" warnings.
		if e, ok := extrasByName[p.ID]; ok && p.SidecarPath == "" {
			p.SidecarPath = e.sidecarPath
		}
		out = append(out, p)
	}

	// Load providers registry for display names and fallback model metadata.
	var reg *providers.Registry
	if configDir != "" {
		reg, _ = providers.NewRegistry(filepath.Join(configDir, "providers"))
	}

	// Append discovered plugins that are not in the static Providers list.
	staticIDs := make(map[string]bool, len(Providers))
	for _, p := range Providers {
		staticIDs[p.ID] = true
	}
	for _, e := range extras {
		if staticIDs[e.name] {
			continue
		}
		meta := sidecarData[e.name]
		// Only include agent-kind plugins in the agent runner provider list.
		if meta.Kind != "" && meta.Kind != "agent" {
			continue
		}
		models := meta.Models
		label := e.name
		if reg != nil {
			if profile, ok := reg.Get(e.name); ok {
				if profile.DisplayName != "" {
					label = profile.DisplayName
				}
				// Use bundled profile models when no sidecar models are declared.
				if len(models) == 0 {
					for _, m := range profile.Models {
						lbl := m.Display
						if lbl == "" {
							lbl = m.ID
						}
						models = append(models, ModelOption{ID: m.ID, Label: lbl})
					}
				}
			}
		}
		cmd := e.command
		if cmd == "" {
			cmd = meta.Command
		}
		out = append(out, ProviderDef{
			ID:          e.name,
			Label:       label,
			Models:      models,
			Command:     cmd,
			SidecarPath: e.sidecarPath,
		})
	}

	return out
}

// BuildProviders returns the runtime-filtered, model-enriched provider list,
// excluding the shell provider (not relevant for pipeline steps).
func BuildProviders() []ProviderDef {
	all := buildProviders()
	out := make([]ProviderDef, 0, len(all))
	for _, p := range all {
		if p.ID != "shell" {
			out = append(out, p)
		}
	}
	return out
}

// pipelineLaunchArgs maps provider IDs to the extra CLI flags needed to invoke
// them in non-interactive (pipeline) mode.
var pipelineLaunchArgs = map[string][]string{}

// PipelineLaunchArgs returns the fixed CLI args to prepend when a provider is
// invoked as a non-interactive pipeline executor.
// Returns nil if no extra args are required for the given provider.
func PipelineLaunchArgs(providerID string) []string {
	return pipelineLaunchArgs[providerID]
}

// injectOllamaModels appends Ollama model entries to the ollama provider.
func injectOllamaModels(p ProviderDef, ollamaModels []string) ProviderDef {
	if p.ID != "ollama" || len(ollamaModels) == 0 {
		return p
	}
	models := make([]ModelOption, 0, len(ollamaModels))
	for _, m := range ollamaModels {
		models = append(models, ModelOption{ID: m, Label: m})
	}
	p.Models = models
	return p
}

// ── Worktree helpers ──────────────────────────────────────────────────────────

// expandPath expands a leading ~ to the user's home directory.
func expandPath(path string) string {
	if path == "" {
		return path
	}
	if path == "~" || strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err == nil {
			path = home + path[1:]
		}
	}
	return path
}

// findGitRoot returns the top-level git directory containing path, or "".
func findGitRoot(path string) string {
	if path == "" {
		return ""
	}
	path = expandPath(path)
	out, err := exec.Command("git", "-C", path, "rev-parse", "--show-toplevel").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// GetOrCreateWorktreeFrom creates a git worktree for sessionName rooted at
// basePath and returns its path plus the repo root. Returns ("", "") if
// basePath is empty or not inside a git repo, or ("", repoRoot) if worktree
// creation fails — callers fall back to the repo root directory.
func GetOrCreateWorktreeFrom(basePath, sessionName string) (worktreePath, repoRoot string) {
	repoRoot = findGitRoot(basePath)
	if repoRoot == "" {
		return "", ""
	}

	// Place the worktree inside <repoRoot>/.worktrees/<sessionName>
	worktreePath = filepath.Join(repoRoot, ".worktrees", sessionName)

	// Reuse an existing worktree path rather than erroring.
	if _, err := os.Stat(worktreePath); err == nil {
		return worktreePath, repoRoot
	}

	// Try to create with a named branch so sessions are traceable.
	branch := "orcai/" + sessionName
	if err := exec.Command("git", "-C", repoRoot, "worktree", "add", worktreePath, "-b", branch).Run(); err != nil {
		// Branch already exists or some other issue — fall back to detached HEAD.
		if err2 := exec.Command("git", "-C", repoRoot, "worktree", "add", "--detach", worktreePath).Run(); err2 != nil {
			return "", repoRoot // worktree creation failed; caller uses repoRoot
		}
	}
	return worktreePath, repoRoot
}

// CopyDotEnv copies .env from src directory to dst directory if the file
// exists in src and does not already exist in dst.
func CopyDotEnv(src, dst string) {
	srcFile := filepath.Join(src, ".env")
	dstFile := filepath.Join(dst, ".env")
	if _, err := os.Stat(srcFile); err != nil {
		return // no .env to copy
	}
	if _, err := os.Stat(dstFile); err == nil {
		return // dst already has a .env
	}
	data, err := os.ReadFile(srcFile)
	if err != nil {
		return
	}
	os.WriteFile(dstFile, data, 0o600) //nolint:errcheck
}

// ── Session helpers ───────────────────────────────────────────────────────────

// WindowEntry represents a running orcai tmux window.
type WindowEntry struct {
	Index string
	Name  string
}

// systemWindows are orcai UI windows that should not appear in the existing sessions list.
var systemWindows = map[string]bool{
	"ORCAI":    true,
	"_sidebar": true,
	"_welcome": true,
}

// ParseWindowList parses the output of:
//
//	tmux list-windows -t orcai -F "#{window_index} #{window_name}"
//
// and returns non-system windows.
func ParseWindowList(output string) []WindowEntry {
	var entries []WindowEntry
	for _, line := range strings.Split(strings.TrimSpace(output), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		idx, name, ok := strings.Cut(line, " ")
		if !ok {
			continue
		}
		if systemWindows[name] {
			continue
		}
		entries = append(entries, WindowEntry{Index: idx, Name: name})
	}
	return entries
}

// selectableModels returns only non-separator model entries from a provider.
func selectableModels(p ProviderDef) []ModelOption {
	var out []ModelOption
	for _, mo := range p.Models {
		if !mo.Separator {
			out = append(out, mo)
		}
	}
	return out
}
