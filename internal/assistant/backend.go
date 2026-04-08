// Package assistant exposes a small set of Ollama discovery helpers used by
// cmd/model.go to pick "the best local model." The historical GL1TCH persona
// system prompt and the Backend/Turn/StreamIntro scaffolding that used to
// live here have been deleted as part of the AI-first redesign — no code
// path injects a hardcoded persona into chat any more. If a caller wants
// a system turn, they load a markdown file themselves and hand it to
// capability.Agent.System.
package assistant

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/8op-org/gl1tch/internal/capability"
)

// OllamaAvailable pings the Ollama API with a 500ms timeout.
func OllamaAvailable() bool {
	client := &http.Client{Timeout: 500 * time.Millisecond}
	resp, err := client.Get("http://localhost:11434/api/tags")
	if err != nil {
		return false
	}
	resp.Body.Close()
	return resp.StatusCode == 200
}

// BestOllamaModel queries /api/tags and returns the best available model from
// the preference list. Falls back to capability.DefaultLocalModel if nothing
// matches.
func BestOllamaModel() string {
	preferred := []string{
		capability.DefaultLocalModel, "llama3.2:3b", "llama3.1", "llama3", "mistral",
		"phi3", "phi3:mini", "gemma2", "gemma2:2b",
	}
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get("http://localhost:11434/api/tags")
	if err != nil {
		return capability.DefaultLocalModel
	}
	defer resp.Body.Close()

	var r struct {
		Models []struct {
			Name string `json:"name"`
		} `json:"models"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil || len(r.Models) == 0 {
		return capability.DefaultLocalModel
	}
	available := make(map[string]bool, len(r.Models))
	for _, m := range r.Models {
		available[m.Name] = true
		if idx := strings.Index(m.Name, ":"); idx != -1 {
			available[m.Name[:idx]] = true
		}
	}
	for _, p := range preferred {
		if available[p] {
			return p
		}
	}
	return r.Models[0].Name
}
