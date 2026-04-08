// Ollama helpers shared across text-only agent plugins.
//
// Both the opencode and github-copilot plugins route "ollama/<name>" model
// strings through a local Ollama daemon. Keeping the pull/list logic here
// means neither plugin has to duplicate the tag-matching rules, and a fix
// to one fixes both.
package glitchctx

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"strings"
	"time"
)

// OllamaPrefix is the provider/model scheme used throughout gl1tch to mark
// a model as local. Exported so plugins can share a single literal.
const OllamaPrefix = "ollama/"

// DefaultOllamaBaseURL is the address Ollama listens on by default. Callers
// that need the OpenAI-compatible shim should append "/v1" themselves —
// /api/tags lives on the root, and the copilot BYOK endpoint lives on /v1,
// so this stays deliberately un-suffixed.
const DefaultOllamaBaseURL = "http://localhost:11434"

// StripOllamaPrefix returns the bare Ollama model name with the "ollama/"
// scheme removed. If the string has no prefix it is returned unchanged, so
// callers can pass raw user input through safely.
func StripOllamaPrefix(model string) string {
	return strings.TrimPrefix(model, OllamaPrefix)
}

// IsOllamaModel reports whether a model string targets local Ollama.
func IsOllamaModel(model string) bool {
	return strings.HasPrefix(model, OllamaPrefix)
}

// PullOllamaModel ensures the named Ollama model is resident. A leading
// "ollama/" is stripped if present. No-ops for non-ollama strings so the
// function can be called unconditionally from plugin run paths.
//
// Behaviour:
//  1. `ollama list` is consulted first; exact match OR base-name match
//     (":" tag stripped) counts as already-present.
//  2. If absent, `ollama pull <name>` runs with progress streamed to stderr.
//  3. A pull error is ignored if a second `ollama list` check shows the
//     model present anyway — this covers locally-created aliases that have
//     no registry entry (e.g. modelfiles created via `ollama create`).
func PullOllamaModel(model string, stderr io.Writer) error {
	if !IsOllamaModel(model) {
		return nil
	}
	name := StripOllamaPrefix(model)

	if ollamaHasModel(name) {
		return nil
	}

	fmt.Fprintf(stderr, "Ensuring Ollama model %q is available — pulling if needed...\n", name)
	cmd := exec.Command("ollama", "pull", name)
	cmd.Stdout = stderr
	cmd.Stderr = stderr
	if err := cmd.Run(); err != nil {
		// Retry the list check: a pull may fail for a locally-created
		// alias that is nonetheless present on disk.
		if ollamaHasModel(name) {
			return nil
		}
		return err
	}
	return nil
}

// ollamaHasModel shells out to `ollama list` and reports whether name is
// among the installed tags. Returns false on any parse error.
func ollamaHasModel(name string) bool {
	out, err := exec.Command("ollama", "list").Output()
	if err != nil {
		return false
	}
	for i, line := range strings.Split(string(out), "\n") {
		if i == 0 && strings.HasPrefix(strings.TrimSpace(line), "NAME") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		listed := fields[0]
		if listed == name {
			return true
		}
		if idx := strings.Index(listed, ":"); idx >= 0 && listed[:idx] == name {
			return true
		}
	}
	return false
}

// QueryOllamaModels calls the local Ollama HTTP API and returns installed
// model names ("llama3.2:latest", "qwen2.5:7b", …). The returned slice is
// empty (not nil-panic) when Ollama is unreachable — callers should treat
// an empty list as "local provider unavailable, skip merging".
//
// A short 500ms timeout is intentional: this runs inside plugin startup
// paths (`--list-models`) that block model pickers in the TUI.
func QueryOllamaModels() []string {
	client := &http.Client{Timeout: 500 * time.Millisecond}
	resp, err := client.Get(DefaultOllamaBaseURL + "/api/tags")
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
