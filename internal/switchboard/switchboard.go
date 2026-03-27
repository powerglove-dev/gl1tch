// Package switchboard implements the ORCAI Switchboard — a full-screen BubbleTea
// TUI that merges the sysop panel and welcome dashboard into a single control
// surface with a Pipeline Launcher, Agent Runner, and Activity Feed.
package switchboard

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"
	"github.com/muesli/ansi"
	"github.com/muesli/reflow/truncate"

	"github.com/adam-stokes/orcai/internal/picker"
	"github.com/adam-stokes/orcai/internal/pipeline"
	"github.com/adam-stokes/orcai/internal/plugin"
	"github.com/adam-stokes/orcai/internal/styles"
	"github.com/adam-stokes/orcai/internal/themes"
)

// ── ANSI palette — Dracula BBS aesthetic ─────────────────────────────────────

const (
	aBlu   = "\x1b[34m"  // blue
	aPur   = "\x1b[35m"  // purple
	aCyn   = "\x1b[36m"  // cyan (unused but defined per spec)
	aBrC   = "\x1b[96m"  // bright cyan
	aPnk   = "\x1b[95m"  // pink
	aGrn   = "\x1b[32m"  // green
	aYlw   = "\x1b[33m"  // yellow
	aRed   = "\x1b[31m"  // red
	aDim   = "\x1b[2m"   // dim
	aBld   = "\x1b[1m"   // bold
	aWht   = "\x1b[97m"  // white
aRst   = "\x1b[0m"   // reset
	aBC    = "\x1b[36m"  // cyan borders (alias)
	aBlu2  = "\x1b[34m"  // blue alias (unused var prevention)
)

// suppress unused-const warnings at compile time
var _ = aBlu
var _ = aCyn
var _ = aBlu2

// ── Feed types ────────────────────────────────────────────────────────────────

// FeedStatus is the lifecycle state of an activity feed entry.
type FeedStatus int

const (
	FeedRunning FeedStatus = iota
	FeedDone
	FeedFailed
)

// agentInnerHeight is the fixed number of body rows inside the AGENT RUNNER box.
const agentInnerHeight = 8

// maxParallelJobs is the maximum number of jobs that can run concurrently.
const maxParallelJobs = 8

// StepInfo tracks the status of a single pipeline step within a feed entry.
type StepInfo struct {
	id     string
	status string // "pending", "running", "done", "failed"
}

type feedEntry struct {
	id         string
	title      string
	status     FeedStatus
	ts         time.Time
	lines      []string
	steps      []StepInfo
	tmuxWindow string // fully-qualified target "session:orcai-<feedID>", empty if no window
	logFile    string // /tmp/orcai-<feedID>.log
	doneFile   string // non-empty for window-mode jobs; written by the shell when the command exits
}

// ── Section types ─────────────────────────────────────────────────────────────

type launcherSection struct {
	pipelines []string
	selected  int
	focused   bool
}

type agentSection struct {
	providers              []picker.ProviderDef
	selectedProvider       int
	selectedModel          int
	prompt                 textarea.Model
	focused                bool
	agentScrollOffset      int
	agentModelScrollOffset int
}

type jobHandle struct {
	id         string
	cancel     context.CancelFunc
	ch         chan tea.Msg
	tmuxWindow string
	logFile    string // /tmp/orcai-<feedID>.log — tailed in the tmux window
}

// ── Tea messages ──────────────────────────────────────────────────────────────

// FeedLineMsg is a tea.Msg carrying one line of output from a running job.
// Exported so test packages can assert on it.
type FeedLineMsg struct {
	ID   string
	Line string
}

// StepStatusMsg carries a step lifecycle update from the log-watcher to the model.
type StepStatusMsg struct {
	FeedID string // feed entry ID
	StepID string
	Status string
}

// parseStepStatus parses a "[step:<id>] status:<state>" log line.
// Returns ok=false for any non-matching or malformed line.
func parseStepStatus(line string) (stepID, status string, ok bool) {
	const prefix = "[step:"
	const sep = "] status:"
	if !strings.HasPrefix(line, prefix) {
		return "", "", false
	}
	rest := line[len(prefix):]
	idx := strings.Index(rest, sep)
	if idx < 0 {
		return "", "", false
	}
	id := rest[:idx]
	state := rest[idx+len(sep):]
	if id == "" || state == "" {
		return "", "", false
	}
	return id, state, true
}

type jobDoneMsg struct {
	id string
}

type jobFailedMsg struct {
	id  string
	err error
}

type tickMsg time.Time

// themeChangedMsg is sent when the active theme changes.
type themeChangedMsg struct{}

// ── Window / telemetry types (preserved from sidebar for backwards compat) ────

// Window represents a tmux window (excluding window 0).
type Window struct {
	Index  int
	Name   string
	Active bool
}

// ParseWindows parses output of:
//
//	tmux list-windows -t orcai -F "#{window_index} #{window_name} #{window_active}"
//
// Skips window 0 (the ORCAI home window).
func ParseWindows(output string) []Window {
	var windows []Window
	for _, line := range strings.Split(strings.TrimSpace(output), "\n") {
		if line == "" {
			continue
		}
		parts := strings.Fields(line)
		if len(parts) < 3 {
			continue
		}
		idx, err := strconv.Atoi(parts[0])
		if err != nil || idx == 0 {
			continue
		}
		windows = append(windows, Window{
			Index:  idx,
			Name:   parts[1],
			Active: parts[2] == "1",
		})
	}
	return windows
}

// TelemetryMsg carries a parsed telemetry event from the bus.
type TelemetryMsg struct {
	SessionID    string
	WindowName   string
	Provider     string
	Status       string
	InputTokens  int
	OutputTokens int
	CostUSD      float64
}

// ── Model ─────────────────────────────────────────────────────────────────────

// Model is the BubbleTea model for the Switchboard.
type Model struct {
	width                 int
	height                int
	feed                  []feedEntry // ring buffer, cap 200
	launcher              launcherSection
	agent                 agentSection
	activeJobs            map[string]*jobHandle
	feedSelected          int // index into feed for expanded view
	confirmQuit           bool
	feedScrollOffset      int
	feedCursor            int // absolute line index within all visible feed content
	feedFocused           bool
	signalBoard           SignalBoard
	signalBoardFocused    bool
	confirmDelete         bool
	pendingDeletePipeline string
	agentModalOpen        bool
	agentModalFocus       int // 0=provider, 1=model, 2=prompt, 3=cwd (within modal)
	helpOpen              bool
	helpScrollOffset      int
	registry              *themes.Registry
	themePickerOpen       bool
	themePickerCursor     int
	// CWD / dir picker
	launchCWD           string         // CWD at orcai startup (immutable after New())
	agentCWD            string         // current agent session CWD (user-editable)
	dirPicker           DirPickerModel // reusable dir picker overlay
	dirPickerOpen       bool           // whether the dir picker overlay is visible
	dirPickerCtx        string         // "agent" or "pipeline"
	pendingPipelineName string         // pipeline waiting for CWD selection
	pendingPipelineYAML string         // YAML path for pendingPipelineName
}

// New creates a new Switchboard Model, discovering pipelines and providers.
func New() Model {
	ta := textarea.New()
	ta.Placeholder = "Enter prompt… (ctrl+s to submit)"
	ta.CharLimit = 4096
	ta.ShowLineNumbers = false
	ta.SetWidth(80)
	ta.SetHeight(4)

	cwd, _ := os.Getwd()

	m := Model{
		launcher: launcherSection{
			pipelines: ScanPipelines(pipelinesDir()),
			focused:   true,
		},
		agent: agentSection{
			providers: picker.BuildProviders(),
			prompt:    ta,
		},
		signalBoard: SignalBoard{activeFilter: "all"},
		activeJobs:  make(map[string]*jobHandle),
		launchCWD:   cwd,
		agentCWD:    cwd,
	}

	// Initialize theme registry from user themes dir.
	userThemesDir := filepath.Join(os.Getenv("HOME"), ".config", "orcai", "themes")
	if reg, err := themes.NewRegistry(userThemesDir); err == nil {
		m.registry = reg
	}

	return m
}

// NewWithWindows is kept for backward-compat with sidebar-based callers.
// It ignores the window list and calls New().
func NewWithWindows(_ []Window) Model { return New() }

// NewWithPipelines creates a Model with a fixed pipeline list — used in tests.
func NewWithPipelines(pipelines []string) Model {
	m := New()
	m.launcher.pipelines = pipelines
	m.launcher.selected = 0
	m.launcher.focused = true
	return m
}

// NewWithTestProviders creates a Model with synthetic providers for testing.
func NewWithTestProviders() Model {
	m := New()
	m.agent.providers = []picker.ProviderDef{
		{
			ID:    "test-provider",
			Label: "Test Provider",
			Models: []picker.ModelOption{
				{ID: "model-a", Label: "Model A"},
				{ID: "model-b", Label: "Model B"},
			},
		},
	}
	return m
}

// Cursor returns the launcher cursor position — used in tests.
func (m Model) Cursor() int { return m.launcher.selected }

// AgentFormStep is kept for backward compatibility — always returns 0.
// The inline multi-step wizard has been replaced by the agent modal overlay.
func (m Model) AgentFormStep() int { return 0 }

// AgentModalOpen returns whether the agent modal overlay is open — used in tests.
func (m Model) AgentModalOpen() bool { return m.agentModalOpen }

// ThemePickerOpen returns whether the theme picker overlay is open — used in tests.
func (m Model) ThemePickerOpen() bool { return m.themePickerOpen }

// FeedScrollOffset returns the current feed scroll offset — used in tests.
func (m Model) FeedScrollOffset() int { return m.feedScrollOffset }

// FeedCursor returns the current feed cursor position — used in tests.
func (m Model) FeedCursor() int { return m.feedCursor }

// FeedFocused returns the feed focus state — used in tests.
func (m Model) FeedFocused() bool { return m.feedFocused }

// BuildAgentSection is an exported wrapper for tests.
func (m Model) BuildAgentSection(w int) []string { return m.buildAgentSection(w) }

// BuildSignalBoard is an exported wrapper for tests.
func (m Model) BuildSignalBoard(height, width int) []string { return m.buildSignalBoard(height, width) }

// SignalBoardBlinkOn returns the current blink state — used in tests.
func (m Model) SignalBoardBlinkOn() bool { return m.signalBoard.blinkOn }

// ActiveJobsCount returns the number of currently active jobs — used in tests.
func (m Model) ActiveJobsCount() int { return len(m.activeJobs) }

// AddActiveJob injects a fake job handle for testing purposes.
func (m Model) AddActiveJob(id string) Model {
	if m.activeJobs == nil {
		m.activeJobs = make(map[string]*jobHandle)
	}
	m.activeJobs[id] = &jobHandle{id: id}
	return m
}

// MaxParallelJobs returns the parallel job cap constant — used in tests.
func MaxParallelJobs() int { return maxParallelJobs }

// MakeTickMsg returns a tickMsg for use in tests.
func MakeTickMsg() tea.Msg { return tickMsg(time.Now()) }

// SignalBoardFocused returns the signal board focus state — used in tests.
func (m Model) SignalBoardFocused() bool { return m.signalBoardFocused }

// SetSignalBoardFocused sets the signal board focus state — used in tests.
func (m Model) SetSignalBoardFocused(v bool) Model {
	m.signalBoardFocused = v
	m.launcher.focused = false
	m.agent.focused = false
	m.feedFocused = false
	return m
}

// SetFeedFocused sets the feed focus state — used in tests.
func (m Model) SetFeedFocused(v bool) Model {
	m.feedFocused = v
	m.launcher.focused = false
	m.agent.focused = false
	m.signalBoardFocused = false
	return m
}

// SetFeedSelected sets the selected feed entry index — used in tests.
func (m Model) SetFeedSelected(idx int) Model {
	m.feedSelected = idx
	return m
}


// AddFeedEntry adds a feed entry — used in tests.
func (m Model) AddFeedEntry(id, title string, status FeedStatus, lines []string) Model {
	entry := feedEntry{
		id:     id,
		title:  title,
		status: status,
		ts:     time.Now(),
		lines:  lines,
	}
	m.feed = append([]feedEntry{entry}, m.feed...)
	return m
}

// AddFeedEntryWithTmux adds a feed entry with a tmux window — used in tests.
func (m Model) AddFeedEntryWithTmux(id, title string, status FeedStatus, tmuxWindow string) Model {
	entry := feedEntry{
		id:         id,
		title:      title,
		status:     status,
		ts:         time.Now(),
		tmuxWindow: tmuxWindow,
	}
	m.feed = append([]feedEntry{entry}, m.feed...)
	return m
}

// ── Theme helpers ─────────────────────────────────────────────────────────────

// activeBundle returns the currently active theme bundle, or nil if no registry.
func (m Model) activeBundle() *themes.Bundle {
	if m.registry == nil {
		return nil
	}
	return m.registry.Active()
}

// ansiPalette returns an ANSI escape sequence palette derived from the active bundle.
// Falls back to Dracula hardcoded defaults when no bundle is active.
func (m Model) ansiPalette() styles.ANSIPalette {
	b := m.activeBundle()
	if b == nil {
		return styles.ANSIPalette{
			Accent:  "\x1b[35m",
			Dim:     "\x1b[2m",
			Success: "\x1b[32m",
			Error:   "\x1b[31m",
			FG:      "\x1b[97m",
			BG:      "\x1b[40m",
			Border:  "\x1b[36m",
			SelBG:   "\x1b[44m",
		}
	}
	return styles.BundleANSI(b)
}

// modalColors holds resolved lipgloss colors for modal overlays.
type modalColors struct {
	border  lipgloss.Color
	titleBG lipgloss.Color
	titleFG lipgloss.Color
	fg      lipgloss.Color
	accent  lipgloss.Color
	dim     lipgloss.Color
	error   lipgloss.Color
}

// resolveModalColors derives modal colors from the active bundle with Dracula fallbacks.
func (m Model) resolveModalColors() modalColors {
	c := modalColors{
		border:  lipgloss.Color("#bd93f9"),
		titleBG: lipgloss.Color("#bd93f9"),
		titleFG: lipgloss.Color("#282a36"),
		fg:      lipgloss.Color("#f8f8f2"),
		accent:  lipgloss.Color("#8be9fd"),
		dim:     lipgloss.Color("#6272a4"),
		error:   lipgloss.Color("#ff5555"),
	}
	b := m.activeBundle()
	if b == nil {
		return c
	}
	if v := b.ResolveRef(b.Modal.Border); v != "" {
		c.border = lipgloss.Color(v)
		c.titleBG = lipgloss.Color(v)
	}
	if v := b.ResolveRef(b.Modal.TitleBG); v != "" {
		c.titleBG = lipgloss.Color(v)
	}
	if v := b.ResolveRef(b.Modal.TitleFG); v != "" {
		c.titleFG = lipgloss.Color(v)
	}
	if v := b.Palette.FG; v != "" {
		c.fg = lipgloss.Color(v)
	}
	if v := b.Palette.Accent; v != "" {
		c.accent = lipgloss.Color(v)
	}
	if v := b.Palette.Dim; v != "" {
		c.dim = lipgloss.Color(v)
	}
	if v := b.Palette.Error; v != "" {
		c.error = lipgloss.Color(v)
	}
	return c
}

// ── Init ──────────────────────────────────────────────────────────────────────

// Init starts the tick command.
func (m Model) Init() tea.Cmd { return tickCmd() }

func tickCmd() tea.Cmd {
	return tea.Tick(3*time.Second, func(t time.Time) tea.Msg { return tickMsg(t) })
}

// ── Pipeline helpers ──────────────────────────────────────────────────────────

func pipelinesDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".config", "orcai", "pipelines")
}

// ScanPipelines lists *.pipeline.yaml basenames (without extension) from dir.
// Exported so tests can call it directly.
// Returns an empty slice if dir is missing or empty.
func ScanPipelines(dir string) []string {
	if dir == "" {
		return nil
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var names []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		n := e.Name()
		// Skip dotfiles (e.g. .pipeline.yaml would produce an empty name).
		if len(n) == 0 || n[0] == '.' {
			continue
		}
		if name := strings.TrimSuffix(n, ".pipeline.yaml"); name != n {
			// Only add if the suffix was actually present and name is non-empty.
			if name != "" {
				names = append(names, name)
			}
		}
	}
	return names
}

// ── ChanPublisher ─────────────────────────────────────────────────────────────

// ChanPublisher implements pipeline.EventPublisher and forwards events as
// FeedLineMsg values through a channel consumed by the BubbleTea update loop.
// Exported so tests can construct and verify it.
type ChanPublisher struct {
	id string
	ch chan<- tea.Msg
}

// NewChanPublisher creates a ChanPublisher for the given feed entry id and channel.
func NewChanPublisher(id string, ch chan<- tea.Msg) *ChanPublisher {
	return &ChanPublisher{id: id, ch: ch}
}

// Publish converts a pipeline lifecycle event to a FeedLineMsg and sends it.
func (p *ChanPublisher) Publish(_ context.Context, topic string, payload []byte) error {
	line := fmt.Sprintf("[%s] %s", topic, strings.TrimSpace(string(payload)))
	select {
	case p.ch <- FeedLineMsg{ID: p.id, Line: line}:
	default:
	}
	return nil
}

// lineWriter is an io.Writer that buffers lines and sends FeedLineMsg per line.
type lineWriter struct {
	id  string
	ch  chan<- tea.Msg
	buf bytes.Buffer
}

func (w *lineWriter) Write(p []byte) (int, error) {
	n, err := w.buf.Write(p)
	for {
		data := w.buf.Bytes()
		idx := bytes.IndexByte(data, '\n')
		if idx < 0 {
			break
		}
		line := strings.TrimRight(string(data[:idx]), "\r")
		w.buf.Next(idx + 1)
		if line != "" {
			select {
			case w.ch <- FeedLineMsg{ID: w.id, Line: line}:
			default:
			}
		}
	}
	return n, err
}

func (w *lineWriter) flush() {
	if remaining := strings.TrimSpace(w.buf.String()); remaining != "" {
		select {
		case w.ch <- FeedLineMsg{ID: w.id, Line: remaining}:
		default:
		}
	}
}

var _ io.Writer = (*lineWriter)(nil)

// ── Update ────────────────────────────────────────────────────────────────────

// Update handles tea.Msg values.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {

	// ── Dir picker messages ───────────────────────────────────────────────────

	case dirWalkResultMsg:
		if m.dirPickerOpen {
			var cmd tea.Cmd
			m.dirPicker, cmd = m.dirPicker.Update(msg)
			return m, cmd
		}
		return m, nil

	case DirSelectedMsg:
		m.dirPickerOpen = false
		if m.dirPickerCtx == "agent" {
			m.agentCWD = msg.Path
		} else if m.dirPickerCtx == "pipeline" {
			// Launch the pending pipeline with the selected CWD.
			return m.launchPendingPipeline(msg.Path)
		}
		return m, nil

	case DirCancelledMsg:
		m.dirPickerOpen = false
		m.pendingPipelineName = ""
		m.pendingPipelineYAML = ""
		return m, nil

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		leftW := m.leftColWidth()
		m.agent.prompt.SetWidth(max(leftW-4, 10))
		m.clampFeedScroll()
		return m, nil

	case tickMsg:
		// Toggle blink if any job is running.
		for _, e := range m.feed {
			if e.status == FeedRunning {
				m.signalBoard.blinkOn = !m.signalBoard.blinkOn
				break
			}
		}
		return m, tickCmd()

	case TelemetryMsg:
		line := fmt.Sprintf("telemetry: window=%s provider=%s status=%s", msg.WindowName, msg.Provider, msg.Status)
		entry := feedEntry{
			id:     "tel-" + msg.SessionID,
			title:  "tmux/" + msg.WindowName,
			status: FeedDone,
			ts:     time.Now(),
			lines:  []string{line},
		}
		m.feed = append([]feedEntry{entry}, m.feed...)
		if len(m.feed) > 200 {
			m.feed = m.feed[:200]
		}
		m.feedScrollOffset = 0
		return m, nil

	case FeedLineMsg:
		m = m.appendFeedLine(msg.ID, msg.Line)
		// For in-process (agent) jobs the log file is written here.
		// Window-mode (pipeline) jobs write via tee in the shell — skip.
		for _, e := range m.feed {
			if e.id == msg.ID && e.logFile != "" && e.doneFile == "" {
				appendToFile(e.logFile, stripANSI(msg.Line)+"\n")
				break
			}
		}
		// Re-issue drain only for the job that produced this message.
		// Draining all jobs would accumulate goroutines and starve channels.
		if jh, ok := m.activeJobs[msg.ID]; ok {
			return m, drainChan(jh.ch)
		}
		return m, nil

	case StepStatusMsg:
		for i := range m.feed {
			if m.feed[i].id == msg.FeedID {
				found := false
				for j := range m.feed[i].steps {
					if m.feed[i].steps[j].id == msg.StepID {
						m.feed[i].steps[j].status = msg.Status
						found = true
						break
					}
				}
				if !found {
					m.feed[i].steps = append(m.feed[i].steps, StepInfo{id: msg.StepID, status: msg.Status})
				}
				break
			}
		}
		if jh, ok := m.activeJobs[msg.FeedID]; ok {
			return m, drainChan(jh.ch)
		}
		return m, nil

	case jobDoneMsg:
		// Drain any remaining lines buffered in the channel before marking done.
		if jh, ok := m.activeJobs[msg.id]; ok {
		drainDone:
			for {
				select {
				case buffered, ok := <-jh.ch:
					if !ok {
						break drainDone
					}
					if fl, ok2 := buffered.(FeedLineMsg); ok2 {
						m = m.appendFeedLine(fl.ID, fl.Line)
					}
				default:
					break drainDone
				}
			}
		}
		m = m.setFeedStatus(msg.id, FeedDone)
		delete(m.activeJobs, msg.id)
		return m, nil

	case jobFailedMsg:
		if jh, ok := m.activeJobs[msg.id]; ok {
		drainFailed:
			for {
				select {
				case buffered, ok := <-jh.ch:
					if !ok {
						break drainFailed
					}
					if fl, ok2 := buffered.(FeedLineMsg); ok2 {
						m = m.appendFeedLine(fl.ID, fl.Line)
					}
				default:
					break drainFailed
				}
			}
		}
		m = m.setFeedStatus(msg.id, FeedFailed)
		if msg.err != nil {
			m = m.appendFeedLine(msg.id, "error: "+msg.err.Error())
		}
		delete(m.activeJobs, msg.id)
		return m, nil

	case tea.KeyMsg:
		return m.handleKey(msg)
	}

	return m, nil
}

func (m Model) handleKey(msg tea.KeyMsg) (Model, tea.Cmd) {
	key := msg.String()

	// Dir picker overlay — capture all keys when open.
	if m.dirPickerOpen {
		var cmd tea.Cmd
		m.dirPicker, cmd = m.dirPicker.Update(msg)
		return m, cmd
	}

	// Theme picker — capture all keys when open.
	if m.themePickerOpen {
		return m.handleThemePicker(msg)
	}

	// Help modal.
	if m.helpOpen {
		switch key {
		case "esc", "ctrl+c", "ctrl+h", "q":
			m.helpOpen = false
		case "j", "down":
			m.helpScrollOffset++
		case "k", "up":
			if m.helpScrollOffset > 0 {
				m.helpScrollOffset--
			}
		case "pgdown":
			m.helpScrollOffset += 10
		case "pgup":
			if m.helpScrollOffset > 10 {
				m.helpScrollOffset -= 10
			} else {
				m.helpScrollOffset = 0
			}
		}
		return m, nil
	}

	// Confirm quit when a job is running.
	if m.confirmQuit {
		switch key {
		case "y", "Y", "enter":
			for _, jh := range m.activeJobs {
				jh.cancel()
			}
			exec.Command("tmux", "kill-session", "-t", "orcai").Run() //nolint:errcheck
			return m, tea.Quit
		default:
			m.confirmQuit = false
			return m, nil
		}
	}

	// Delete confirmation modal.
	if m.confirmDelete {
		switch key {
		case "y", "Y":
			return m.execDeletePipeline()
		default:
			m.confirmDelete = false
			m.pendingDeletePipeline = ""
			return m, nil
		}
	}

	// Agent modal — all keys captured before panel handlers.
	if m.agentModalOpen {
		return m.handleAgentModal(msg)
	}

	// Signal board search input — intercept chars when searching.
	if m.signalBoardFocused && m.signalBoard.searching {
		switch key {
		case "esc", "ctrl+c":
			m.signalBoard.searching = false
			if m.signalBoard.query != "" {
				m.signalBoard.query = ""
				m.signalBoard.selectedIdx = 0
				m.signalBoard.scrollOffset = 0
			}
			return m, nil
		case "backspace", "ctrl+h":
			runes := []rune(m.signalBoard.query)
			if len(runes) > 0 {
				m.signalBoard.query = string(runes[:len(runes)-1])
				m.signalBoard.selectedIdx = 0
				m.signalBoard.scrollOffset = 0
			}
			return m, nil
		case "enter":
			m.signalBoard.searching = false
			return m, nil
		case "j", "down":
			return m.handleDown(), nil
		case "k", "up":
			return m.handleUp(), nil
		default:
			if len(msg.Runes) == 1 {
				m.signalBoard.query += string(msg.Runes[0])
				m.signalBoard.selectedIdx = 0
				m.signalBoard.scrollOffset = 0
				return m, nil
			}
		}
		return m, nil
	}

	// / enters signal board search mode.
	if m.signalBoardFocused && key == "/" {
		m.signalBoard.searching = true
		return m, nil
	}

	// Global focus shortcuts — p focuses pipelines.
	switch key {
	case "p":
		m.launcher.focused = true
		m.agent.focused = false
		m.feedFocused = false
		m.signalBoardFocused = false
		return m, nil
	}

	switch key {
	case "ctrl+h":
		m.helpOpen = true
		m.helpScrollOffset = 0
		return m, nil

	case "ctrl+c", "ctrl+q":
		m.confirmQuit = true
		return m, nil

	case "tab":
		if m.feedFocused {
			// feed → launcher
			m.feedFocused = false
			m.launcher.focused = true
		} else if m.signalBoardFocused {
			// signalBoard → feed
			m.signalBoardFocused = false
			m.signalBoard.clearSearch()
			m.feedFocused = true
			m.feedCursor = 0
		} else if m.agent.focused {
			// agent → signalBoard
			m.agent.focused = false
			m.signalBoardFocused = true
		} else if m.launcher.focused {
			// launcher → agent
			m.launcher.focused = false
			m.agent.focused = true
		}
		return m, nil

	case "f":
		if m.signalBoardFocused {
			m.signalBoard.cycleFilter()
		} else {
			// Toggle activity feed focus so ↑↓ scrolls through output lines.
			m.launcher.focused = false
			m.agent.focused = false
			m.signalBoardFocused = false
			m.feedFocused = !m.feedFocused
		}
		return m, nil

	case "a":
		m.launcher.focused = false
		m.agent.focused = true
		m.feedFocused = false
		return m, nil

	case "s":
		m.launcher.focused = false
		m.agent.focused = false
		m.feedFocused = false
		m.signalBoardFocused = true
		return m, nil

	case "r":
		m.launcher.pipelines = ScanPipelines(pipelinesDir())
		if m.launcher.selected >= len(m.launcher.pipelines) && m.launcher.selected > 0 {
			m.launcher.selected = max(len(m.launcher.pipelines)-1, 0)
		}
		m.agent.providers = picker.BuildProviders()
		m.agent.selectedProvider = 0
		m.agent.selectedModel = 0
		return m, nil

	case "T":
		if !m.agentModalOpen && !m.confirmDelete && !m.confirmQuit {
			m.themePickerOpen = true
			m.themePickerCursor = 0
		}
		return m, nil

	case "pgdown":
		if m.feedFocused {
			total := totalFeedLines(m.feed)
			step := m.feedVisibleHeight()
			m.feedCursor = min(m.feedCursor+step, max(0, total-1))
			m.feedScrollOffset = m.feedCursor
			m.clampFeedScroll()
			return m, nil
		}

	case "pgup":
		if m.feedFocused {
			step := m.feedVisibleHeight()
			m.feedCursor = max(m.feedCursor-step, 0)
			m.feedScrollOffset = m.feedCursor
			m.clampFeedScroll()
			return m, nil
		}

	case "g":
		if m.feedFocused {
			m.feedCursor = 0
			m.feedScrollOffset = 0
			return m, nil
		}

	case "G":
		if m.feedFocused {
			total := totalFeedLines(m.feed)
			m.feedCursor = max(0, total-1)
			m.feedScrollOffset = m.feedCursor
			m.clampFeedScroll()
			return m, nil
		}

	case "j", "down":
		return m.handleDown(), nil

	case "k", "up":
		return m.handleUp(), nil

	case "esc":
		if m.feedFocused {
			m.feedFocused = false
			m.launcher.focused = true
			return m, nil
		} else if m.signalBoardFocused {
			m.signalBoardFocused = false
			m.signalBoard.clearSearch()
			m.launcher.focused = true
		} else if m.agent.focused {
			m.agent.focused = false
			m.launcher.focused = true
		}
		return m, nil

	// Pipeline CRUD keys (launcher focused).
	case "n":
		if m.launcher.focused {
			return m.handleNewPipeline()
		}

	case "e":
		if m.launcher.focused && len(m.launcher.pipelines) > 0 {
			return m.handleEditPipeline()
		}

	case "d":
		if m.launcher.focused && len(m.launcher.pipelines) > 0 {
			m.confirmDelete = true
			m.pendingDeletePipeline = m.launcher.pipelines[m.launcher.selected]
			return m, nil
		}

	case "enter":
		return m.handleEnter()
	}

	return m, nil
}

// feedVisibleHeight returns an approximation of the number of visible lines in the feed panel.
func (m Model) feedVisibleHeight() int {
	h := m.height
	if h <= 0 {
		h = 40
	}
	v := h/2 - 4
	if v < 1 {
		v = 1
	}
	return v
}

func (m Model) handleDown() Model {
	if m.feedFocused {
		total := totalFeedLines(m.feed)
		m.feedCursor = min(m.feedCursor+1, max(0, total-1))
		m.feedScrollOffset = m.feedCursor
		m.clampFeedScroll()
		return m
	}
	if m.signalBoardFocused {
		filtered := fuzzyFeed(m.signalBoard.query, m.filteredFeed())
		if m.signalBoard.selectedIdx < len(filtered)-1 {
			m.signalBoard.selectedIdx++
			m.signalBoard.clampScroll(m.signalBoardVisibleRows())
		}
		return m
	}
	if m.launcher.focused {
		if m.launcher.selected < len(m.launcher.pipelines)-1 {
			m.launcher.selected++
		}
		return m
	}
	if m.agent.focused {
		if m.agent.selectedProvider < len(m.agent.providers)-1 {
			m.agent.selectedProvider++
		}
	}
	return m
}

func (m Model) handleUp() Model {
	if m.feedFocused {
		m.feedCursor = max(m.feedCursor-1, 0)
		m.feedScrollOffset = m.feedCursor
		m.clampFeedScroll()
		return m
	}
	if m.signalBoardFocused {
		if m.signalBoard.selectedIdx > 0 {
			m.signalBoard.selectedIdx--
			m.signalBoard.clampScroll(m.signalBoardVisibleRows())
		}
		return m
	}
	if m.launcher.focused {
		if m.launcher.selected > 0 {
			m.launcher.selected--
		}
		return m
	}
	if m.agent.focused {
		if m.agent.selectedProvider > 0 {
			m.agent.selectedProvider--
		}
	}
	return m
}

// handleAgentModal routes key events when the agent modal overlay is open.
func (m Model) handleAgentModal(msg tea.KeyMsg) (Model, tea.Cmd) {
	key := msg.String()
	switch key {
	case "esc", "ctrl+c":
		m.agentModalOpen = false
		m.agent.prompt.Blur()
		return m, nil

	case "ctrl+s":
		return m.submitAgentJob()

	case "tab":
		m.agentModalFocus = (m.agentModalFocus + 1) % 4
		if m.agentModalFocus == 2 {
			m.agent.prompt.Focus()
		} else {
			m.agent.prompt.Blur()
		}
		return m, nil

	case "shift+tab":
		m.agentModalFocus = (m.agentModalFocus + 3) % 4
		if m.agentModalFocus == 2 {
			m.agent.prompt.Focus()
		} else {
			m.agent.prompt.Blur()
		}
		return m, nil

	case "enter":
		if m.agentModalFocus == 3 {
			// Open dir picker to select CWD.
			m.dirPicker = NewDirPickerModel()
			m.dirPickerOpen = true
			m.dirPickerCtx = "agent"
			return m, DirPickerInit()
		}

	case "up", "k":
		switch m.agentModalFocus {
		case 0:
			if m.agent.selectedProvider > 0 {
				m.agent.selectedProvider--
			}
		case 1:
			if m.agent.selectedModel > 0 {
				m.agent.selectedModel--
			}
		}
		return m, nil

	case "down", "j":
		switch m.agentModalFocus {
		case 0:
			if m.agent.selectedProvider < len(m.agent.providers)-1 {
				m.agent.selectedProvider++
			}
		case 1:
			prov := m.currentProvider()
			if prov != nil {
				models := nonSepModels(prov.Models)
				if m.agent.selectedModel < len(models)-1 {
					m.agent.selectedModel++
				}
			}
		}
		return m, nil

	default:
		if m.agentModalFocus == 2 {
			var cmd tea.Cmd
			m.agent.prompt, cmd = m.agent.prompt.Update(msg)
			return m, cmd
		}
	}
	return m, nil
}

// handleNewPipeline creates a template pipeline file and opens it in $EDITOR.
func (m Model) handleNewPipeline() (Model, tea.Cmd) {
	dir := pipelinesDir()
	if dir == "" {
		return m, nil
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return m, nil
	}
	name := fmt.Sprintf("new-pipeline-%d", time.Now().Unix())
	path := filepath.Join(dir, name+".pipeline.yaml")
	template := "name: " + name + "\nsteps:\n  - name: hello\n    run: echo \"hello world\"\n"
	if err := os.WriteFile(path, []byte(template), 0o600); err != nil {
		return m, nil
	}
	openEditorInWindow(path)
	// Refresh pipeline list so the new entry is immediately visible.
	m.launcher.pipelines = ScanPipelines(dir)
	if m.launcher.selected >= len(m.launcher.pipelines) && m.launcher.selected > 0 {
		m.launcher.selected = max(len(m.launcher.pipelines)-1, 0)
	}
	return m, nil
}

// handleEditPipeline opens the selected pipeline's YAML file in $EDITOR.
func (m Model) handleEditPipeline() (Model, tea.Cmd) {
	if len(m.launcher.pipelines) == 0 {
		return m, nil
	}
	name := m.launcher.pipelines[m.launcher.selected]
	path := filepath.Join(pipelinesDir(), name+".pipeline.yaml")
	openEditorInWindow(path)
	return m, nil
}

// execDeletePipeline removes the pending pipeline file and refreshes the list.
func (m Model) execDeletePipeline() (Model, tea.Cmd) {
	m.confirmDelete = false
	name := m.pendingDeletePipeline
	m.pendingDeletePipeline = ""
	if name == "" {
		return m, nil
	}
	path := filepath.Join(pipelinesDir(), name+".pipeline.yaml")
	os.Remove(path) //nolint:errcheck
	m.launcher.pipelines = ScanPipelines(pipelinesDir())
	if m.launcher.selected >= len(m.launcher.pipelines) {
		m.launcher.selected = max(len(m.launcher.pipelines)-1, 0)
	}
	return m, nil
}

// openEditorInWindow opens path in $EDITOR (or vi) via a new tmux window and
// switches the user to it immediately.
func openEditorInWindow(path string) {
	editor := os.Getenv("EDITOR")
	if editor == "" {
		editor = "vi"
	}
	session := currentTmuxSession()
	if session == "" {
		return
	}
	cmd := editor + " " + path
	out, err := exec.Command("tmux", "new-window", "-d", "-P", "-F", "#{window_id}", "-t", session+":", "-n", "orcai-edit", cmd).Output()
	if err != nil {
		return
	}
	winID := strings.TrimSpace(string(out))
	if winID != "" {
		exec.Command("tmux", "select-window", "-t", session+":"+winID).Run() //nolint:errcheck
	}
}

func (m Model) handleEnter() (Model, tea.Cmd) {
	// Signal board: navigate directly into the job's tmux window.
	if m.signalBoardFocused {
		filtered := fuzzyFeed(m.signalBoard.query, m.filteredFeed())
		if len(filtered) > 0 && m.signalBoard.selectedIdx < len(filtered) {
			tw := filtered[m.signalBoard.selectedIdx].tmuxWindow
			if tw != "" {
				exec.Command("tmux", "select-window", "-t", tw).Run() //nolint:errcheck
			}
		}
		return m, nil
	}

	// Launcher: show dir picker before launching pipeline.
	if m.launcher.focused {
		if len(m.launcher.pipelines) == 0 {
			return m, nil
		}
		// Enforce parallel job cap before opening the picker.
		if len(m.activeJobs) >= maxParallelJobs {
			feedID := fmt.Sprintf("warn-%d", time.Now().UnixNano())
			warnEntry := feedEntry{
				id:     feedID,
				title:  "warning",
				status: FeedFailed,
				ts:     time.Now(),
				lines:  []string{"max parallel jobs reached (8)"},
			}
			m.feed = append([]feedEntry{warnEntry}, m.feed...)
			if len(m.feed) > 200 {
				m.feed = m.feed[:200]
			}
			return m, nil
		}

		name := m.launcher.pipelines[m.launcher.selected]
		yamlPath := filepath.Join(pipelinesDir(), name+".pipeline.yaml")

		// Open the dir picker so the user can choose the working directory
		// before the pipeline runs.
		m.pendingPipelineName = name
		m.pendingPipelineYAML = yamlPath
		m.dirPicker = NewDirPickerModel()
		m.dirPickerOpen = true
		m.dirPickerCtx = "pipeline"
		return m, DirPickerInit()
	}
	// Agent section: open modal overlay.
	if m.agent.focused {
		if m.width >= 62 {
			m.agentModalOpen = true
			m.agentModalFocus = 0 // start at provider so user confirms selection
			m.agent.prompt.Blur()
		}
		return m, nil
	}

	return m, nil
}

// launchPendingPipeline runs the pipeline stored in pendingPipelineName/YAML
// using cwd as the working directory. Called when the dir picker confirms.
func (m Model) launchPendingPipeline(cwd string) (Model, tea.Cmd) {
	name := m.pendingPipelineName
	yamlPath := m.pendingPipelineYAML
	m.pendingPipelineName = ""
	m.pendingPipelineYAML = ""

	if name == "" || yamlPath == "" {
		return m, nil
	}

	feedID := fmt.Sprintf("pipe-%d", time.Now().UnixNano())
	entry := feedEntry{
		id:     feedID,
		title:  "pipeline: " + name,
		status: FeedRunning,
		ts:     time.Now(),
	}
	// Load pipeline YAML to populate the initial step list.
	if f, err := os.Open(yamlPath); err == nil {
		if pl, err := pipeline.Load(f); err == nil {
			for _, s := range pl.Steps {
				if s.Type != "input" && s.Type != "output" {
					entry.steps = append(entry.steps, StepInfo{id: s.ID, status: "pending"})
				}
			}
		}
		f.Close()
	}
	m.feed = append([]feedEntry{entry}, m.feed...)
	if len(m.feed) > 200 {
		m.feed = m.feed[:200]
	}
	m.feedSelected = 0
	m.feedScrollOffset = 0

	orcaiBin := orcaiBinaryPath()
	shellCmd := orcaiBin + " pipeline run " + yamlPath
	windowName, logFile, doneFile := createJobWindow(feedID, shellCmd, name, cwd)
	entry.tmuxWindow = windowName
	entry.logFile = logFile
	entry.doneFile = doneFile
	m.feed[0] = entry

	ch := make(chan tea.Msg, 256)
	_, cancel := context.WithCancel(context.Background())
	m.activeJobs[feedID] = &jobHandle{id: feedID, cancel: cancel, ch: ch, tmuxWindow: windowName, logFile: logFile}

	startLogWatcher(feedID, logFile, doneFile, ch)
	return m, drainChan(ch)
}

// submitAgentJob submits the agent job from the modal overlay.
func (m Model) submitAgentJob() (Model, tea.Cmd) {
	input := strings.TrimSpace(m.agent.prompt.Value())
	if input == "" {
		return m, nil
	}
	prov := m.currentProvider()
	if prov == nil {
		return m, nil
	}

	// Enforce parallel job cap.
	if len(m.activeJobs) >= maxParallelJobs {
		feedID := fmt.Sprintf("warn-%d", time.Now().UnixNano())
		warnEntry := feedEntry{
			id:     feedID,
			title:  "warning",
			status: FeedFailed,
			ts:     time.Now(),
			lines:  []string{"max parallel jobs reached (8)"},
		}
		m.feed = append([]feedEntry{warnEntry}, m.feed...)
		if len(m.feed) > 200 {
			m.feed = m.feed[:200]
		}
		return m, nil
	}

	var modelID string
	models := nonSepModels(prov.Models)
	if len(models) > 0 && m.agent.selectedModel < len(models) {
		modelID = models[m.agent.selectedModel].ID
	}

	title := "agent: " + prov.ID
	if modelID != "" {
		title += "/" + modelID
	}

	feedID := fmt.Sprintf("agent-%d", time.Now().UnixNano())
	entry := feedEntry{
		id:     feedID,
		title:  title,
		status: FeedRunning,
		ts:     time.Now(),
	}
	m.feed = append([]feedEntry{entry}, m.feed...)
	if len(m.feed) > 200 {
		m.feed = m.feed[:200]
	}
	m.feedSelected = 0
	m.feedScrollOffset = 0

	// Agent runs in-process; window shows live output via tail.
	// Pass agentCWD as the tmux window start directory so the tail window
	// opens in the selected project directory.
	cwd := m.agentCWD
	if cwd == "" {
		cwd = m.launchCWD
	}

	// If the selected CWD is inside a git repo, create (or reuse) a worktree
	// so the agent session has an isolated branch to work on.
	if worktreePath, _ := picker.GetOrCreateWorktreeFrom(cwd, feedID); worktreePath != "" {
		picker.CopyDotEnv(cwd, worktreePath)
		cwd = worktreePath
	}

	windowName, logFile, _ := createJobWindow(feedID, "", title, cwd)
	entry.tmuxWindow = windowName
	entry.logFile = logFile
	m.feed[0] = entry

	provArgs := picker.PipelineLaunchArgs(prov.ID)
	binary := prov.Command
	if binary == "" {
		binary = prov.ID
	}
	adapter := plugin.NewCliAdapter(prov.ID, prov.Label+" CLI adapter", binary, provArgs...)
	vars := map[string]string{}
	if modelID != "" {
		vars["model"] = modelID
	}
	if cwd != "" {
		vars["cwd"] = cwd
	}

	ch := make(chan tea.Msg, 256)
	_, cancel := context.WithCancel(context.Background())
	m.activeJobs[feedID] = &jobHandle{id: feedID, cancel: cancel, ch: ch, tmuxWindow: windowName, logFile: logFile}

	cmd := runAgentCmdCh(adapter, input, vars, feedID, ch, cancel)
	drain := drainChan(ch)

	// Close modal and reset prompt after submission.
	m.agentModalOpen = false
	m.agent.prompt.SetValue("")
	m.agent.prompt.Blur()

	return m, tea.Batch(cmd, drain)
}

// runAgentCmdCh starts CliAdapter.Execute in a goroutine, streaming output to ch.
func runAgentCmdCh(adapter *plugin.CliAdapter, input string, vars map[string]string, feedID string, ch chan tea.Msg, cancel context.CancelFunc) tea.Cmd {
	return func() tea.Msg {
		defer cancel()
		w := &lineWriter{id: feedID, ch: ch}
		err := adapter.Execute(context.Background(), input, vars, w)
		w.flush()
		if err != nil {
			ch <- jobFailedMsg{id: feedID, err: err}
		} else {
			ch <- jobDoneMsg{id: feedID}
		}
		return nil
	}
}

// orcaiBinaryPath returns the path to the running orcai binary, falling back
// to a PATH lookup and finally the bare name.
func orcaiBinaryPath() string {
	if exe, err := os.Executable(); err == nil {
		return exe
	}
	if p, err := exec.LookPath("orcai"); err == nil {
		return p
	}
	return "orcai"
}

// drainChan returns a tea.Cmd that blocks until a message arrives on ch.
func drainChan(ch chan tea.Msg) tea.Cmd {
	return func() tea.Msg {
		msg, ok := <-ch
		if !ok {
			return nil
		}
		return msg
	}
}

// ── Feed helpers ──────────────────────────────────────────────────────────────

func (m Model) appendFeedLine(id, line string) Model {
	// Strip carriage returns — progress bars use \r for in-place updates.
	// Keep only the last "frame" so the displayed text is clean.
	if idx := strings.LastIndexByte(line, '\r'); idx >= 0 {
		line = line[idx+1:]
	}
	if line == "" {
		return m
	}
	for i := range m.feed {
		if m.feed[i].id == id {
			m.feed[i].lines = append(m.feed[i].lines, line)
			return m
		}
	}
	return m
}

func (m Model) setFeedStatus(id string, status FeedStatus) Model {
	for i := range m.feed {
		if m.feed[i].id == id {
			m.feed[i].status = status
			return m
		}
	}
	return m
}

// clampFeedScroll clamps feedScrollOffset to valid range.
func (m *Model) clampFeedScroll() {
	h := m.height
	if h <= 0 {
		h = 40
	}
	contentH := max(h-1, 5)
	maxSBClamp := max(contentH*40/100, 8)
	sbHeight := min(len(m.feed)+6, maxSBClamp)
	if sbHeight < 5 {
		sbHeight = 5
	}
	if sbHeight > contentH-3 {
		sbHeight = max(contentH-3, 5)
	}
	feedH := max(contentH-sbHeight, 3)
	visibleH := feedH - 2
	if visibleH <= 0 {
		visibleH = 1
	}
	total := totalFeedLines(m.feed)
	maxOffset := max(0, total-visibleH)
	if m.feedScrollOffset > maxOffset {
		m.feedScrollOffset = maxOffset
	}
	if m.feedScrollOffset < 0 {
		m.feedScrollOffset = 0
	}
}

// filteredFeed returns feed entries matching the current signal board filter.
func (m Model) filteredFeed() []feedEntry {
	filter := m.signalBoard.activeFilter
	if filter == "all" || filter == "" {
		return m.feed
	}
	var out []feedEntry
	for _, e := range m.feed {
		switch filter {
		case "running":
			if e.status == FeedRunning {
				out = append(out, e)
			}
		case "done":
			if e.status == FeedDone {
				out = append(out, e)
			}
		case "failed":
			if e.status == FeedFailed {
				out = append(out, e)
			}
		}
	}
	return out
}

func (m Model) currentProvider() *picker.ProviderDef {
	if len(m.agent.providers) == 0 {
		return nil
	}
	if m.agent.selectedProvider >= len(m.agent.providers) {
		return &m.agent.providers[0]
	}
	return &m.agent.providers[m.agent.selectedProvider]
}

// nonSepModels filters separator entries from a model list.
func nonSepModels(models []picker.ModelOption) []picker.ModelOption {
	var out []picker.ModelOption
	for _, mo := range models {
		if !mo.Separator {
			out = append(out, mo)
		}
	}
	return out
}

// ── View ──────────────────────────────────────────────────────────────────────

// View renders the full-screen switchboard layout.
func (m Model) View() string {
	w := m.width
	if w <= 0 {
		w = 120
	}
	h := m.height
	if h <= 0 {
		h = 40
	}

	leftW := m.leftColWidth()
	feedW := max(w-leftW-1, 20)
	contentH := max(h-1, 5) // reserve one line for bottom bar

	// Signal board: grows with entries up to 40% of screen height.
	// Minimum 5 rows so the box is always visible (header+border).
	maxSB := max(contentH*40/100, 8)
	sbHeight := min(len(m.feed)+6, maxSB)
	if sbHeight < 5 {
		sbHeight = 5
	}
	// Clamp sbHeight so feedH is at least 3.
	if sbHeight > contentH-3 {
		sbHeight = max(contentH-3, 5)
	}
	feedH := max(contentH-sbHeight, 3)

	left := m.viewLeftColumn(contentH, leftW)
	sb := m.buildSignalBoard(sbHeight, feedW)
	feed := m.viewActivityFeed(feedH, feedW)

	// Right column: signal board lines followed by feed lines, clipped to contentH.
	rightLines := append(sb, strings.Split(feed, "\n")...)
	if len(rightLines) > contentH {
		rightLines = rightLines[:contentH]
	}

	leftLines := strings.Split(left, "\n")
	totalRows := max(len(leftLines), len(rightLines))

	var rows []string
	for i := range totalRows {
		l := ""
		if i < len(leftLines) {
			l = leftLines[i]
		}
		f := ""
		if i < len(rightLines) {
			f = rightLines[i]
		}
		rows = append(rows, padToVis(l, leftW)+" "+f)
	}

	body := strings.Join(rows, "\n")

	// Dir picker overlay — highest priority, shown on top of everything.
	if m.dirPickerOpen {
		base := body + "\n" + m.viewBottomBar(w)
		return overlayCenter(base, m.dirPicker.viewDirPickerBox(w), w, h)
	}

	if m.helpOpen {
		base := body + "\n" + m.viewBottomBar(w)
		return overlayCenter(base, m.viewHelpModal(w, h), w, h)
	}

	if m.confirmQuit {
		base := body + "\n" + m.viewBottomBar(w)
		return overlayCenter(base, m.viewQuitModalBox(w), w, h)
	}

	// Agent modal — floating overlay on top of the switchboard.
	if m.agentModalOpen {
		base := body + "\n" + m.viewBottomBar(w)
		return overlayCenter(base, m.viewAgentModalBox(w), w, h)
	}

	// Delete confirmation — floating overlay on top of the switchboard.
	if m.confirmDelete {
		base := body + "\n" + m.viewBottomBar(w)
		return overlayCenter(base, m.viewDeleteModalBox(w), w, h)
	}

	// Theme picker overlay.
	if m.themePickerOpen && m.registry != nil {
		base := body + "\n" + m.viewBottomBar(w)
		bundles := m.registry.All()
		content := viewThemePicker(bundles, m.themePickerCursor, m.registry.Active(), w)
		return overlayCenter(base, content, w, h)
	}

	return body + "\n" + m.viewBottomBar(w)
}

// viewQuitModalBox renders the quit confirmation popup box.
func (m Model) viewQuitModalBox(w int) string {
	jobs := len(m.activeJobs)
	jobWord := "job"
	if jobs != 1 {
		jobWord = "jobs"
	}

	innerW := 44
	if innerW+4 > w {
		innerW = max(w-4, 10)
	}
	outerW := innerW + 2

	mc := m.resolveModalColors()

	headerStyle := lipgloss.NewStyle().
		Background(mc.titleBG).
		Foreground(mc.titleFG).
		Bold(true).
		Width(innerW).
		Padding(0, 1)

	rowStyle := func(fg lipgloss.Color) lipgloss.Style {
		return lipgloss.NewStyle().Foreground(fg).Width(innerW).Padding(0, 1)
	}

	rows := []string{headerStyle.Render("ORCAI  Quit?")}
	if jobs > 0 {
		rows = append(rows, rowStyle(mc.error).Render(fmt.Sprintf("%d %s still running.", jobs, jobWord)))
	} else {
		rows = append(rows, rowStyle(mc.fg).Render("Quit ORCAI?"))
	}
	rows = append(rows,
		"",
		rowStyle(mc.fg).Render(
			lipgloss.NewStyle().Foreground(mc.accent).Bold(true).Render("[y]")+"es   "+
				lipgloss.NewStyle().Foreground(mc.dim).Render("[n]")+"o / esc",
		),
	)

	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(mc.border).
		Width(outerW).
		Render(strings.Join(rows, "\n"))
}

// viewDeleteModalBox renders just the delete confirmation popup box (no
// positioning). overlayCenter places it over the base view.
func (m Model) viewDeleteModalBox(w int) string {
	name := m.pendingDeletePipeline
	fullPath := filepath.Join(pipelinesDir(), name+".pipeline.yaml")

	// Shorten path for display: replace $HOME with ~.
	displayPath := fullPath
	if home, err := os.UserHomeDir(); err == nil {
		displayPath = strings.Replace(fullPath, home, "~", 1)
	}

	// innerW = text content width (row Width). Rows are rendered at innerW,
	// then Padding(0,1) adds 2, giving each row a total of innerW+2.
	// The outer border uses Width(innerW+2) so it matches exactly.
	innerW := max(len(displayPath)+4, 48)
	if innerW > 68 {
		innerW = 68
	}
	if innerW+4 > w { // +4 = 2 padding + 2 border
		innerW = max(w-4, 10)
	}
	outerW := innerW + 2 // +2 for Padding(0,1) on each row

	mc := m.resolveModalColors()

	rowStyle := func(fg lipgloss.Color) lipgloss.Style {
		return lipgloss.NewStyle().Foreground(fg).Width(innerW).Padding(0, 1)
	}

	headerStyle := lipgloss.NewStyle().
		Background(mc.titleBG).
		Foreground(mc.titleFG).
		Bold(true).
		Width(innerW).
		Padding(0, 1)

	rows := []string{
		headerStyle.Render("ORCAI  Delete Pipeline"),
		rowStyle(mc.accent).Bold(true).Render(name),
		rowStyle(mc.dim).Render(displayPath),
		"",
		rowStyle(mc.fg).Render(
			lipgloss.NewStyle().Foreground(mc.accent).Render("[y]") + "es   " +
				lipgloss.NewStyle().Foreground(mc.dim).Render("[n]") + "o / esc",
		),
	}

	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(mc.border).
		Width(outerW).
		Render(strings.Join(rows, "\n"))
}

// viewAgentModal renders the full-screen agent overlay.
// viewAgentModalBox renders the agent modal box content only. overlayCenter
// places it over the base view.
func (m Model) viewAgentModalBox(w int) string {
	modalW := min(max(w-4, 60), 90)
	if w < 62 {
		modalW = w
	}

	pal := m.ansiPalette()
	modalBorderColor := aPur
	if b := m.activeBundle(); b != nil {
		if border := b.ResolveRef(b.Modal.Border); border != "" {
			r, g, bv := hexToRGBFromStyles(border)
			modalBorderColor = fmt.Sprintf("\x1b[38;2;%d;%d;%dm", r, g, bv)
		}
	}

	hint := func(key, desc string) string {
		return pal.Accent + key + pal.Dim + " " + desc + aRst
	}
	sep := pal.Dim + " · " + aRst

	sectionLabel := func(title string, active bool) string {
		if active {
			return pal.Accent + aBld + title + aRst
		}
		return pal.Dim + title + aRst
	}

	var rows []string
	rows = append(rows, boxTop(modalW, "AGENT", modalBorderColor, modalBorderColor))

	// ── PROVIDER section ──────────────────────────────────────────────────────
	provHeader := "  " + sectionLabel("PROVIDER", m.agentModalFocus == 0)
	rows = append(rows, boxRow(provHeader, modalW, modalBorderColor))
	if len(m.agent.providers) == 0 {
		rows = append(rows, boxRow(pal.Dim+"  no providers"+aRst, modalW, modalBorderColor))
	} else {
		const provWindow = 4
		offset := m.agent.agentScrollOffset
		if m.agent.selectedProvider < offset {
			offset = m.agent.selectedProvider
		} else if m.agent.selectedProvider >= offset+provWindow {
			offset = m.agent.selectedProvider - provWindow + 1
		}
		end := min(offset+provWindow, len(m.agent.providers))
		for i := offset; i < end; i++ {
			prov := m.agent.providers[i]
			label := prov.Label
			if label == "" {
				label = prov.ID
			}
			maxLen := max(modalW-6, 1)
			if len(label) > maxLen {
				label = label[:maxLen-1] + "…"
			}
			contentVis := 4 + len(label)
			if i == m.agent.selectedProvider {
				sel := pal.Accent
				if m.agentModalFocus == 0 {
					sel = pal.SelBG + aWht
				}
				content := sel + "  > " + label + aRst
				rows = append(rows, modalBorderColor+"│"+content+strings.Repeat(" ", max(modalW-2-contentVis, 0))+modalBorderColor+"│"+aRst)
			} else {
				content := pal.Dim + "    " + pal.Accent + label + aRst
				rows = append(rows, boxRow(content, modalW, modalBorderColor))
			}
		}
	}

	// ── MODEL section ─────────────────────────────────────────────────────────
	rows = append(rows, boxRow("", modalW, modalBorderColor))
	modelHeader := "  " + sectionLabel("MODEL", m.agentModalFocus == 1)
	rows = append(rows, boxRow(modelHeader, modalW, modalBorderColor))
	prov := m.currentProvider()
	if prov == nil {
		rows = append(rows, boxRow(pal.Dim+"  select a provider first"+aRst, modalW, modalBorderColor))
	} else {
		models := nonSepModels(prov.Models)
		if len(models) == 0 {
			rows = append(rows, boxRow(pal.Dim+"  no models"+aRst, modalW, modalBorderColor))
		} else {
			const modelWindow = 4
			offset := m.agent.agentModelScrollOffset
			if m.agent.selectedModel < offset {
				offset = m.agent.selectedModel
			} else if m.agent.selectedModel >= offset+modelWindow {
				offset = m.agent.selectedModel - modelWindow + 1
			}
			end := min(offset+modelWindow, len(models))
			for i := offset; i < end; i++ {
				mo := models[i]
				label := mo.Label
				if label == "" {
					label = mo.ID
				}
				maxLen := max(modalW-6, 1)
				if len(label) > maxLen {
					label = label[:maxLen-1] + "…"
				}
				contentVis := 4 + len(label)
				if i == m.agent.selectedModel {
					sel := pal.Accent
					if m.agentModalFocus == 1 {
						sel = pal.SelBG + aWht
					}
					content := sel + "  > " + label + aRst
					rows = append(rows, modalBorderColor+"│"+content+strings.Repeat(" ", max(modalW-2-contentVis, 0))+modalBorderColor+"│"+aRst)
				} else {
					content := pal.Dim + "    " + pal.Accent + label + aRst
					rows = append(rows, boxRow(content, modalW, modalBorderColor))
				}
			}
		}
	}

	// ── PROMPT section ────────────────────────────────────────────────────────
	rows = append(rows, boxRow("", modalW, modalBorderColor))
	promptHeader := "  " + sectionLabel("PROMPT", m.agentModalFocus == 2)
	rows = append(rows, boxRow(promptHeader, modalW, modalBorderColor))
	if len(m.activeJobs) > 0 {
		warn := pal.Error + fmt.Sprintf("  ⚠ %d job(s) running", len(m.activeJobs)) + aRst
		rows = append(rows, boxRow(warn, modalW, modalBorderColor))
	}
	promptInnerW := max(modalW-6, 10)
	m.agent.prompt.SetWidth(promptInnerW)
	for _, pLine := range strings.Split(m.agent.prompt.View(), "\n") {
		pLine = strings.TrimRight(pLine, "\r")
		padded := "  " + pLine
		rows = append(rows, boxRow(padded, modalW, modalBorderColor))
	}

	// ── WORKING DIRECTORY section ─────────────────────────────────────────────
	rows = append(rows, boxRow("", modalW, modalBorderColor))
	cwdHeader := "  " + sectionLabel("WORKING DIRECTORY", m.agentModalFocus == 3)
	rows = append(rows, boxRow(cwdHeader, modalW, modalBorderColor))
	cwdDisplay := m.agentCWD
	if cwdDisplay == "" {
		cwdDisplay = m.launchCWD
	}
	// Trim to home-relative display for readability.
	if home := os.Getenv("HOME"); home != "" && strings.HasPrefix(cwdDisplay, home) {
		cwdDisplay = "~" + cwdDisplay[len(home):]
	}
	// Truncate long paths to fit the modal.
	maxCWDLen := max(modalW-8, 10)
	if len(cwdDisplay) > maxCWDLen {
		cwdDisplay = "…" + cwdDisplay[len(cwdDisplay)-maxCWDLen+1:]
	}
	var cwdContent string
	if m.agentModalFocus == 3 {
		cwdContent = pal.SelBG + aWht + "  > " + cwdDisplay + aRst
	} else {
		cwdContent = pal.Dim + "    " + aRst + pal.Accent + cwdDisplay + aRst
	}
	rows = append(rows, boxRow(cwdContent, modalW, modalBorderColor))
	if m.agentModalFocus == 3 {
		rows = append(rows, boxRow(pal.Dim+"  enter to browse directories"+aRst, modalW, modalBorderColor))
	}

	// ── Hint bar ──────────────────────────────────────────────────────────────
	rows = append(rows, boxRow("", modalW, modalBorderColor))
	hintParts := []string{
		hint("tab", "focus"),
		hint("ctrl+s", "submit"),
		hint("esc", "close"),
	}
	if m.agentModalFocus == 3 {
		hintParts = append([]string{hint("enter", "browse dirs")}, hintParts...)
	}
	hintStr := "  " + strings.Join(hintParts, sep)
	rows = append(rows, boxRow(hintStr, modalW, modalBorderColor))
	rows = append(rows, boxBot(modalW, modalBorderColor))

	return strings.Join(rows, "\n")
}

// sectionLabel returns a section header with focus indicator.
func sectionLabel(title string, focused bool) string {
	if focused {
		return aBrC + aBld + title + aRst
	}
	return aDim + title + aRst
}

func (m Model) leftColWidth() int {
	w := m.width
	if w <= 0 {
		w = 120
	}
	lw := w * 30 / 100
	if lw < 28 {
		lw = 28
	}
	return lw
}

// viewLeftColumn renders the left column: banner + launcher + agent sections.
func (m Model) viewLeftColumn(height, width int) string {
	var lines []string

	banner := m.buildBanner(width)
	lines = append(lines, strings.Split(banner, "\n")...)
	lines = append(lines, "")

	lines = append(lines, m.buildLauncherSection(width)...)
	lines = append(lines, "")

	lines = append(lines, m.buildAgentSection(width)...)

	for len(lines) < height {
		lines = append(lines, "")
	}
	if len(lines) > height {
		lines = lines[:height]
	}

	return strings.Join(lines, "\n")
}

// buildBanner renders the ORCAI BBS header banner as a single line.
func (m Model) buildBanner(w int) string {
	pal := m.ansiPalette()
	rst := aRst

	// Single line: logo + separator + subtitle
	logo := pal.Accent + aBld + " ░▒▓ ORCAI ▓▒░" + rst
	sep := pal.Border + "  │  " + rst
	sub := pal.FG + "ABBS Switchboard" + rst

	return logo + sep + sub
}

// buildLauncherSection renders the Pipeline Launcher box.
func (m Model) buildLauncherSection(w int) []string {
	pal := m.ansiPalette()
	borderColor := pal.Border
	if m.launcher.focused {
		borderColor = pal.Accent
	}

	var rows []string
	if sprite := PanelHeader(m.activeBundle(), "pipelines", w); sprite != nil {
		rows = append(rows, sprite...)
		if n := len(m.activeJobs); n > 0 {
			countLine := fmt.Sprintf("  %s%d running%s", pal.Accent, n, aRst)
			rows = append(rows, boxRow(countLine, w, borderColor))
		}
	} else {
		header := RenderHeader("pipelines")
		if n := len(m.activeJobs); n > 0 {
			header += fmt.Sprintf(" [%d running]", n)
		}
		rows = append(rows, boxTop(w, header, borderColor, pal.Accent))
	}

	if len(m.launcher.pipelines) == 0 {
		rows = append(rows, boxRow(pal.Dim+"  no pipelines saved"+aRst, w, borderColor))
	} else {
		for i, name := range m.launcher.pipelines {
			maxNameLen := max(w-4, 1)
			displayName := name
			if len(displayName) > maxNameLen {
				displayName = displayName[:maxNameLen-1] + "…"
			}
			if i == m.launcher.selected {
				cursor := pal.Dim
				if m.launcher.focused {
					cursor = pal.Accent
				}
				content := cursor + "> " + pal.FG + displayName + aRst
				rows = append(rows, boxRow(content, w, borderColor))
			} else {
				content := "  " + pal.Dim + displayName + aRst
				rows = append(rows, boxRow(content, w, borderColor))
			}
		}
	}

	rows = append(rows, boxBot(w, borderColor))
	return rows
}

// buildAgentSection renders a compact Agent Runner box showing the provider list.
// The full form (model selection + prompt) lives in the modal overlay.
func (m Model) buildAgentSection(w int) []string {
	pal := m.ansiPalette()
	borderColor := pal.Border
	if m.agent.focused {
		borderColor = pal.Accent
	}

	var rows []string
	if sprite := PanelHeader(m.activeBundle(), "agent_runner", w); sprite != nil {
		rows = append(rows, sprite...)
	} else {
		rows = append(rows, boxTop(w, RenderHeader("agent_runner"), borderColor, pal.Accent))
	}

	if len(m.agent.providers) == 0 {
		rows = append(rows, boxRow(pal.Dim+"  no providers available"+aRst, w, borderColor))
	} else {
		// Show provider list (scrollable).
		windowSize := agentInnerHeight
		offset := m.agent.agentScrollOffset
		if m.agent.selectedProvider < offset {
			offset = m.agent.selectedProvider
		} else if m.agent.selectedProvider >= offset+windowSize {
			offset = m.agent.selectedProvider - windowSize + 1
		}
		end := min(offset+windowSize, len(m.agent.providers))
		for i := offset; i < end; i++ {
			prov := m.agent.providers[i]
			label := prov.Label
			if label == "" {
				label = prov.ID
			}
			maxLen := max(w-5, 1)
			if len(label) > maxLen {
				label = label[:maxLen-1] + "…"
			}
			if i == m.agent.selectedProvider {
				cursor := pal.Dim
				if m.agent.focused {
					cursor = pal.Accent
				}
				content := cursor + "> " + pal.FG + label + aRst
				rows = append(rows, boxRow(content, w, borderColor))
			} else {
				content := "  " + pal.Dim + label + aRst
				rows = append(rows, boxRow(content, w, borderColor))
			}
		}
	}

	rows = append(rows, boxBot(w, borderColor))
	return rows
}

// totalFeedLines computes the total number of content lines for a feed (not counting borders).
func totalFeedLines(feed []feedEntry) int {
	n := 0
	for _, entry := range feed {
		n++ // title line
		n += len(entry.lines)
	}
	return n
}

// viewActivityFeed renders the center activity feed with scroll support.
func (m Model) viewActivityFeed(height, width int) string {
	pal := m.ansiPalette()
	feedSprite := PanelHeader(m.activeBundle(), "activity_feed", width)
	headerH := 1
	if feedSprite != nil {
		headerH = len(feedSprite)
	}
	visibleH := height - headerH - 1 // minus header and bottom border

	borderColor := pal.Border
	if m.feedFocused {
		borderColor = pal.Accent
	}

	// feedRowAt appends a content row, applying the cursor highlight if the
	// current line index matches feedCursor when the feed is focused.
	appendRow := func(lines *[]string, content string) {
		idx := len(*lines)
		if m.feedFocused && idx == m.feedCursor {
			*lines = append(*lines, boxRowCursorColor(content, width, borderColor))
		} else {
			*lines = append(*lines, boxRow(content, width, borderColor))
		}
	}

	// Flatten all feed entries into content lines.
	var allLines []string
	if len(m.feed) == 0 {
		appendRow(&allLines, pal.Dim+"  no activity yet"+aRst)
	} else {
		for _, entry := range m.feed {
			badge, badgeColor := statusBadge(entry.status, pal)
			ts := entry.ts.Format("15:04:05")
			titleLine := fmt.Sprintf("  %s%s%s %s%s%s  %s",
				badgeColor, badge, aRst,
				pal.Dim, ts, aRst,
				pal.Accent+entry.title+aRst)
			appendRow(&allLines, titleLine)

			// Render per-step status badges if this entry has steps.
			if len(entry.steps) > 0 {
				var badges []string
				for _, step := range entry.steps {
					var col string
					switch step.status {
					case "running":
						col = aYlw
					case "done":
						col = pal.Success
					case "failed":
						col = pal.Error
					default: // "pending" or unknown
						col = pal.Dim
					}
					badges = append(badges, col+"["+step.status+"] "+step.id+aRst)
				}
				badgeLine := "  " + strings.Join(badges, "  ")
				appendRow(&allLines, badgeLine)
			}

			// Cap output lines per entry: show the last feedLinesPerEntry lines only.
			const feedLinesPerEntry = 10
			entryLines := entry.lines
			skipped := 0
			if len(entryLines) > feedLinesPerEntry {
				skipped = len(entryLines) - feedLinesPerEntry
				entryLines = entryLines[skipped:]
			}
			if skipped > 0 {
				skipMsg := fmt.Sprintf("    … %d earlier lines (press f to scroll)", skipped)
				appendRow(&allLines, pal.Dim+skipMsg+aRst)
			}
			for _, outLine := range entryLines {
				// Strip ANSI codes — feed renders with its own dim style.
				outLine = stripANSI(outLine)
				maxLen := max(width-6, 1)
				if len(outLine) > maxLen {
					outLine = outLine[:maxLen-1] + "…"
				}
				appendRow(&allLines, pal.Dim+"    "+outLine+aRst)
			}
		}
	}

	// Clamp offset and slice visible window.
	offset := m.feedScrollOffset
	total := len(allLines)
	if visibleH <= 0 {
		visibleH = 1
	}
	maxOffset := max(0, total-visibleH)
	if offset > maxOffset {
		offset = maxOffset
	}
	if offset < 0 {
		offset = 0
	}
	end := offset + visibleH
	if end > total {
		end = total
	}
	visible := allLines[offset:end]

	// Compute scroll indicators.
	hasAbove := offset > 0
	hasBelow := end < total
	scrollSuffix := ""
	switch {
	case hasAbove && hasBelow:
		scrollSuffix = " ↕"
	case hasAbove:
		scrollSuffix = " ↑"
	case hasBelow:
		scrollSuffix = " ↓"
	}

	var lines []string
	if feedSprite != nil {
		lines = append(lines, feedSprite...)
	} else {
		lines = append(lines, boxTop(width, RenderHeader("activity_feed")+scrollSuffix, borderColor, pal.Accent))
	}
	lines = append(lines, visible...)

	// Pad to fill the box body.
	for len(lines) < height-1 {
		lines = append(lines, boxRow("", width, borderColor))
	}
	lines = append(lines, boxBot(width, borderColor))

	// Trim to exact height.
	if len(lines) > height {
		lines = lines[:height]
	}

	return strings.Join(lines, "\n")
}

// viewBottomBar renders the one-line keybinding hint strip.
func (m Model) viewBottomBar(width int) string {
	pal := m.ansiPalette()
	keyColor := pal.Accent
	descColor := pal.Dim
	sepColor := pal.Dim
	hint := func(key, desc string) string {
		return keyColor + key + descColor + " " + desc + aRst
	}
	sep := sepColor + " · " + aRst

	var parts []string
	switch {
	case m.signalBoardFocused && m.signalBoard.searching:
		parts = []string{
			hint("type", "search"),
			hint("↑↓", "nav"),
			hint("enter", "confirm"),
			hint("esc", "clear"),
		}
	case m.signalBoardFocused:
		parts = []string{
			hint("↑↓", "nav"),
			hint("/", "search"),
			hint("f", "filter"),
			hint("enter", "go to window"),
			hint("tab", "focus"),
			}
	case m.feedFocused:
		parts = []string{
			hint("↑↓", "nav"),
			hint("PgUp/PgDn", "page"),
			hint("g/G", "top/bottom"),
			hint("enter", "open"),
			hint("tab", "focus"),
			}
	case m.launcher.focused:
		parts = []string{
			hint("enter", "launch"),
			hint("n", "new"),
			hint("e", "edit"),
			hint("d", "delete"),
			hint("↑↓", "nav"),
			hint("tab", "focus"),
			}
	default:
		parts = []string{
			hint("enter", "launch"),
			hint("ctrl+s", "submit"),
			hint("tab", "focus"),
			hint("p", "pipelines"),
			hint("a", "agent"),
			hint("s", "signals"),
			hint("f", "feed"),
			}
	}

	bar := "  " + strings.Join(parts, sep)
	if lipgloss.Width(bar) < width {
		bar += strings.Repeat(" ", width-lipgloss.Width(bar))
	}
	return bar + aRst
}

// ── Box drawing helpers ────────────────────────────────────────────────────────

func boxTop(w int, title, borderColor, labelColor string) string {
	if title == "" {
		return borderColor + "┌" + strings.Repeat("─", max(w-2, 0)) + "┐" + aRst
	}
	label := " " + title + " "
	dashes := max(w-2-lipgloss.Width(label), 0)
	left := dashes / 2
	right := dashes - left
	return borderColor + "┌" + strings.Repeat("─", left) + labelColor + label + borderColor + strings.Repeat("─", right) + "┐" + aRst
}

func boxBot(w int, borderColor string) string {
	return borderColor + "└" + strings.Repeat("─", max(w-2, 0)) + "┘" + aRst
}

func boxRow(content string, w int, borderColor string) string {
	inner := w - 2
	pad := max(inner-lipgloss.Width(content), 0)
	return borderColor + "│" + aRst + content + strings.Repeat(" ", pad) + borderColor + "│" + aRst
}

// boxRowCursor renders a feed row with a "> " cursor marker prepended to the
// content, keeping the total visible width equal to a normal boxRow.
func boxRowCursor(content string, w int) string {
	return boxRowCursorColor(content, w, aBC)
}

// boxRowCursorColor is the color-aware version of boxRowCursor.
func boxRowCursorColor(content string, w int, borderColor string) string {
	cursorMark := aBrC + "> " + aRst
	// The cursor mark occupies 2 visible columns; reduce available content width
	// accordingly so the overall row width stays at w.
	inner := w - 2
	cursorMarkVis := 2
	contentWidth := lipgloss.Width(content)
	// Trim content if it would overflow.
	availForContent := inner - cursorMarkVis
	if availForContent < 0 {
		availForContent = 0
	}
	if contentWidth > availForContent {
		content = truncate.String(stripANSI(content), uint(availForContent))
		contentWidth = lipgloss.Width(content)
	}
	pad := max(availForContent-contentWidth, 0)
	return borderColor + "│" + aRst + cursorMark + content + strings.Repeat(" ", pad) + borderColor + "│" + aRst
}

func statusBadge(s FeedStatus, pal styles.ANSIPalette) (string, string) {
	switch s {
	case FeedRunning:
		return "▶ running", pal.Accent
	case FeedDone:
		return "✓ done", pal.Success
	case FeedFailed:
		return "✗ failed", pal.Error
	default:
		return "? unknown", pal.Dim
	}
}

// padToVis right-pads s with spaces until its visible length equals w.
func padToVis(s string, w int) string {
	vl := lipgloss.Width(s)
	if vl >= w {
		return s
	}
	return s + strings.Repeat(" ", w-vl)
}

// overlayCenter draws overlay centered over base. Each overlay line replaces
// the base line from startCol onward so the switchboard content shows around
// the floating box.
func overlayCenter(base, overlay string, w, h int) string {
	baseLines := strings.Split(base, "\n")
	overlayLines := strings.Split(overlay, "\n")

	popW := 0
	for _, l := range overlayLines {
		if vl := lipgloss.Width(l); vl > popW {
			popW = vl
		}
	}
	popH := len(overlayLines)
	startRow := max((h-popH)/2, 0)
	startCol := max((w-popW)/2, 0)

	// For each popup row: splice left base + popup + right base so all panels
	// remain visible on both sides of the floating box.
	for i, oLine := range overlayLines {
		row := startRow + i
		for len(baseLines) <= row {
			baseLines = append(baseLines, "")
		}
		left := ansiTrunc(baseLines[row], startCol)
		right := ansiFrom(baseLines[row], startCol+popW)
		baseLines[row] = left + oLine + right
	}
	return strings.Join(baseLines, "\n")
}

// ansiTrunc returns s truncated at visible column n, skipping SGR escapes.
// ansiTrunc truncates s at visible column n using muesli/reflow/truncate,
// which correctly handles ANSI SGR sequences and Unicode wide characters.
func ansiTrunc(s string, n int) string {
	if n <= 0 {
		return ""
	}
	return truncate.String(s, uint(n))
}

// ansiFrom returns the portion of s starting at visible column n.
// Uses muesli/ansi for escape-sequence detection and go-runewidth for
// accurate Unicode column widths. A reset is prepended so prior SGR state
// doesn't bleed into the returned segment.
func ansiFrom(s string, n int) string {
	if n <= 0 {
		return s
	}
	vis := 0
	i := 0
	inEsc := false
	for i < len(s) {
		r, size := utf8.DecodeRuneInString(s[i:])
		if r == ansi.Marker {
			inEsc = true
			i += size
			continue
		}
		if inEsc {
			if ansi.IsTerminator(r) {
				inEsc = false
			}
			i += size
			continue
		}
		if vis >= n {
			return aRst + s[i:]
		}
		vis += runewidth.RuneWidth(r)
		i += size
	}
	return ""
}

// hexToRGBFromStyles parses "#rrggbb" → uint8 r, g, b.
// This is a local helper; see styles.BundleANSI for the exported version.
func hexToRGBFromStyles(hex string) (uint8, uint8, uint8) {
	hex = strings.TrimPrefix(hex, "#")
	if len(hex) != 6 {
		return 189, 147, 249 // Dracula purple fallback
	}
	parse := func(s string) uint8 {
		v, _ := strconv.ParseUint(s, 16, 8)
		return uint8(v)
	}
	return parse(hex[0:2]), parse(hex[2:4]), parse(hex[4:6])
}

// ── Run ───────────────────────────────────────────────────────────────────────

// Run starts the Switchboard as a full-screen BubbleTea program.
func Run() {
	p := tea.NewProgram(New(), tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Printf("switchboard error: %v\n", err)
	}
}

// RunToggle opens the switchboard as a tmux popup.
func RunToggle() {
	bin := resolveSwitchboardBin()
	exec.Command("tmux", "display-popup", "-E", "-w", "100%", "-h", "100%", bin).Run() //nolint:errcheck
}

func resolveSwitchboardBin() string {
	if bin, err := exec.LookPath("orcai-sysop"); err == nil {
		return bin
	}
	self, _ := os.Executable()
	if resolved, err := filepath.EvalSymlinks(self); err == nil {
		self = resolved
	}
	return filepath.Join(filepath.Dir(self), "orcai-sysop")
}
