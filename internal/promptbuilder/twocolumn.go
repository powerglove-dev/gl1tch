package promptbuilder

// TwoColumnModel is the new two-column prompt builder TUI.
// Layout:
//   Left  column : Sidebar (saved prompts)
//   Right column : RunnerPanel + SendPanel

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/powerglove-dev/gl1tch/internal/buildershared"
	"github.com/powerglove-dev/gl1tch/internal/busd/topics"
	"github.com/powerglove-dev/gl1tch/internal/executor"
	"github.com/powerglove-dev/gl1tch/internal/panelrender"
	"github.com/powerglove-dev/gl1tch/internal/picker"
	"github.com/powerglove-dev/gl1tch/internal/pipeline"
	"github.com/powerglove-dev/gl1tch/internal/styles"
)

// tcFocus constants for TwoColumnModel outer focus.
const (
	tcFocusSidebar = 0
	tcFocusRunner  = 1
	tcFocusChat    = 2
)

// TwoColumnModel implements tea.Model for the prompt builder TUI.
type TwoColumnModel struct {
	sidebar buildershared.Sidebar
	runner  buildershared.RunnerPanel
	send    buildershared.SendPanel

	focus int

	// Persistence
	promptsDir string
	executorMgr  *executor.Manager

	// Feedback loop
	firstPrompt string
	sentOnce    bool

	// Status
	statusMsg string
	statusErr bool

	width, height int
	pal           styles.ANSIPalette
}

// promptEntry is stored on disk as JSON.
type promptEntry struct {
	Name     string `json:"name"`
	Provider string `json:"provider"`
	Model    string `json:"model"`
	Content  string `json:"content"`
}

// NewTwoColumn creates a new TwoColumnModel.
func NewTwoColumn(promptsDir string, providers []picker.ProviderDef, mgr *executor.Manager) *TwoColumnModel {
	pal := styles.ANSIPalette{
		Accent:  "\x1b[35m",
		Dim:     "\x1b[2m",
		Success: "\x1b[32m",
		Error:   "\x1b[31m",
		Warn:    "\x1b[33m",
		FG:      "\x1b[97m",
		BG:      "\x1b[40m",
		Border:  "\x1b[36m",
		SelBG:   "\x1b[44m",
	}

	m := &TwoColumnModel{
		sidebar:    buildershared.NewSidebar("PROMPTS", nil),
		runner:     buildershared.NewRunnerPanel(),
		send:       buildershared.NewSendPanel(providers),
		promptsDir: promptsDir,
		executorMgr:  mgr,
		pal:        pal,
	}
	m.sidebar = m.sidebar.SetItems(m.loadPromptNames())
	m.sidebar = m.sidebar.SetFocused(true)
	return m
}

// SetPalette updates the color palette.
func (m *TwoColumnModel) SetPalette(pal styles.ANSIPalette) {
	m.pal = pal
}

// Init implements tea.Model.
func (m *TwoColumnModel) Init() tea.Cmd { return nil }

// Update implements tea.Model.
func (m *TwoColumnModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch v := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = v.Width
		m.height = v.Height
		return m, nil

	case buildershared.RunLineMsg, buildershared.RunDoneMsg:
		var cmd tea.Cmd
		m.runner, cmd = m.runner.Update(v)
		return m, cmd

	case tea.KeyMsg:
		return m.handleKey(v)
	}
	return m, nil
}

func (m *TwoColumnModel) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()

	// Global keys.
	switch key {
	case "J":
		// Open jump window as a tmux popup.
		if os.Getenv("TMUX") != "" {
			return m, func() tea.Msg {
				self, _ := os.Executable()
				exec.Command("tmux", "display-popup", "-E", "-w", "80%", "-h", "70%",
					filepath.Clean(self)+" widget jump-window").Run() //nolint:errcheck
				return nil
			}
		}
		return m, nil

	case "ctrl+s":
		if err := m.saveCurrentPrompt(); err != nil {
			m.statusMsg = "save error: " + err.Error()
			m.statusErr = true
		} else {
			m.statusMsg = "saved"
			m.statusErr = false
			m.sidebar = m.sidebar.SetItems(m.loadPromptNames())
		}
		return m, nil

	case "ctrl+r":
		if m.firstPrompt != "" {
			m.runner = m.runner.Clear()
			return m, m.startRun(m.firstPrompt)
		}
		return m, nil

	case "ctrl+c":
		return m, tea.Quit
	}

	switch m.focus {
	case tcFocusSidebar:
		return m.handleSidebarKey(msg)
	case tcFocusRunner:
		return m.handleRunnerKey(msg)
	case tcFocusChat:
		return m.handleChatKey(msg)
	}
	return m, nil
}

func (m *TwoColumnModel) handleSidebarKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()

	var cmd tea.Cmd
	m.sidebar, cmd = m.sidebar.Update(msg)

	if cmd != nil {
		innerMsg := cmd()
		switch v := innerMsg.(type) {
		case buildershared.SidebarSelectMsg:
			m.send = m.send.SetName(v.Name)
			m.sidebar = m.sidebar.SetFocused(false)
			m.send = m.send.Enter()
			m.focus = tcFocusChat
			return m, nil
		case buildershared.SidebarDeleteMsg:
			os.Remove(filepath.Join(m.promptsDir, v.Name+".json")) //nolint:errcheck
			m.sidebar = m.sidebar.SetItems(m.loadPromptNames())
			return m, nil
		}
	}

	if key == "n" {
		m.send = m.send.SetName("new-prompt")
		m.sidebar = m.sidebar.SetFocused(false)
		m.send = m.send.Enter()
		m.focus = tcFocusChat
		return m, nil
	}

	if key == "tab" {
		m.sidebar = m.sidebar.SetFocused(false)
		m.send = m.send.Enter()
		m.focus = tcFocusChat
		return m, nil
	}

	return m, nil
}

func (m *TwoColumnModel) handleRunnerKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()
	var cmd tea.Cmd
	m.runner, cmd = m.runner.Update(msg)

	switch key {
	case "shift+tab", "esc":
		m.runner = m.runner.SetFocused(false)
		m.send = m.send.Enter()
		m.focus = tcFocusChat
	case "tab":
		m.runner = m.runner.SetFocused(false)
		m.sidebar = m.sidebar.SetFocused(true)
		m.focus = tcFocusSidebar
	}
	return m, cmd
}

func (m *TwoColumnModel) handleChatKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	m.send, cmd = m.send.Update(msg)

	if cmd != nil {
		innerMsg := cmd()
		switch v := innerMsg.(type) {
		case buildershared.SendSubmitMsg:
			if v.Message == "" {
				return m, nil
			}
			if !m.sentOnce {
				m.firstPrompt = v.Message
				m.sentOnce = true
			}
			m.runner = m.runner.Clear()
			m.runner = m.runner.SetFocused(true)
			m.focus = tcFocusRunner
			return m, m.startRun(v.Message)
		case buildershared.SendTabOutMsg:
			m.send = m.send.SetFocused(false)
			m.sidebar = m.sidebar.SetFocused(true)
			m.focus = tcFocusSidebar
			return m, nil
		case buildershared.SendShiftTabOutMsg:
			m.send = m.send.SetFocused(false)
			m.runner = m.runner.SetFocused(true)
			m.focus = tcFocusRunner
			return m, nil
		}
	}

	return m, cmd
}

// View implements tea.Model.
func (m *TwoColumnModel) View() string {
	w, h := m.width, m.height
	if w <= 0 {
		w = 120
	}
	if h <= 0 {
		h = 40
	}

	pal := m.pal
	leftW := w / 4
	if leftW < 20 {
		leftW = 20
	}
	rightW := w - leftW
	bodyH := h

	leftLines := m.sidebar.SetFocused(m.focus == tcFocusSidebar).View(leftW, bodyH, pal)
	rightLines := m.buildRight(rightW, bodyH)

	var rows []string
	for i := range bodyH {
		var l, r string
		if i < len(leftLines) {
			l = leftLines[i]
		}
		if i < len(rightLines) {
			r = rightLines[i]
		}
		lv := lipgloss.Width(l)
		if lv < leftW {
			l = l + strings.Repeat(" ", leftW-lv)
		}
		rows = append(rows, l+r)
	}
	base := strings.Join(rows, "\n")

	// Overlay agent picker popup if open.
	if m.send.AgentOpen() {
		overlay := m.send.OverlayView(w, pal)
		return panelrender.OverlayCenter(base, overlay, w, h)
	}
	return base
}

func (m *TwoColumnModel) buildRight(w, h int) []string {
	sendH := 8
	runnerH := h - sendH
	if runnerH < 5 {
		runnerH = 5
	}

	rn := m.runner.SetFocused(m.focus == tcFocusRunner)
	snd := m.send.SetFocused(m.focus == tcFocusChat)

	var rows []string
	rows = append(rows, rn.View(w, runnerH, m.pal)...)
	rows = append(rows, snd.View(w, sendH, m.pal)...)
	return rows
}

// ── Persistence ───────────────────────────────────────────────────────────────

func (m *TwoColumnModel) loadPromptNames() []string {
	entries, err := os.ReadDir(m.promptsDir)
	if err != nil {
		return nil
	}
	var names []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		n := e.Name()
		if strings.HasSuffix(n, ".json") {
			names = append(names, strings.TrimSuffix(n, ".json"))
		}
	}
	return names
}

func (m *TwoColumnModel) saveCurrentPrompt() error {
	name := strings.TrimSpace(m.send.Name())
	if name == "" {
		return fmt.Errorf("prompt name is required")
	}
	if err := os.MkdirAll(m.promptsDir, 0o755); err != nil {
		return err
	}
	p := promptEntry{
		Name:     name,
		Provider: m.send.ProviderID(),
		Model:    m.send.ModelID(),
	}
	data, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(m.promptsDir, name+".json"), data, 0o644)
}

// ── Run logic ─────────────────────────────────────────────────────────────────

func (m *TwoColumnModel) startRun(userMsg string) tea.Cmd {
	if userMsg == "" {
		return nil
	}

	executorID := m.send.ProviderID()
	if executorID == "" {
		executorID = "claude"
	}
	modelID := m.send.ModelID()

	fullPrompt := userMsg

	yamlContent := buildPromptYAML("run-prompt", executorID, modelID, fullPrompt)

	ch := make(chan string, 200)
	ctx, cancel := context.WithCancel(context.Background())

	mgr := m.executorMgr
	providers := picker.BuildProviders()

	go func() {
		defer close(ch)

		if mgr == nil {
			var err error
			mgr, err = buildPromptExecutorManager(providers)
			if err != nil {
				ch <- "error: " + err.Error()
				return
			}
		}

		p, err := pipeline.Load(strings.NewReader(yamlContent))
		if err != nil {
			ch <- "error: " + err.Error()
			return
		}

		pub := &tcLinePublisher{ch: ch}
		_, runErr := pipeline.Run(ctx, p, mgr, "", pipeline.WithEventPublisher(pub))
		if runErr != nil {
			if ctx.Err() != nil {
				ch <- "cancelled"
			} else {
				ch <- "error: " + runErr.Error()
			}
		}
	}()

	m.runner, _ = m.runner.StartRun(ch, cancel)
	return buildershared.WaitForLine(ch)
}

func buildPromptYAML(name, executorID, modelID, prompt string) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("name: %s\nversion: \"1\"\nsteps:\n", name))
	sb.WriteString(fmt.Sprintf("  - id: run\n    executor: %s\n", executorID))
	if modelID != "" {
		sb.WriteString(fmt.Sprintf("    model: %s\n", modelID))
	}
	if prompt != "" {
		sb.WriteString("    prompt: |\n")
		for _, line := range strings.Split(prompt, "\n") {
			sb.WriteString("      " + line + "\n")
		}
	}
	return sb.String()
}

func buildPromptExecutorManager(providers []picker.ProviderDef) (*executor.Manager, error) {
	mgr := executor.NewManager()
	for _, prov := range providers {
		if prov.SidecarPath != "" {
			continue
		}
		binary := prov.Command
		if binary == "" {
			binary = prov.ID
		}
		_ = mgr.Register(executor.NewCliAdapter(prov.ID, prov.Label+" CLI adapter", binary, prov.PipelineArgs...))
	}
	configDir := picker.GlitchConfigDir()
	if configDir != "" {
		_ = mgr.LoadWrappersFromDir(filepath.Join(configDir, "wrappers"))
	}
	return mgr, nil
}

// tcLinePublisher implements pipeline.EventPublisher for the TwoColumnModel.
type tcLinePublisher struct {
	ch chan<- string
}

func (p *tcLinePublisher) Publish(_ context.Context, topic string, payload []byte) error {
	switch topic {
	case topics.StepDone, topics.StepFailed:
		var evt struct {
			Output string `json:"output"`
			StepID string `json:"step_id"`
		}
		if err := json.Unmarshal(payload, &evt); err == nil {
			if topic == topics.StepFailed {
				p.ch <- fmt.Sprintf("[fail] %s", evt.StepID)
			}
			if evt.Output != "" {
				for _, line := range strings.Split(evt.Output, "\n") {
					line = strings.TrimRight(line, "\r")
					if line != "" {
						p.ch <- line
					}
				}
			}
		}
	case topics.StepStarted:
		var evt struct {
			StepID string `json:"step_id"`
		}
		if err := json.Unmarshal(payload, &evt); err == nil && evt.StepID != "" {
			p.ch <- fmt.Sprintf("[running via %s…]", evt.StepID)
		}
	case topics.RunCompleted:
		p.ch <- "[done]"
	case topics.RunFailed:
		var evt struct {
			Error string `json:"error"`
		}
		if err := json.Unmarshal(payload, &evt); err == nil && evt.Error != "" {
			p.ch <- "[fail] " + evt.Error
		} else {
			p.ch <- "[fail] run failed"
		}
	}
	return nil
}
