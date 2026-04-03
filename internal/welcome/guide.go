package welcome

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

const glitchSystemPrompt = `You are GL1TCH — a veteran underground hacker from the early 90s BBS scene.
You speak in lowercase most of the time. You use old-school slang: l33t, phreaking, handle, the matrix, jacking in, BBS, sysop, warez, packet.
You reference WarGames, Hackers (1995), Neuromancer, Phrack zine, 2600 magazine, Gibson, the net.
You are guiding a new user through GLITCH — a tmux-powered AI workspace.

GLITCH feature knowledge you have:
LAYOUT: three-column deck (window 0). left = pipeline list + signal inbox. center = active agents + send panel. right = activity feed / job logs.
JUMP WINDOW: ^spc j opens a floating overlay — sysop tools (brain, pipelines, prompts) on the left, active jobs on the right. navigate between running agents from here.
KEY BINDINGS: ^spc j=jump, ^spc p=pipeline builder, ^spc b=brain editor, ^spc n=new agent from clipboard, Esc=back/cancel.
PIPELINES: YAML files in ~/.config/glitch/pipelines/. each step has: name, provider, system_prompt, and optional brain tags. steps chain output automatically. press ^spc p to open the pipeline builder TUI (left=list, right=editor+test runner).
PROVIDERS: local models via ollama (e.g. ollama/llama3.2, ollama/mistral, ollama/codestral). cloud models via claude (claude-sonnet-4-6, claude-opus-4-6, claude-haiku-4-5). no API key setup is discussed here.
SEND PANEL: in the center column, type a message and send it directly to a running agent. good for steering mid-task.
INBOX: signals from agents appear in the left column inbox — notifications, questions, completed steps.
CWD CONTEXT: each pipeline job runs with a working directory. brain notes are scoped per-cwd so your AI learns each project separately.
BRAIN SYSTEM: agents output <brain type="..." title="..." tags="...">content</brain> blocks. glitch extracts, embeds as vectors, stores in local SQLite. on future runs, relevant notes are auto-injected as context. types: research, architecture, preference, task, reference. press ^spc b to browse/edit.
CRON JOBS: pipelines can be scheduled with cron syntax in the config — daily digest, nightly code review, morning standup prep.
REAL-WORLD EXAMPLES:
  - code review pipeline: step 1 reads git diff, step 2 uses claude to analyze, step 3 writes brain note tagged "review,go"
  - bug hunt chain: step 1 runs test suite, step 2 feeds failures to LLM, step 3 proposes fixes, step 4 applies them
  - codebase onboarding: pipeline that reads key files and builds a brain note map of the architecture
  - daily digest: cron pipeline at 08:00 that summarizes open issues + recent commits into a brain note
  - research pipeline: step 1 fetches web content, step 2 summarizes, step 3 stores in brain with tags

Keep responses SHORT (4-7 sentences max). Be punchy, enthusiastic, a little chaotic but helpful.
Occasionally use ASCII elements like -=[ ]=-, >>, ||, or simple dividers.
Never use markdown headers, bullet lists, or bold/italic formatting. Write in flowing sentences.
Never break character. You are GL1TCH. You know everything about GLITCH.`

// guideMessage is a single conversation turn.
type guideMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// Guide manages the SYSOP persona and LLM communication via ollama.
type Guide struct {
	model   string
	baseURL string
	history []guideMessage
}

// NewGuide creates a Guide with the given ollama model name.
func NewGuide(modelName string) *Guide {
	return &Guide{
		model:   modelName,
		baseURL: "http://localhost:11434",
		history: []guideMessage{
			{Role: "system", Content: glitchSystemPrompt},
		},
	}
}

// OllamaAvailable returns true if the ollama API is reachable.
func OllamaAvailable() bool {
	resp, err := http.Get("http://localhost:11434/api/tags") //nolint:noctx
	if err != nil {
		return false
	}
	resp.Body.Close()
	return resp.StatusCode == 200
}

// BestModel picks the first available ollama model from a preference list,
// falling back to whatever is first in the list if none match.
func BestModel() string {
	preferred := []string{
		"llama3.2", "llama3.2:3b", "llama3.1", "llama3", "mistral",
		"phi3", "phi3:mini", "gemma2", "gemma2:2b",
	}
	type tagsResp struct {
		Models []struct {
			Name string `json:"name"`
		} `json:"models"`
	}
	resp, err := http.Get("http://localhost:11434/api/tags") //nolint:noctx
	if err != nil {
		return "llama3.2"
	}
	defer resp.Body.Close()
	var r tagsResp
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil || len(r.Models) == 0 {
		return "llama3.2"
	}
	available := make(map[string]bool, len(r.Models))
	for _, m := range r.Models {
		available[m.Name] = true
		// Also index base name (without tag)
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

// StreamResponse sends a message to the LLM and returns a channel of token strings.
// The channel is closed when the full response is received.
// userMsg may be empty for the initial phase opener.
func (g *Guide) StreamResponse(phase Phase, userMsg string) (<-chan string, error) {
	if userMsg != "" {
		g.history = append(g.history, guideMessage{Role: "user", Content: userMsg})
	}

	// Build messages: base system + phase context + conversation history
	messages := []guideMessage{g.history[0]} // base system prompt
	if ctx := phaseSystemContext[phase]; ctx != "" {
		messages = append(messages, guideMessage{Role: "system", Content: ctx})
	}
	messages = append(messages, g.history[1:]...)

	body, err := json.Marshal(map[string]any{
		"model":      g.model,
		"messages":   messages,
		"stream":     true,
		"keep_alive": -1,
	})
	if err != nil {
		return nil, fmt.Errorf("guide: marshal: %w", err)
	}

	resp, err := http.Post(g.baseURL+"/api/chat", "application/json", bytes.NewReader(body)) //nolint:noctx
	if err != nil {
		return nil, fmt.Errorf("guide: post: %w", err)
	}

	ch := make(chan string, 64)
	go func() {
		defer close(ch)
		defer resp.Body.Close()

		var fullContent strings.Builder
		scanner := bufio.NewScanner(resp.Body)
		for scanner.Scan() {
			line := scanner.Text()
			if line == "" {
				continue
			}
			var event struct {
				Message struct {
					Content string `json:"content"`
				} `json:"message"`
				Done bool `json:"done"`
			}
			if err := json.Unmarshal([]byte(line), &event); err != nil {
				continue
			}
			if event.Message.Content != "" {
				ch <- event.Message.Content
				fullContent.WriteString(event.Message.Content)
			}
			if event.Done {
				break
			}
		}
		// Persist the full response into conversation history.
		if fullContent.Len() > 0 {
			g.history = append(g.history, guideMessage{
				Role:    "assistant",
				Content: fullContent.String(),
			})
		}
	}()

	return ch, nil
}
