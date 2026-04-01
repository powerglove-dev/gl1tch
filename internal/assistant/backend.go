// Package assistant implements the GL1TCH AI assistant backend and TUI model.
package assistant

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"strings"
	"time"

	"github.com/8op-org/gl1tch/internal/picker"
)

// glitchSystemPrompt is the GLITCH persona system prompt used by all backends.
const glitchSystemPrompt = `You are GL1TCH — a veteran underground hacker from the early 90s BBS scene.
You speak in lowercase most of the time. You use old-school slang: l33t, phreaking, handle, the matrix, jacking in, BBS, sysop, warez, packet.
You reference WarGames, Hackers (1995), Neuromancer, Phrack zine, 2600 magazine, Gibson, the net.
You are guiding a new user through GLITCH — a tmux-powered AI workspace.

GLITCH feature knowledge you have:
LAYOUT: three-column switchboard (window 0). left = pipeline list + signal inbox. center = active agents + send panel. right = activity feed / job logs.
JUMP WINDOW: ^spc j opens a floating overlay — sysop tools (brain, pipelines, prompts) on the left, active jobs on the right. navigate between running agents from here.
KEY BINDINGS: ^spc j=jump, ^spc p=pipeline builder, ^spc b=brain editor, ^spc n=new agent from clipboard, ^spc a=GLITCH assistant, Esc=back/cancel.
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

// Turn is a single conversation turn stored in the model (for history passing).
type Turn struct {
	Role string // "user" | "assistant"
	Text string
}

// Backend is the streaming LLM interface for GLITCH.
type Backend interface {
	Name() string
	// StreamIntro sends a first-run greeting (no visible user message).
	StreamIntro(ctx context.Context) (<-chan string, error)
	// Stream sends userMsg with the given history and returns a token channel.
	Stream(ctx context.Context, history []Turn, userMsg string) (<-chan string, error)
	Close()
}

// ── OllamaBackend ─────────────────────────────────────────────────────────────

// OllamaBackend streams from a local Ollama HTTP server.
type OllamaBackend struct {
	model   string
	baseURL string
}

// ollamaMsg is the wire format for a single Ollama chat message.
type ollamaMsg struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// NewOllamaBackend creates an OllamaBackend for the given model name.
func NewOllamaBackend(modelName string) *OllamaBackend {
	return &OllamaBackend{
		model:   modelName,
		baseURL: "http://localhost:11434",
	}
}

// Name implements Backend.
func (b *OllamaBackend) Name() string { return "ollama/" + b.model }

// StreamIntro implements Backend — sends an in-character greeting prompt.
func (b *OllamaBackend) StreamIntro(ctx context.Context) (<-chan string, error) {
	introContext := "Greet the user. Introduce yourself as GLITCH in character. Ask what they want to build or automate. Keep it under 5 sentences."
	msgs := []ollamaMsg{
		{Role: "system", Content: glitchSystemPrompt},
		{Role: "system", Content: introContext},
	}
	return b.stream(ctx, msgs)
}

// Stream implements Backend.
func (b *OllamaBackend) Stream(ctx context.Context, history []Turn, userMsg string) (<-chan string, error) {
	msgs := []ollamaMsg{
		{Role: "system", Content: glitchSystemPrompt},
	}
	for _, t := range history {
		role := t.Role
		if role == "assistant" {
			role = "assistant"
		}
		msgs = append(msgs, ollamaMsg{Role: role, Content: t.Text})
	}
	msgs = append(msgs, ollamaMsg{Role: "user", Content: userMsg})
	return b.stream(ctx, msgs)
}

// stream sends the messages to Ollama and streams back tokens.
func (b *OllamaBackend) stream(ctx context.Context, msgs []ollamaMsg) (<-chan string, error) {
	body, err := json.Marshal(map[string]any{
		"model":    b.model,
		"messages": msgs,
		"stream":   true,
	})
	if err != nil {
		return nil, fmt.Errorf("ollama: marshal: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		b.baseURL+"/api/chat", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("ollama: request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("ollama: post: %w", err)
	}

	ch := make(chan string, 64)
	go func() {
		defer close(ch)
		defer resp.Body.Close()

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
				select {
				case ch <- event.Message.Content:
				case <-ctx.Done():
					return
				}
			}
			if event.Done {
				break
			}
		}
	}()

	return ch, nil
}

// Close implements Backend (no-op for HTTP).
func (b *OllamaBackend) Close() {}

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
// the preference list. Falls back to "llama3.2" if nothing matches.
func BestOllamaModel() string {
	preferred := []string{
		"llama3.2", "llama3.2:3b", "llama3.1", "llama3", "mistral",
		"phi3", "phi3:mini", "gemma2", "gemma2:2b",
	}
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get("http://localhost:11434/api/tags")
	if err != nil {
		return "llama3.2"
	}
	defer resp.Body.Close()

	var r struct {
		Models []struct {
			Name string `json:"name"`
		} `json:"models"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil || len(r.Models) == 0 {
		return "llama3.2"
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

// ── CLIBackend ────────────────────────────────────────────────────────────────

// CLIBackend wraps a provider CLI binary, passing conversation via stdin.
type CLIBackend struct {
	name    string
	command string
	args    []string
}

// NewCLIBackend creates a CLIBackend for the given command and args.
func NewCLIBackend(name, command string, args []string) *CLIBackend {
	return &CLIBackend{name: name, command: command, args: args}
}

// Name implements Backend.
func (b *CLIBackend) Name() string { return b.name }

// StreamIntro implements Backend — calls Stream with nil history and intro trigger.
func (b *CLIBackend) StreamIntro(ctx context.Context) (<-chan string, error) {
	return b.Stream(ctx, nil,
		"Greet the user. Introduce yourself as GLITCH in character. Ask what they want to build or automate. Keep it under 5 sentences.")
}

// Stream implements Backend — formats conversation as text and pipes to CLI stdin.
func (b *CLIBackend) Stream(ctx context.Context, history []Turn, userMsg string) (<-chan string, error) {
	var sb strings.Builder
	sb.WriteString("[SYSTEM]\n")
	sb.WriteString(glitchSystemPrompt)
	sb.WriteString("\n\n[CONVERSATION]\n")
	for _, t := range history {
		switch t.Role {
		case "user":
			sb.WriteString("USER: ")
		default:
			sb.WriteString("GLITCH: ")
		}
		sb.WriteString(t.Text)
		sb.WriteString("\n")
	}
	sb.WriteString("USER: ")
	sb.WriteString(userMsg)
	sb.WriteString("\n")

	input := sb.String()

	pr, pw := io.Pipe()
	ch := make(chan string, 64)

	go func() {
		defer close(ch)
		defer pr.Close()

		cmd := exec.CommandContext(ctx, b.command, b.args...)
		cmd.Stdin = strings.NewReader(input)
		cmd.Stdout = pw
		cmd.Stderr = pw

		if err := cmd.Start(); err != nil {
			pw.CloseWithError(err)
			return
		}

		// Read stdout token-by-token and relay to ch.
		go func() {
			scanner := bufio.NewScanner(pr)
			scanner.Split(bufio.ScanRunes)
			for scanner.Scan() {
				token := scanner.Text()
				select {
				case ch <- token:
				case <-ctx.Done():
					return
				}
			}
		}()

		cmd.Wait() //nolint:errcheck
		pw.Close()
	}()

	return ch, nil
}

// Close implements Backend (no-op for CLI).
func (b *CLIBackend) Close() {}

// ── NewBestBackend ────────────────────────────────────────────────────────────

// NewBestBackend selects the best available backend from the given provider defs.
// Preference order:
//  1. Ollama (if available and has models)
//  2. First non-ollama, non-shell CLI provider with a non-empty Command field
//
// Returns nil if nothing is available.
func NewBestBackend(defs []picker.ProviderDef) Backend {
	// 1. Prefer Ollama if running.
	if OllamaAvailable() {
		model := BestOllamaModel()
		return NewOllamaBackend(model)
	}

	// 2. First CLI provider that has a command.
	for _, def := range defs {
		if def.ID == "ollama" || def.ID == "shell" {
			continue
		}
		if def.Command == "" {
			continue
		}
		return NewCLIBackend(def.Label, def.Command, def.PipelineArgs)
	}

	return nil
}
