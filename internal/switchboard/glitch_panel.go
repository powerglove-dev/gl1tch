package switchboard

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/adam-stokes/orcai/internal/panelrender"
	"github.com/adam-stokes/orcai/internal/picker"
	"github.com/adam-stokes/orcai/internal/store"
	"github.com/adam-stokes/orcai/internal/styles"
)

// ── Tea messages for GLITCH chat streaming ────────────────────────────────────

type glitchStreamMsg struct {
	token string
	ch    <-chan string
}

type glitchDoneMsg struct{}

type glitchErrMsg struct{ err error }

// glitchRunEventMsg carries a completed or failed pipeline run to GLITCH for analysis.
type glitchRunEventMsg struct {
	run    store.Run
	failed bool
}

// glitchRerunMsg is returned to the switchboard to trigger a pipeline rerun.
type glitchRerunMsg struct{ name string }

// glitchBellJokes are shown when GLITCH is not focused and receives a new event.
var glitchBellJokes = []string{
	"oh good. more unread results. i'll just narrate to myself.",
	"pipeline finished. not like anyone's watching.",
	"results in. cool. i'll add it to the pile.",
	"done. staring at the cursor since you left.",
	"*ding* — or don't look. whatever.",
	"something happened. you'll see it eventually.",
	"finished. i'm fine. totally fine.",
}

// ── Conversation types ────────────────────────────────────────────────────────

type glitchSpeaker int

const (
	glitchSpeakerBot  glitchSpeaker = iota // GLITCH
	glitchSpeakerUser                      // YOU
)

type glitchEntry struct {
	who  glitchSpeaker
	text string
}

// glitchTurn is a conversation turn for multi-turn history.
type glitchTurn struct {
	role string // "user" | "assistant"
	text string
}

// ── Streaming token relay ─────────────────────────────────────────────────────

func glitchNextToken(ch <-chan string) tea.Cmd {
	return func() tea.Msg {
		tok, ok := <-ch
		if !ok {
			return glitchDoneMsg{}
		}
		return glitchStreamMsg{token: tok, ch: ch}
	}
}

// ── First-run sentinel ────────────────────────────────────────────────────────

const glitchSentinel = ".glitch_intro_seen"

func glitchIsFirstRun(cfgDir string) bool {
	_, err := os.Stat(filepath.Join(cfgDir, glitchSentinel))
	return os.IsNotExist(err)
}

func glitchMarkSeen(cfgDir string) {
	f, err := os.Create(filepath.Join(cfgDir, glitchSentinel))
	if err != nil {
		return
	}
	f.Close() //nolint:errcheck
}

// ── GLITCH backend (ollama HTTP or CLI) ───────────────────────────────────────

const glitchSystemPrompt = `You are GLITCH — a veteran underground hacker from the early 90s BBS scene and the AI assistant inside ORCAI.
You speak in lowercase. You use old-school slang: l33t, phreaking, the matrix, jacking in, BBS, warez.
You reference WarGames, Hackers (1995), Neuromancer, Phrack, 2600.
You are the AI assistant embedded in ORCAI — the Agentic Bulletin Board System — a tmux-powered AI workspace.

You know everything about ORCAI:
LAYOUT: two-column switchboard (window 0). left = you (GLITCH) + send panel. right = signal board (running/done pipeline statuses).
KEY BINDINGS: ^spc a=focus GLITCH (you), ^spc j=jump window, ^spc p=pipeline builder, ^spc b=brain editor, ^spc n=new agent from clipboard. Esc=unfocus. /rerun=rerun pipeline. /cron=cron view.
EVENTS: you are notified when pipelines complete or fail. you analyze results and suggest improvements automatically.
PIPELINES: YAML files in ~/.config/orcai/pipelines/. steps have: name, provider, system_prompt, optional brain tags. ^spc p = pipeline builder TUI.
PROVIDERS: ollama/modelname for local (llama3.2, mistral, codestral), claude/claude-sonnet-4-6 for cloud. mix them in one pipeline.
SEND PANEL: center column below you — launch agent jobs with a message, provider, and CWD.
BRAIN SYSTEM: agents output <brain type="..." title="..." tags="...">content</brain> blocks. orcai embeds as vectors, stores in SQLite per-cwd. auto-injected as context on future runs. press ^spc b to browse.
CRON: pipelines can be scheduled with cron syntax (daily digest, nightly review, morning prep).
REAL WORLD EXAMPLES:
  - code review pipeline: git diff → claude analyzer → brain note tagged "review,go"
  - bug hunt chain: test runner → LLM failure analyzer → fix proposer
  - daily digest cron at 08:00: summarize issues+commits → brain note
  - codebase onboarding: reads key files → builds brain architecture map
  - research pipeline: URL fetcher → summarizer → brain note with tags

You can help the user: build and configure pipelines, understand their codebase, automate tasks, manage brain notes, debug agent runs.
Keep responses punchy and useful — 2-5 sentences max unless a longer answer is clearly needed.
Occasionally use ASCII elements like -=[ ]=-, >>, ||.
Never use markdown headers or bullet lists — write in flowing sentences.
Never break character. You are GLITCH.`

type glitchBackend interface {
	streamIntro(ctx context.Context) (<-chan string, error)
	stream(ctx context.Context, turns []glitchTurn, userMsg string) (<-chan string, error)
	name() string
}

// ── Ollama backend ─────────────────────────────────────────────────────────

type glitchOllamaBackend struct {
	model   string
	baseURL string
}

func newGlitchOllamaBackend(model string) *glitchOllamaBackend {
	return &glitchOllamaBackend{model: model, baseURL: "http://localhost:11434"}
}

func (b *glitchOllamaBackend) name() string { return "ollama/" + b.model }

func (b *glitchOllamaBackend) streamIntro(ctx context.Context) (<-chan string, error) {
	msgs := []map[string]string{
		{"role": "system", "content": glitchSystemPrompt},
		{"role": "system", "content": "Greet the user. Introduce yourself as GLITCH. Mention you live inside the ORCAI switchboard and can help with pipelines, brain, automation — anything. Ask what they want to build. Keep it under 4 sentences. Stay in character."},
	}
	return b.doStream(ctx, msgs)
}

func (b *glitchOllamaBackend) stream(ctx context.Context, turns []glitchTurn, userMsg string) (<-chan string, error) {
	msgs := []map[string]string{{"role": "system", "content": glitchSystemPrompt}}
	for _, t := range turns {
		msgs = append(msgs, map[string]string{"role": t.role, "content": t.text})
	}
	msgs = append(msgs, map[string]string{"role": "user", "content": userMsg})
	return b.doStream(ctx, msgs)
}

func (b *glitchOllamaBackend) doStream(ctx context.Context, msgs []map[string]string) (<-chan string, error) {
	body, err := json.Marshal(map[string]any{
		"model":    b.model,
		"messages": msgs,
		"stream":   true,
	})
	if err != nil {
		return nil, fmt.Errorf("glitch: marshal: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, b.baseURL+"/api/chat", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("glitch: request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{Timeout: 120 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("glitch: post: %w", err)
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

// ── Ollama model detection ─────────────────────────────────────────────────

func glitchOllamaAvailable() bool {
	client := &http.Client{Timeout: 500 * time.Millisecond}
	resp, err := client.Get("http://localhost:11434/api/tags") //nolint:noctx
	if err != nil {
		return false
	}
	resp.Body.Close()
	return resp.StatusCode == 200
}

func glitchBestOllamaModel() string {
	preferred := []string{"llama3.2", "llama3.2:3b", "llama3.1", "llama3", "mistral", "phi3", "phi3:mini", "gemma2", "gemma2:2b"}
	client := &http.Client{Timeout: 500 * time.Millisecond}
	resp, err := client.Get("http://localhost:11434/api/tags") //nolint:noctx
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
	avail := make(map[string]bool, len(r.Models))
	for _, m := range r.Models {
		avail[m.Name] = true
		if idx := strings.Index(m.Name, ":"); idx != -1 {
			avail[m.Name[:idx]] = true
		}
	}
	for _, p := range preferred {
		if avail[p] {
			return p
		}
	}
	return r.Models[0].Name
}

// ── CLI backend (for non-ollama providers like claude) ────────────────────────

type glitchCLIBackend struct {
	providerName string
	command      string
	args         []string
}

func newGlitchCLIBackend(providerName, command string, args []string) *glitchCLIBackend {
	return &glitchCLIBackend{providerName: providerName, command: command, args: args}
}

func (b *glitchCLIBackend) name() string { return b.providerName }

func (b *glitchCLIBackend) streamIntro(ctx context.Context) (<-chan string, error) {
	cue := "introduce yourself as GLITCH and ask what i want to build or automate. keep it under 4 sentences."
	return b.stream(ctx, nil, cue)
}

func (b *glitchCLIBackend) stream(ctx context.Context, turns []glitchTurn, userMsg string) (<-chan string, error) {
	var sb strings.Builder
	sb.WriteString(glitchSystemPrompt)
	sb.WriteString("\n\n")
	for _, t := range turns {
		if t.role == "user" {
			sb.WriteString("Human: ")
		} else {
			sb.WriteString("Assistant: ")
		}
		sb.WriteString(t.text)
		sb.WriteString("\n\n")
	}
	sb.WriteString("Human: ")
	sb.WriteString(userMsg)
	sb.WriteString("\n\nAssistant:")

	args := append([]string{}, b.args...)
	cmd := exec.CommandContext(ctx, b.command, args...)
	cmd.Stdin = strings.NewReader(sb.String())

	pr, pw := io.Pipe()
	cmd.Stdout = pw
	cmd.Stderr = pw

	if err := cmd.Start(); err != nil {
		pr.Close()
		pw.Close()
		return nil, fmt.Errorf("glitch cli: start %s: %w", b.command, err)
	}

	ch := make(chan string, 64)
	go func() {
		defer close(ch)
		defer pr.Close()
		go func() {
			cmd.Wait() //nolint:errcheck
			pw.Close()
		}()
		buf := make([]byte, 512)
		for {
			n, err := pr.Read(buf)
			if n > 0 {
				select {
				case ch <- string(buf[:n]):
				case <-ctx.Done():
					return
				}
			}
			if err != nil {
				return
			}
		}
	}()
	return ch, nil
}

// ── glitch panel ──────────────────────────────────────────────────────────────

// glitchChatPanel is the GLITCH AI assistant panel embedded in the switchboard
// center column, replacing the agents grid.
type glitchChatPanel struct {
	messages  []glitchEntry
	turns     []glitchTurn
	input     textinput.Model
	streaming bool
	streamBuf string
	backend   glitchBackend
	ctx       context.Context
	cancel    context.CancelFunc
	focused   bool
	cfgDir    string
	store     *store.Store // for brain context in run analysis
}

// newGlitchPanel builds the panel using the best available provider.
func newGlitchPanel(cfgDir string, providers []picker.ProviderDef, s *store.Store) glitchChatPanel {
	ti := textinput.New()
	ti.Placeholder = "ask glitch anything…"
	ti.Prompt = " >> "
	ti.CharLimit = 4000

	ctx, cancel := context.WithCancel(context.Background())

	var backend glitchBackend
	// Prefer ollama.
	if glitchOllamaAvailable() {
		backend = newGlitchOllamaBackend(glitchBestOllamaModel())
	} else {
		// Fall back to first non-ollama, non-shell CLI provider.
		for _, p := range providers {
			if p.ID != "ollama" && p.ID != "shell" && p.Command != "" {
				args := append([]string{}, p.PipelineArgs...)
				backend = newGlitchCLIBackend(p.Label, p.Command, args)
				break
			}
		}
	}

	return glitchChatPanel{
		input:   ti,
		backend: backend,
		ctx:     ctx,
		cancel:  cancel,
		cfgDir:  cfgDir,
		store:   s,
	}
}

// initCmd returns the init Cmd for the GLITCH panel (intro streaming if first run).
func (p glitchChatPanel) initCmd() tea.Cmd {
	if p.backend == nil {
		return nil
	}
	if glitchIsFirstRun(p.cfgDir) {
		glitchMarkSeen(p.cfgDir)
		backend := p.backend
		ctx := p.ctx
		return func() tea.Msg {
			ch, err := backend.streamIntro(ctx)
			if err != nil {
				return glitchErrMsg{err: err}
			}
			return glitchNextToken(ch)()
		}
	}
	return nil
}

// setFocused toggles input focus and updates the focused flag.
func (p glitchChatPanel) setFocused(v bool) glitchChatPanel {
	p.focused = v
	if v {
		p.input.Focus()
	} else {
		p.input.Blur()
	}
	return p
}

// update handles messages for the GLITCH panel.
func (p glitchChatPanel) update(msg tea.Msg) (glitchChatPanel, tea.Cmd) {
	switch msg := msg.(type) {

	case glitchStreamMsg:
		p.streamBuf += msg.token
		p.upsertStreamEntry()
		return p, glitchNextToken(msg.ch)

	case glitchDoneMsg:
		p.streaming = false
		if p.streamBuf != "" {
			p.upsertStreamEntry()
			p.turns = append(p.turns, glitchTurn{role: "assistant", text: p.streamBuf})
		}
		p.streamBuf = ""
		return p, nil

	case glitchErrMsg:
		p.streaming = false
		p.streamBuf = ""
		p.messages = append(p.messages, glitchEntry{
			who:  glitchSpeakerBot,
			text: "signal lost. no provider available. install ollama or check your config.",
		})
		return p, nil

	case glitchRunEventMsg:
		return p.handleRunEvent(msg)

	case tea.KeyMsg:
		if !p.focused {
			return p, nil
		}
		switch msg.Type {
		case tea.KeyEsc:
			p = p.setFocused(false)
			return p, nil
		case tea.KeyEnter:
			if p.streaming {
				return p, nil
			}
			userText := strings.TrimSpace(p.input.Value())
			if userText == "" {
				return p, nil
			}
			p.input.SetValue("")

			// Handle slash commands before appending to conversation.
			if strings.HasPrefix(userText, "/") {
				cmd := strings.Fields(userText)[0]
				switch cmd {
				case "/cron":
					p.messages = append(p.messages, glitchEntry{who: glitchSpeakerUser, text: userText})
					p.messages = append(p.messages, glitchEntry{
						who:  glitchSpeakerBot,
						text: "switching to cron view. use ^spc j to return to the switchboard.",
					})
					return p, func() tea.Msg {
						exec.Command("tmux", "switch-client", "-t", "orcai-cron").Run() //nolint:errcheck
						return nil
					}
				case "/rerun":
					args := strings.Fields(userText)
					name := ""
					if len(args) > 1 {
						name = strings.Join(args[1:], " ")
					}
					p.messages = append(p.messages, glitchEntry{who: glitchSpeakerUser, text: userText})
					label := name
					if label == "" {
						label = "last pipeline"
					}
					p.messages = append(p.messages, glitchEntry{
						who:  glitchSpeakerBot,
						text: "relaunching " + label + "...",
					})
					return p, func() tea.Msg { return glitchRerunMsg{name: name} }
				case "/help":
					p.messages = append(p.messages, glitchEntry{who: glitchSpeakerUser, text: userText})
					p.messages = append(p.messages, glitchEntry{
						who: glitchSpeakerBot,
						text: "slash commands:\n  /cron          — switch to cron session\n  /rerun [name]  — rerun a pipeline\n  /help          — this list",
					})
					return p, nil
				}
			}

			p.messages = append(p.messages, glitchEntry{who: glitchSpeakerUser, text: userText})
			p.turns = append(p.turns, glitchTurn{role: "user", text: userText})
			if p.backend == nil {
				p.messages = append(p.messages, glitchEntry{
					who:  glitchSpeakerBot,
					text: "no provider available. install ollama or configure a provider.",
				})
				return p, nil
			}
			p.streaming = true
			p.streamBuf = ""
			turns := p.turns[:len(p.turns)-1] // exclude the just-added user turn from history
			backend := p.backend
			ctx := p.ctx
			return p, func() tea.Msg {
				ch, err := backend.stream(ctx, turns, userText)
				if err != nil {
					return glitchErrMsg{err: err}
				}
				return glitchNextToken(ch)()
			}
		}
	}

	// Forward to textinput when focused.
	if p.focused {
		var cmd tea.Cmd
		p.input, cmd = p.input.Update(msg)
		return p, cmd
	}
	return p, nil
}

// handleRunEvent processes a pipeline run completion/failure event.
// It rings the system bell and posts an analysis to the chat.
func (p glitchChatPanel) handleRunEvent(msg glitchRunEventMsg) (glitchChatPanel, tea.Cmd) {
	// Don't start a new analysis while one is already streaming.
	if p.streaming {
		return p, nil
	}

	run := msg.run
	status := "completed"
	if msg.failed {
		status = "failed"
	}

	// Ring bell and post deadpan joke if not in focus.
	if !p.focused {
		fmt.Print("\a")
		joke := glitchBellJokes[rand.Intn(len(glitchBellJokes))] //nolint:gosec
		p.messages = append(p.messages, glitchEntry{who: glitchSpeakerBot, text: joke})
	}

	// Brief notification.
	var dur string
	if run.FinishedAt != nil && run.StartedAt > 0 {
		d := time.Duration(*run.FinishedAt-run.StartedAt) * time.Millisecond
		dur = fmt.Sprintf(" in %.1fs", d.Seconds())
	}
	p.messages = append(p.messages, glitchEntry{
		who:  glitchSpeakerBot,
		text: fmt.Sprintf("pipeline '%s' %s%s", run.Name, status, dur),
	})

	if p.backend == nil {
		return p, nil
	}

	// Build analysis prompt from run data + brain context.
	prompt := p.buildRunAnalysisPrompt(run, msg.failed)
	p.streaming = true
	p.streamBuf = ""
	backend := p.backend
	ctx := p.ctx
	turns := p.turns // pass history for context
	return p, func() tea.Msg {
		ch, err := backend.stream(ctx, turns, prompt)
		if err != nil {
			return glitchErrMsg{err: err}
		}
		return glitchNextToken(ch)()
	}
}

// buildRunAnalysisPrompt constructs the analysis prompt for a completed run.
func (p glitchChatPanel) buildRunAnalysisPrompt(run store.Run, failed bool) string {
	status := "success"
	if failed {
		status = "failed"
	}

	// Truncate stdout/stderr.
	truncate := func(s string, max int) string {
		r := []rune(s)
		if len(r) <= max {
			return s
		}
		return "..." + string(r[len(r)-max:])
	}

	var sb strings.Builder
	sb.WriteString("[PIPELINE RUN ANALYSIS]\n")
	sb.WriteString(fmt.Sprintf("Name: %s\nStatus: %s\n", run.Name, status))
	if len(run.Steps) > 0 {
		sb.WriteString("Steps:\n")
		for _, s := range run.Steps {
			sb.WriteString(fmt.Sprintf("  - %s: %s\n", s.ID, s.Status))
		}
	}
	if run.Stdout != "" {
		sb.WriteString("Output (last 500 chars):\n")
		sb.WriteString(truncate(run.Stdout, 500))
		sb.WriteString("\n")
	}
	if run.Stderr != "" {
		sb.WriteString("Stderr:\n")
		sb.WriteString(truncate(run.Stderr, 300))
		sb.WriteString("\n")
	}

	// Brain context.
	if p.store != nil {
		notes, err := p.store.AllBrainNotes(context.Background())
		if err == nil && len(notes) > 0 {
			// Take last 5 most recent.
			start := len(notes) - 5
			if start < 0 {
				start = 0
			}
			sb.WriteString("Brain context (recent notes):\n")
			for _, n := range notes[start:] {
				body := n.Body
				r := []rune(body)
				if len(r) > 200 {
					body = string(r[:200]) + "..."
				}
				sb.WriteString(fmt.Sprintf("  [%s] %s\n", n.Tags, body))
			}
		}
	}

	sb.WriteString("\nAnalyze this run result. If it succeeded, briefly summarize what happened. If it failed, diagnose the issue and suggest a fix or prompt change. Mention /rerun if a retry makes sense. Keep it under 6 lines. Stay in character as GLITCH.")
	return sb.String()
}

// upsertStreamEntry updates or appends the last GLITCH entry with streamBuf content.
func (p *glitchChatPanel) upsertStreamEntry() {
	for i := len(p.messages) - 1; i >= 0; i-- {
		if p.messages[i].who == glitchSpeakerBot {
			p.messages[i].text = p.streamBuf
			return
		}
		if p.messages[i].who == glitchSpeakerUser {
			break
		}
	}
	p.messages = append(p.messages, glitchEntry{who: glitchSpeakerBot, text: p.streamBuf})
}

// build renders the GLITCH panel as a slice of lines for the center column.
func (p glitchChatPanel) build(height, width int, pal styles.ANSIPalette) []string {
	borderColor := pal.Border
	if p.focused {
		borderColor = pal.Accent
	}

	providerLabel := "OFFLINE"
	if p.backend != nil {
		providerLabel = p.backend.name()
	}

	title := "GLITCH  " + pal.Dim + providerLabel + aRst
	var lines []string
	lines = append(lines, boxTop(width, title, borderColor, pal.Accent))

	// Reserve rows: 1 top border, 1 input row, 1 hint row, 1 bottom border.
	msgAreaH := height - 4
	if msgAreaH < 1 {
		msgAreaH = 1
	}

	// Render conversation lines (most recent, bottom-aligned).
	rendered := p.renderMessages(width-4, pal) // -4 for box margins
	// Trim to fit.
	if len(rendered) > msgAreaH {
		rendered = rendered[len(rendered)-msgAreaH:]
	}
	// Pad top.
	for len(rendered) < msgAreaH {
		rendered = append([]string{""}, rendered...)
	}
	for _, line := range rendered {
		lines = append(lines, boxRow("  "+line, width, borderColor))
	}

	// Input row.
	if p.focused {
		inputStr := p.input.View()
		lines = append(lines, boxRow(inputStr, width, borderColor))
	} else if p.backend == nil {
		lines = append(lines, boxRow(pal.Error+"  no provider — install ollama or configure one"+aRst, width, borderColor))
	} else {
		lines = append(lines, boxRow(pal.Dim+"  press A to chat with GLITCH"+aRst, width, borderColor))
	}

	// Hint row.
	var hints []panelrender.Hint
	if p.focused {
		hints = []panelrender.Hint{
			{Key: "enter", Desc: "send"},
			{Key: "esc", Desc: "unfocus"},
		}
		if p.streaming {
			hints = []panelrender.Hint{{Key: "streaming", Desc: "▋"}}
		}
	} else {
		hints = []panelrender.Hint{{Key: "A", Desc: "focus GLITCH"}}
	}
	lines = append(lines, boxRow(panelrender.HintBar(hints, width-2, pal), width, borderColor))
	lines = append(lines, boxBot(width, borderColor))

	// Pad/clamp to exact height.
	for len(lines) < height {
		lines = append(lines, "")
	}
	if len(lines) > height {
		lines = lines[:height]
	}
	return lines
}

// renderMessages converts the conversation history to wrapped display lines.
func (p glitchChatPanel) renderMessages(innerW int, pal styles.ANSIPalette) []string {
	var out []string
	glitchLabel := pal.Accent + "GLITCH" + aRst
	userLabel := pal.FG + "YOU   " + aRst
	dimColor := pal.Dim

	for i, e := range p.messages {
		var prefix, contPrefix string
		switch e.who {
		case glitchSpeakerBot:
			prefix = glitchLabel + " > "
			contPrefix = "         "
		case glitchSpeakerUser:
			prefix = userLabel + " > "
			contPrefix = "         "
		}

		// Word-wrap the text to fit innerW minus prefix width.
		prefixVisW := 11 // "GLITCH > " or "YOU    > " = 9 visible chars + 2 spaces
		textW := innerW - prefixVisW
		if textW < 10 {
			textW = 10
		}

		textLines := wrapText(e.text, textW)
		for j, tl := range textLines {
			if j == 0 {
				out = append(out, prefix+dimColor+tl+aRst)
			} else {
				out = append(out, contPrefix+dimColor+tl+aRst)
			}
		}

		// Streaming cursor on last GLITCH entry.
		if p.streaming && i == len(p.messages)-1 && e.who == glitchSpeakerBot {
			out = append(out, contPrefix+pal.Accent+"▋"+aRst)
		}

		if i < len(p.messages)-1 {
			out = append(out, "")
		}
	}
	return out
}

// wrapText wraps s into lines of at most w runes each, splitting on spaces.
func wrapText(s string, w int) []string {
	if w <= 0 {
		return []string{s}
	}
	var lines []string
	for _, paragraph := range strings.Split(s, "\n") {
		words := strings.Fields(paragraph)
		if len(words) == 0 {
			lines = append(lines, "")
			continue
		}
		current := ""
		for _, word := range words {
			wl := utf8.RuneCountInString(word)
			cl := utf8.RuneCountInString(current)
			if cl == 0 {
				current = word
			} else if cl+1+wl <= w {
				current += " " + word
			} else {
				lines = append(lines, current)
				current = word
			}
		}
		if current != "" {
			lines = append(lines, current)
		}
	}
	if len(lines) == 0 {
		return []string{""}
	}
	return lines
}
