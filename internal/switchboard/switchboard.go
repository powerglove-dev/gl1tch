// Package switchboard implements the ORCAI Switchboard — a full-screen BubbleTea
// TUI that merges the sysop panel and welcome dashboard into a single control
// surface with a Pipeline Launcher, Agent Runner, and Activity Feed.
package switchboard

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/reflow/wrap"
	robfigcron "github.com/robfig/cron/v3"

	orcaicron "github.com/adam-stokes/orcai/internal/cron"
	"github.com/adam-stokes/orcai/internal/busd/topics"
	"github.com/adam-stokes/orcai/internal/inbox"
	"github.com/adam-stokes/orcai/internal/panelrender"
	"github.com/adam-stokes/orcai/internal/picker"
	"github.com/adam-stokes/orcai/internal/plugin"
	"github.com/adam-stokes/orcai/internal/store"
	"github.com/adam-stokes/orcai/internal/styles"
	"github.com/adam-stokes/orcai/internal/themes"
	"github.com/adam-stokes/orcai/internal/translations"
	"github.com/adam-stokes/orcai/internal/tuikit"
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

// MarkModeState represents the mark mode cycle for the activity feed.
type MarkModeState int

const (
	MarkModeOff    MarkModeState = iota // normal navigation
	MarkModeActive                      // j/k marks/unmarks lines while navigating
	MarkModePaused                      // j/k navigates without marking
)

// agentInnerHeight is the fixed number of body rows inside the AGENT RUNNER box.
const agentInnerHeight = 8

// Pipeline launch mode constants.
const (
	plModeNone      = 0
	plModeSelect    = 1
	plScheduleInput = 2
)

// maxParallelJobs is the maximum number of jobs that can run concurrently.
const maxParallelJobs = 8

// StepInfo tracks the status of a single pipeline step within a feed entry.
type StepInfo struct {
	id     string
	status string   // "pending", "running", "done", "failed"
	lines  []string // per-step output lines
}

type feedEntry struct {
	id         string
	title      string
	status     FeedStatus
	ts         time.Time
	lines      []string
	steps      []StepInfo
	cwd        string // working directory the job runs in, shown in feed and signal board
	tmuxWindow string // fully-qualified target "session:orcai-<feedID>", empty if no window
	logFile    string // /tmp/orcai-<feedID>.log
	doneFile   string // non-empty for window-mode jobs; written by the shell when the command exits
	archived   bool   // true when the user has dismissed this entry from the board
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
	id           string
	cancel       context.CancelFunc
	ch           chan tea.Msg
	tmuxWindow   string
	logFile      string // /tmp/orcai-<feedID>.log — tailed in the tmux window
	storeRunID   int64  // non-zero when run was recorded in the result store
	pipelineName string // non-empty for pipeline jobs; matched against busd RunStarted payload
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

// InboxPanel holds display and interaction state for the INBOX panel.
type InboxPanel struct {
	focused      bool
	selectedIdx  int
	scrollOffset int
	filterQuery  string
	filterActive bool
}

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
	feedMarked        map[int]bool   // marked absolute line indices
	feedMarkedContent map[int]string // ANSI-stripped content at each marked index
	feedMarkMode      MarkModeState
	signalBoard           SignalBoard
	signalBoardFocused    bool
	confirmDelete         bool
	pendingDeletePipeline string
	agentModalOpen  bool
	agentModalFocus int // 0=provider, 1=model, 2=prompt, 3=cwd, 4=schedule (within modal)
	agentSchedule         textarea.Model
	agentScheduleErr      string
	helpOpen              bool
	registry              *themes.Registry
	themeState            tuikit.ThemeState
	themePicker tuikit.ThemePicker
	// CWD / dir picker
	launchCWD           string         // CWD at orcai startup (immutable after New())
	agentCWD            string         // current agent session CWD (user-editable)
	dirPicker           DirPickerModel // reusable dir picker overlay
	dirPickerOpen       bool           // whether the dir picker overlay is visible
	dirPickerCtx        string         // "agent" or "pipeline"
	pendingPipelineName string         // pipeline waiting for CWD selection
	pendingPipelineYAML string         // YAML path for pendingPipelineName
	// Pipeline mode-select overlay
	pipelineLaunchMode   int          // plModeNone / plModeSelect / plScheduleInput
	pipelineModeSelected int          // 0=Run now, 1=Schedule recurring
	pipelineScheduleInput textarea.Model
	pipelineScheduleErr  string
	// Inbox panel
	inboxModel              inbox.Model
	inboxPanel              InboxPanel
	inboxReadIDs            map[int64]bool
	store                   *store.Store
	inboxDetailOpen         bool
	inboxMarkdownMode       bool
	inboxDetailIdx          int
	inboxDetailScroll       int
	inboxDetailCursor       int            // absolute line index within current run content
	inboxDetailMarked       map[int]bool   // marked absolute line indices
	inboxDetailMarkedLines  map[int]string // content at each marked line
	inboxMarkMode           MarkModeState
	// Cron panel
	cronPanel CronPanel
	// Pipeline bus subscription (tasks 7.2–7.8)
	pipelineBusCh chan pipelineBusEventMsg
}

// New creates a new Switchboard Model, discovering pipelines and providers.
func New() Model { return NewWithStore(nil) }

// NewWithStore creates a Switchboard Model with an attached result store.
// The store is passed to the Inbox panel for polling run results.
// Passing nil is valid; the Inbox will render an empty state.
func NewWithStore(s *store.Store) Model {
	ta := textarea.New()
	ta.Placeholder = "Enter prompt… (ctrl+s to submit)"
	ta.CharLimit = 4096
	ta.ShowLineNumbers = false
	ta.SetWidth(80)
	ta.SetHeight(4)

	schedTA := textarea.New()
	schedTA.Placeholder = "cron expr, blank = run now"
	schedTA.CharLimit = 128
	schedTA.ShowLineNumbers = false
	schedTA.SetWidth(80)
	schedTA.SetHeight(1)

	pipeSchedTA := textarea.New()
	pipeSchedTA.Placeholder = "cron expr (e.g. 0 * * * *)"
	pipeSchedTA.CharLimit = 128
	pipeSchedTA.ShowLineNumbers = false
	pipeSchedTA.SetWidth(60)
	pipeSchedTA.SetHeight(1)

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
		agentSchedule:         schedTA,
		pipelineScheduleInput: pipeSchedTA,
		signalBoard:           SignalBoard{activeFilter: "running"},
		activeJobs:            make(map[string]*jobHandle),
		launchCWD:             cwd,
		agentCWD:              cwd,
		inboxModel:            inbox.New(s, nil), // bundle set after registry loads
		store:                 s,
	}

	// Initialize theme registry from user themes dir.
	userThemesDir := filepath.Join(os.Getenv("HOME"), ".config", "orcai", "themes")
	if reg, err := themes.NewRegistry(userThemesDir); err == nil {
		m.registry = reg
		m.inboxModel.SetTheme(reg.Active())
		themes.SetGlobalRegistry(reg)
	}

	// Initialize translations provider from ~/.config/orcai/translations.yaml.
	translations.SetGlobalProvider(translations.NewYAMLProvider())

	m.inboxReadIDs = LoadReadSet(m.readStateFile())

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

// readStateFile returns the path to the inbox read-state persistence file.
func (m Model) readStateFile() string {
	return filepath.Join(os.Getenv("HOME"), ".config", "orcai", "inbox-read.json")
}

// Cursor returns the launcher cursor position — used in tests.
func (m Model) Cursor() int { return m.launcher.selected }

// AgentFormStep is kept for backward compatibility — always returns 0.
// The inline multi-step wizard has been replaced by the agent modal overlay.
func (m Model) AgentFormStep() int { return 0 }

// AgentModalOpen returns whether the agent modal overlay is open — used in tests.
func (m Model) AgentModalOpen() bool { return m.agentModalOpen }

// ThemePickerOpen returns whether the theme picker overlay is open — used in tests.
func (m Model) ThemePickerOpen() bool { return m.themePicker.Open }

// AgentModalFocus returns the current agent modal focus slot — used in tests.
func (m Model) AgentModalFocus() int { return m.agentModalFocus }

// AgentScheduleErr returns the agent schedule error string — used in tests.
func (m Model) AgentScheduleErr() string { return m.agentScheduleErr }

// PipelineLaunchMode returns the pipeline launch mode — used in tests.
func (m Model) PipelineLaunchMode() int { return m.pipelineLaunchMode }

// PipelineModeSelected returns the selected mode item in mode-select overlay — used in tests.
func (m Model) PipelineModeSelected() int { return m.pipelineModeSelected }

// PipelineScheduleErr returns the pipeline schedule error string — used in tests.
func (m Model) PipelineScheduleErr() string { return m.pipelineScheduleErr }

// PlModeNone returns the plModeNone constant — used in tests.
func PlModeNone() int { return plModeNone }

// PlModeSelect returns the plModeSelect constant — used in tests.
func PlModeSelect() int { return plModeSelect }

// PlScheduleInput returns the plScheduleInput constant — used in tests.
func PlScheduleInput() int { return plScheduleInput }

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

// ViewAgentModalBox is an exported wrapper for tests.
func (m Model) ViewAgentModalBox(w, h int) string { return m.viewAgentModalBox(w, h) }

// BuildCronSection is an exported wrapper for tests.
func (m Model) BuildCronSection(w int) []string { return m.buildCronSection(w, 10) }

// ViewActivityFeed renders the activity feed panel — used in tests.
func (m Model) ViewActivityFeed(h, w int) string { return m.viewActivityFeed(h, w) }

// CronPanelFocused returns whether the cron panel is focused — used in tests.
func (m Model) CronPanelFocused() bool { return m.cronPanel.focused }

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

// AddActiveJobWithCancel injects a job handle with a real cancel function —
// used in kill tests to verify the cancel was called.
func (m Model) AddActiveJobWithCancel(id string, cancel context.CancelFunc) Model {
	if m.activeJobs == nil {
		m.activeJobs = make(map[string]*jobHandle)
	}
	m.activeJobs[id] = &jobHandle{id: id, cancel: cancel}
	return m
}

// MaxParallelJobs returns the parallel job cap constant — used in tests.
func MaxParallelJobs() int { return maxParallelJobs }

// MakeTickMsg returns a tickMsg for use in tests.
func MakeTickMsg() tea.Msg { return tickMsg(time.Now()) }

// MakeJobDoneMsg returns a jobDoneMsg for use in tests.
func MakeJobDoneMsg(id string) tea.Msg { return jobDoneMsg{id: id} }

// FeedEntryStatus returns the FeedStatus for the entry with the given id,
// and whether it was found. Used in tests to inspect state without going
// through the rendered view.
func (m Model) FeedEntryStatus(id string) (FeedStatus, bool) {
	for _, e := range m.feed {
		if e.id == id {
			return e.status, true
		}
	}
	return 0, false
}

// FeedEntryArchived returns whether the entry with the given id is archived, and
// whether it was found. Used in tests.
func (m Model) FeedEntryArchived(id string) (bool, bool) {
	for _, e := range m.feed {
		if e.id == id {
			return e.archived, true
		}
	}
	return false, false
}

// SignalBoardActiveFilter returns the current active filter — used in tests.
func (m Model) SignalBoardActiveFilter() string { return m.signalBoard.activeFilter }

// FeedHasMarks returns whether any feed lines are marked — used in tests.
func (m Model) FeedHasMarks() bool { return len(m.feedMarked) > 0 }

// FeedMarkedAt returns whether the given absolute line index is marked — used in tests.
func (m Model) FeedMarkedAt(idx int) bool { return m.feedMarked[idx] }

// FeedMarkMode returns the current feed mark mode — used in tests.
func (m Model) FeedMarkMode() MarkModeState { return m.feedMarkMode }

// AgentPromptValue returns the current agent prompt textarea value — used in tests.
func (m Model) AgentPromptValue() string { return m.agent.prompt.Value() }

// SetSignalBoardFilter sets the active filter directly — used in tests.
func (m Model) SetSignalBoardFilter(f string) Model {
	m.signalBoard.activeFilter = f
	return m
}

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

// AddStepLines appends output lines to a step within a feed entry — used in tests.
func (m Model) AddStepLines(feedID, stepID string, lines []string) Model {
	return m.appendStepLines(feedID, stepID, lines)
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
			Warn:    "\x1b[33m",
			FG:      "\x1b[97m",
			BG:      "\x1b[40m",
			Border:  "\x1b[36m",
			SelBG:   "\x1b[44m",
		}
	}
	return styles.BundleANSI(b)
}

// looksLikeMarkdown returns true when s contains common markdown signals.
func looksLikeMarkdown(s string) bool {
	return strings.Contains(s, "# ") ||
		strings.Contains(s, "**") ||
		strings.Contains(s, "```")
}

// ── Init ──────────────────────────────────────────────────────────────────────

// Init starts the tick command and the inbox poll.
func (m Model) Init() tea.Cmd {
	// If cron.yaml already has entries, ensure the daemon is running so
	// existing schedules fire without requiring the user to reschedule.
	if entries, err := orcaicron.LoadConfig(); err == nil && len(entries) > 0 {
		go ensureCronDaemon()
	}
	return tea.Batch(
		tickCmd(),
		m.inboxModel.Init(),
		m.themeState.Init(),
		tryPipelineBusSubscribeCmd(),
		seedFeedFromStoreCmd(m.store),
	)
}

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

// writeSingleStepPipeline generates a minimal single-step pipeline YAML for a
// scheduled agent run and writes it to the .agents/ subdirectory of the pipelines
// directory so it does not appear in the PIPELINES launcher panel. Returns the
// absolute path of the written file so the caller can reference it in a cron entry.
func writeSingleStepPipeline(name, providerID, modelID, prompt string) (string, error) {
	dir := filepath.Join(pipelinesDir(), ".agents")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	path := filepath.Join(dir, name+".pipeline.yaml")

	// Indent every line of the prompt for the YAML block scalar.
	var promptLines strings.Builder
	for _, line := range strings.Split(prompt, "\n") {
		promptLines.WriteString("      ")
		promptLines.WriteString(line)
		promptLines.WriteString("\n")
	}

	model := ""
	if modelID != "" {
		model = "\n    model: " + modelID
	}

	content := fmt.Sprintf("name: %s\nversion: \"1\"\nsteps:\n  - id: run\n    executor: %s%s\n    prompt: |\n%s",
		name, providerID, model, promptLines.String())

	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		return "", err
	}
	return path, nil
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
	// ── Theme change from another process (e.g. cron TUI) ────────────────────
	if ts, cmd, ok := m.themeState.Handle(msg); ok {
		m.themeState = ts
		if m.registry != nil {
			m.registry.RefreshActive()
		}
		return m, cmd
	}

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
		m.inboxModel.SetSize(leftW, msg.Height)
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
		m.feedCursor = 0
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
						// 7.6: log-line parser is the fallback — skip if busd
						// already delivered a terminal (authoritative) status.
						if !isTerminalStepStatus(m.feed[i].steps[j].status) {
							m.feed[i].steps[j].status = msg.Status
						}
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

	// ── pipeline busd events (tasks 7.2–7.8) ─────────────────────────────

	case feedSeedMsg:
		m = m.handleFeedSeedMsg(msg)
		return m, nil

	case pipelineBusConnectMsg:
		m.pipelineBusCh = msg.ch
		return m, waitForPipelineBusEvent(m.pipelineBusCh)

	case pipelineBusDisconnectedMsg:
		m.pipelineBusCh = nil
		return m, nil

	case pipelineBusEventMsg:
		m = m.handlePipelineBusEvent(msg)
		if m.pipelineBusCh != nil {
			return m, waitForPipelineBusEvent(m.pipelineBusCh)
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
		// If any step recorded a failure, promote the entry to FeedFailed so
		// the signal board and feed badge reflect the true outcome even when
		// the pipeline process itself exited 0.
		finalStatus := FeedDone
		for _, e := range m.feed {
			if e.id == msg.id {
				for _, s := range e.steps {
					if s.status == "failed" {
						finalStatus = FeedFailed
						break
					}
				}
				break
			}
		}
		m = m.setFeedStatus(msg.id, finalStatus)
		if jh, ok := m.activeJobs[msg.id]; ok && jh.storeRunID != 0 && m.store != nil {
			exitCode := 0
			if finalStatus == FeedFailed {
				exitCode = 1
			}
			var out string
			for _, e := range m.feed {
				if e.id == msg.id {
					out = strings.Join(e.lines, "\n")
					break
				}
			}
			_ = m.store.RecordRunComplete(jh.storeRunID, exitCode, out, "")
		}
		var doneCmd tea.Cmd
		if jh, ok := m.activeJobs[msg.id]; ok {
			exitCode := 0
			if finalStatus == FeedFailed {
				exitCode = 1
			}
			doneCmd = publishAgentEventCmd(topics.AgentRunCompleted, map[string]any{
				"run_id":      msg.id,
				"store_run_id": jh.storeRunID,
				"exit_status": exitCode,
			})
		}
		delete(m.activeJobs, msg.id)
		return m, doneCmd

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
		if jh, ok := m.activeJobs[msg.id]; ok && jh.storeRunID != 0 && m.store != nil {
			var out string
			for _, e := range m.feed {
				if e.id == msg.id {
					out = strings.Join(e.lines, "\n")
					break
				}
			}
			if msg.err != nil {
				out += "\nerror: " + msg.err.Error()
			}
			_ = m.store.RecordRunComplete(jh.storeRunID, 1, out, "")
		}
		var failCmd tea.Cmd
		if jh, ok := m.activeJobs[msg.id]; ok {
			failCmd = publishAgentEventCmd(topics.AgentRunFailed, map[string]any{
				"run_id":       msg.id,
				"store_run_id": jh.storeRunID,
				"exit_status":  1,
			})
		}
		delete(m.activeJobs, msg.id)
		return m, failCmd

	case tea.KeyMsg:
		// These keys always go through handleKey regardless of which panel is focused.
		switch msg.String() {
		case "tab", "ctrl+c", "ctrl+q":
			return m.handleKey(msg)
		}
		// When any global overlay is active, all keys must go through handleKey
		// so ESC / y / n can dismiss it regardless of which panel is focused.
		if m.confirmQuit || m.helpOpen || m.agentModalOpen || m.themePicker.Open || m.dirPickerOpen || m.confirmDelete || m.pipelineLaunchMode != plModeNone {
			return m.handleKey(msg)
		}
		// Inbox captures all other keys when focused, but the detail overlay
		// takes priority and routes through handleKey so it can intercept keys.
		if m.inboxPanel.focused {
			if m.inboxDetailOpen {
				return m.handleKey(msg)
			}
			// Global overlay keys pass through even when inbox is focused.
			switch msg.String() {
			case "ctrl+h", "T":
				return m.handleKey(msg)
			}
			return m.handleKey(msg)
		}
		return m.handleKey(msg)

	case inbox.RunCompletedMsg:
		// Immediate inbox refresh when a run completes in-process.
		var inboxCmd tea.Cmd
		m.inboxModel, inboxCmd = m.inboxModel.Update(msg)
		return m, inboxCmd
	}

	// Forward all other messages to the inbox model (poll ticks, etc.).
	var inboxCmd tea.Cmd
	m.inboxModel, inboxCmd = m.inboxModel.Update(msg)

	return m, inboxCmd
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
	if m.themePicker.Open {
		return m.handleThemePicker(msg)
	}

	// Help modal.
	if m.helpOpen {
		switch key {
		case "esc", "ctrl+c", "ctrl+h", "q":
			m.helpOpen = false
		}
		return m, nil
	}

	// Inbox detail overlay — capture all keys when open.
	if m.inboxDetailOpen {
		runs := m.filteredInboxRuns()
		switch key {
		case "q", "esc":
			m.inboxDetailOpen = false
			m.inboxMarkMode = MarkModeOff
			m.inboxDetailMarked = nil
			m.inboxDetailMarkedLines = nil
		case "n":
			if len(runs) > 0 {
				m.inboxDetailIdx = (m.inboxDetailIdx + 1) % len(runs)
				m.inboxDetailScroll = 0
				m.inboxDetailCursor = 0
				m.inboxMarkMode = MarkModeOff
				m.inboxDetailMarked = nil
				m.inboxDetailMarkedLines = nil
			}
		case "p":
			if len(runs) > 0 {
				m.inboxDetailIdx = (m.inboxDetailIdx - 1 + len(runs)) % len(runs)
				m.inboxDetailScroll = 0
				m.inboxDetailCursor = 0
				m.inboxMarkMode = MarkModeOff
				m.inboxDetailMarked = nil
				m.inboxDetailMarkedLines = nil
			}
		case "j", "down":
			if m.inboxMarkMode == MarkModeActive {
				m = m.inboxDetailToggleMark()
			}
			m.inboxDetailCursor++
			m.inboxDetailScroll = m.inboxDetailCursor
		case "k", "up":
			if m.inboxMarkMode == MarkModeActive {
				m = m.inboxDetailToggleMark()
			}
			if m.inboxDetailCursor > 0 {
				m.inboxDetailCursor--
			}
			m.inboxDetailScroll = m.inboxDetailCursor
		case "pgup", "[":
			if m.inboxDetailScroll > 10 {
				m.inboxDetailScroll -= 10
				m.inboxDetailCursor = m.inboxDetailScroll
			} else {
				m.inboxDetailScroll = 0
				m.inboxDetailCursor = 0
			}
		case "pgdown", "]":
			m.inboxDetailScroll += 10
			m.inboxDetailCursor = m.inboxDetailScroll
		case "M":
			m.inboxMarkdownMode = !m.inboxMarkdownMode
			m.inboxDetailScroll = 0
			m.inboxDetailCursor = 0
			return m, nil
		case "m":
			// Cycle mark mode: off → active → paused → off (exits and clears marks).
			switch m.inboxMarkMode {
			case MarkModeOff:
				m.inboxMarkMode = MarkModeActive
			case MarkModeActive:
				m.inboxMarkMode = MarkModePaused
			case MarkModePaused:
				m.inboxMarkMode = MarkModeOff
				m.inboxDetailMarked = nil
				m.inboxDetailMarkedLines = nil
			}
			return m, nil
		case "A":
			// Mark all lines in the current run (available while in mark mode).
			if m.inboxMarkMode != MarkModeOff {
				if idx := m.inboxDetailIdx; idx >= 0 && idx < len(runs) {
					run := runs[idx]
					pal := m.ansiPalette()
					content := buildRunContent(run, pal, false, 80)
					lines := strings.Split(strings.TrimRight(content, "\n"), "\n")
					if m.inboxDetailMarked == nil {
						m.inboxDetailMarked = make(map[int]bool)
						m.inboxDetailMarkedLines = make(map[int]string)
					}
					for i, line := range lines {
						m.inboxDetailMarked[i] = true
						m.inboxDetailMarkedLines[i] = strings.TrimSpace(stripANSI(line))
					}
				}
			}
			return m, nil
		case "D":
			// Clear all marks (available while in mark mode).
			if m.inboxMarkMode != MarkModeOff {
				m.inboxDetailMarked = nil
				m.inboxDetailMarkedLines = nil
			}
			return m, nil
		case "r":
			// Dispatch marked lines to agent modal.
			if len(m.inboxDetailMarked) > 0 {
				keys := make([]int, 0, len(m.inboxDetailMarkedLines))
				for k := range m.inboxDetailMarkedLines {
					keys = append(keys, k)
				}
				sort.Ints(keys)
				var parts []string
				for _, k := range keys {
					if line := strings.TrimSpace(m.inboxDetailMarkedLines[k]); line != "" {
						parts = append(parts, line)
					}
				}
				m.inboxDetailMarked = nil
				m.inboxDetailMarkedLines = nil
				m.inboxDetailOpen = false
				m.agent.prompt.SetValue(strings.Join(parts, "\n"))
				m.agentModalOpen = true
				m.agentModalFocus = 2
				m.agent.prompt.Focus()
			}
			return m, nil
		default:
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
			exec.Command("tmux", "kill-session", "-t", "orcai-cron").Run() //nolint:errcheck
			exec.Command("tmux", "kill-session", "-t", "orcai").Run()      //nolint:errcheck
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

	// Pipeline mode-select / schedule-input overlay — captured before panel handlers.
	if m.pipelineLaunchMode != plModeNone {
		return m.handlePipelineLaunchOverlay(msg)
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

	// ── Cron panel ────────────────────────────────────────────────────────────
	if m.cronPanel.focused {
		entries := m.filteredCronEntries()
		// Search mode.
		if m.cronPanel.filterActive {
			switch key {
			case "esc":
				m.cronPanel.filterActive = false
				m.cronPanel.filterQuery = ""
				m.cronPanel.scrollOffset = 0
			case "backspace":
				if len(m.cronPanel.filterQuery) > 0 {
					_, size := utf8.DecodeLastRuneInString(m.cronPanel.filterQuery)
					m.cronPanel.filterQuery = m.cronPanel.filterQuery[:len(m.cronPanel.filterQuery)-size]
					m.cronPanel.scrollOffset = 0
					filtered := m.filteredCronEntries()
					if m.cronPanel.selectedIdx >= len(filtered) {
						m.cronPanel.selectedIdx = max(len(filtered)-1, 0)
					}
				}
			default:
				if len(msg.Runes) > 0 {
					m.cronPanel.filterQuery += string(msg.Runes)
					m.cronPanel.scrollOffset = 0
					if m.cronPanel.selectedIdx >= len(m.filteredCronEntries()) {
						m.cronPanel.selectedIdx = 0
					}
				}
			}
			return m, nil
		}
		switch key {
		case "/":
			m.cronPanel.filterActive = true
			return m, nil
		case "m":
			go ensureCronDaemon()
			exec.Command("tmux", "switch-client", "-t", "orcai-cron").Run() //nolint:errcheck
			return m, nil
		case "esc", "p":
			m.cronPanel.focused = false
			m.launcher.focused = true
			return m, nil
		case "j", "down":
			if m.cronPanel.selectedIdx < len(entries)-1 {
				m.cronPanel.selectedIdx++
				if m.cronPanel.selectedIdx >= m.cronPanel.scrollOffset+8 {
					m.cronPanel.scrollOffset = m.cronPanel.selectedIdx - 7
				}
			}
			return m, nil
		case "k", "up":
			if m.cronPanel.selectedIdx > 0 {
				m.cronPanel.selectedIdx--
				if m.cronPanel.selectedIdx < m.cronPanel.scrollOffset {
					m.cronPanel.scrollOffset = m.cronPanel.selectedIdx
				}
			}
			return m, nil
		}
	}

	// ── Inbox panel ───────────────────────────────────────────────────────────
	if m.inboxPanel.focused && !m.inboxDetailOpen {
		runs := m.filteredInboxRuns()
		// Search mode captures printable keys.
		if m.inboxPanel.filterActive {
			switch key {
			case "esc":
				m.inboxPanel.filterActive = false
				m.inboxPanel.filterQuery = ""
				m.inboxPanel.scrollOffset = 0
			case "backspace":
				if len(m.inboxPanel.filterQuery) > 0 {
					_, size := utf8.DecodeLastRuneInString(m.inboxPanel.filterQuery)
					m.inboxPanel.filterQuery = m.inboxPanel.filterQuery[:len(m.inboxPanel.filterQuery)-size]
					m.inboxPanel.scrollOffset = 0
					// Re-clamp selectedIdx to new filtered length.
					filtered := m.filteredInboxRuns()
					if m.inboxPanel.selectedIdx >= len(filtered) {
						m.inboxPanel.selectedIdx = max(len(filtered)-1, 0)
					}
				}
			default:
				if len(msg.Runes) > 0 {
					m.inboxPanel.filterQuery += string(msg.Runes)
					m.inboxPanel.scrollOffset = 0
					if m.inboxPanel.selectedIdx >= len(m.filteredInboxRuns()) {
						m.inboxPanel.selectedIdx = 0
					}
				}
			}
			return m, nil
		}
		switch key {
		case "/":
			m.inboxPanel.filterActive = true
			return m, nil
		case "j", "down":
			if m.inboxPanel.selectedIdx < len(runs)-1 {
				m.inboxPanel.selectedIdx++
				if m.inboxPanel.selectedIdx >= m.inboxPanel.scrollOffset+8 {
					m.inboxPanel.scrollOffset = m.inboxPanel.selectedIdx - 7
				}
			}
			return m, nil
		case "k", "up":
			if m.inboxPanel.selectedIdx > 0 {
				m.inboxPanel.selectedIdx--
				if m.inboxPanel.selectedIdx < m.inboxPanel.scrollOffset {
					m.inboxPanel.scrollOffset = m.inboxPanel.selectedIdx
				}
			}
			return m, nil
		case "enter":
			if len(runs) > 0 && m.inboxPanel.selectedIdx >= 0 && m.inboxPanel.selectedIdx < len(runs) {
				m.inboxDetailOpen = true
				m.inboxDetailIdx = m.inboxPanel.selectedIdx
				m.inboxDetailScroll = 0
				m.inboxMarkdownMode = false
			}
			return m, nil
		case "x":
			if len(runs) > 0 && m.inboxPanel.selectedIdx >= 0 && m.inboxPanel.selectedIdx < len(runs) {
				run := runs[m.inboxPanel.selectedIdx]
				m.inboxReadIDs[run.ID] = true
				_ = SaveReadSet(m.readStateFile(), m.inboxReadIDs)
				// Advance or clamp cursor.
				newRuns := m.filteredInboxRuns()
				if m.inboxPanel.selectedIdx >= len(newRuns) {
					m.inboxPanel.selectedIdx = max(len(newRuns)-1, 0)
				}
				if m.inboxPanel.scrollOffset > m.inboxPanel.selectedIdx {
					m.inboxPanel.scrollOffset = m.inboxPanel.selectedIdx
				}
			}
			return m, nil
		case "esc":
			m.inboxPanel.focused = false
			m.inboxModel.SetFocused(false)
			return m, nil
		}
	}

	// Global focus shortcuts — p focuses pipelines.
	switch key {
	case "p":
		m = m.clearFeedMarkMode()
		m.launcher.focused = true
		m.agent.focused = false
		m.feedFocused = false
		m.signalBoardFocused = false
		m.cronPanel.focused = false
		return m, nil
	case "c":
		m = m.clearFeedMarkMode()
		m.launcher.focused = false
		m.agent.focused = false
		m.feedFocused = false
		m.signalBoardFocused = false
		m.inboxPanel.focused = false
		m.inboxModel.SetFocused(false)
		m.cronPanel.focused = true
		return m, nil
	}

	switch key {
	case "ctrl+h":
		m.helpOpen = true
		return m, nil

	case "ctrl+c", "ctrl+q":
		m.confirmQuit = true
		return m, nil

	case "tab":
		if m.cronPanel.focused {
			// cron → launcher
			m.cronPanel.focused = false
			m.launcher.focused = true
		} else if m.feedFocused {
			// feed → cron
			m = m.clearFeedMarkMode()
			m.feedFocused = false
			m.cronPanel.focused = true
		} else if m.inboxPanel.focused {
			// inbox → feed
			m.inboxPanel.focused = false
			m.inboxModel.SetFocused(false)
			m.feedFocused = true
			m.feedCursor = 0
		} else if m.signalBoardFocused {
			// signalBoard → inbox
			m.signalBoardFocused = false
			m.signalBoard.clearSearch()
			m.inboxPanel.focused = true
			m.inboxModel.SetFocused(true)
		} else if m.agent.focused {
			// agent → signalBoard
			m.agent.focused = false
			m.signalBoardFocused = true
		} else if m.launcher.focused {
			// launcher → agent
			m.launcher.focused = false
			m.agent.focused = true
		} else {
			// nothing focused → return to top-left panel
			m.launcher.focused = true
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
			if m.feedFocused {
				m = m.clearFeedMarkMode()
			}
			m.feedFocused = !m.feedFocused
		}
		return m, nil

	case "a":
		m = m.clearFeedMarkMode()
		m.launcher.focused = false
		m.agent.focused = true
		m.feedFocused = false
		return m, nil

	case "s":
		m = m.clearFeedMarkMode()
		m.launcher.focused = false
		m.agent.focused = false
		m.feedFocused = false
		m.signalBoardFocused = true
		m.inboxPanel.focused = false
		m.inboxModel.SetFocused(false)
		return m, nil

	case "i":
		m = m.clearFeedMarkMode()
		m.launcher.focused = false
		m.agent.focused = false
		m.feedFocused = false
		m.signalBoardFocused = false
		m.inboxPanel.focused = true
		m.inboxModel.SetFocused(true)
		return m, nil

	case "r":
		// Feed: open agent runner modal with marked lines injected.
		if m.feedFocused && len(m.feedMarked) > 0 {
			keys := make([]int, 0, len(m.feedMarkedContent))
			for k := range m.feedMarkedContent {
				keys = append(keys, k)
			}
			sort.Ints(keys)
			var parts []string
			for _, k := range keys {
				if content := strings.TrimSpace(m.feedMarkedContent[k]); content != "" {
					parts = append(parts, content)
				}
			}
			m.feedMarked = nil
			m.feedMarkedContent = nil
			m.agent.prompt.SetValue(strings.Join(parts, "\n"))
			m.agentModalOpen = true
			m.agentModalFocus = 2
			m.agent.prompt.Focus()
			return m, nil
		}
		// Global refresh: reload pipelines and providers.
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
			m.themePicker.Open = true
			m.themePicker.OriginalTheme = m.registry.Active()
			// Set initial tab based on active theme's mode
			if active := m.registry.Active(); active != nil && active.Mode == "light" {
				m.themePicker.Tab = 1
			} else {
				m.themePicker.Tab = 0
			}
		}
		return m, nil

	case "pgdown", "]":
		if m.feedFocused {
			if m.feedMarkMode == MarkModeActive {
				m = m.feedToggleMark(m.feedCursor)
			}
			total := totalFeedLines(m.feed, m.feedPanelWidth())
			step := m.feedVisibleHeight()
			m.feedCursor = min(m.feedCursor+step, max(0, total-1))
			m.feedScrollOffset = m.feedCursor
			m.clampFeedScroll()
			return m, nil
		}
		if m.signalBoardFocused {
			filtered := fuzzyFeed(m.signalBoard.query, m.filteredFeed())
			step := m.signalBoardVisibleRows()
			m.signalBoard.selectedIdx = min(m.signalBoard.selectedIdx+step, max(0, len(filtered)-1))
			m.signalBoard.clampScroll(step)
			return m, nil
		}

	case "pgup", "[":
		if m.feedFocused {
			if m.feedMarkMode == MarkModeActive {
				m = m.feedToggleMark(m.feedCursor)
			}
			step := m.feedVisibleHeight()
			m.feedCursor = max(m.feedCursor-step, 0)
			m.feedScrollOffset = m.feedCursor
			m.clampFeedScroll()
			return m, nil
		}
		if m.signalBoardFocused {
			step := m.signalBoardVisibleRows()
			m.signalBoard.selectedIdx = max(m.signalBoard.selectedIdx-step, 0)
			m.signalBoard.clampScroll(step)
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
			total := totalFeedLines(m.feed, m.feedPanelWidth())
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

	case "x":
		if m.signalBoardFocused {
			filtered := fuzzyFeed(m.signalBoard.query, m.filteredFeed())
			if m.signalBoard.selectedIdx < len(filtered) {
				entry := filtered[m.signalBoard.selectedIdx]
				if entry.status == FeedRunning {
					if jh, ok := m.activeJobs[entry.id]; ok {
						jh.cancel()
						if jh.tmuxWindow != "" {
							exec.Command("tmux", "kill-window", "-t", jh.tmuxWindow).Run() //nolint:errcheck
						}
						delete(m.activeJobs, entry.id)
					}
					m = m.setFeedStatus(entry.id, FeedFailed)
				}
			}
			return m, nil
		}

	case "d":
		if m.signalBoardFocused {
			filtered := fuzzyFeed(m.signalBoard.query, m.filteredFeed())
			if m.signalBoard.selectedIdx < len(filtered) {
				entry := filtered[m.signalBoard.selectedIdx]
				for i := range m.feed {
					if m.feed[i].id == entry.id {
						m.feed[i].archived = true
						break
					}
				}
				// Clamp cursor so it doesn't point at the now-hidden row.
				newFiltered := fuzzyFeed(m.signalBoard.query, m.filteredFeed())
				if m.signalBoard.selectedIdx >= len(newFiltered) && m.signalBoard.selectedIdx > 0 {
					m.signalBoard.selectedIdx = len(newFiltered) - 1
				}
				if m.signalBoard.selectedIdx < 0 {
					m.signalBoard.selectedIdx = 0
				}
			}
			return m, nil
		}
		if m.launcher.focused && len(m.launcher.pipelines) > 0 {
			m.confirmDelete = true
			m.pendingDeletePipeline = m.launcher.pipelines[m.launcher.selected]
			return m, nil
		}

	case "m":
		if m.feedFocused {
			switch m.feedMarkMode {
			case MarkModeOff:
				m.feedMarkMode = MarkModeActive
			case MarkModeActive:
				m.feedMarkMode = MarkModePaused
			case MarkModePaused:
				// Third press exits mark mode entirely and clears marks.
				m = m.clearFeedMarkMode()
			}
			return m, nil
		}

	case "A":
		// Mark all feed lines (available while in mark mode).
		if m.feedFocused && m.feedMarkMode != MarkModeOff {
			rawLines := m.feedRawLines(m.feedPanelWidth())
			if m.feedMarked == nil {
				m.feedMarked = make(map[int]bool)
			}
			if m.feedMarkedContent == nil {
				m.feedMarkedContent = make(map[int]string)
			}
			for i, line := range rawLines {
				m.feedMarked[i] = true
				m.feedMarkedContent[i] = line
			}
			return m, nil
		}

	case "D":
		// Clear all feed marks (available while in mark mode).
		if m.feedFocused && m.feedMarkMode != MarkModeOff {
			m.feedMarked = nil
			m.feedMarkedContent = nil
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

// clearFeedMarkMode resets mark mode to off and clears all feed marks.
// Call whenever the feed loses focus.
func (m Model) clearFeedMarkMode() Model {
	m.feedMarkMode = MarkModeOff
	m.feedMarked = nil
	m.feedMarkedContent = nil
	return m
}

// feedToggleMark toggles the mark state of the given absolute line index.
func (m Model) feedToggleMark(idx int) Model {
	rawLines := m.feedRawLines(m.feedPanelWidth())
	if m.feedMarked == nil {
		m.feedMarked = make(map[int]bool)
	}
	if m.feedMarkedContent == nil {
		m.feedMarkedContent = make(map[int]string)
	}
	if m.feedMarked[idx] {
		delete(m.feedMarked, idx)
		delete(m.feedMarkedContent, idx)
	} else {
		m.feedMarked[idx] = true
		if idx < len(rawLines) {
			m.feedMarkedContent[idx] = rawLines[idx]
		}
	}
	return m
}

// inboxDetailToggleMark toggles the mark state of the inbox detail cursor line.
func (m Model) inboxDetailToggleMark() Model {
	runs := m.filteredInboxRuns()
	if m.inboxDetailMarked == nil {
		m.inboxDetailMarked = make(map[int]bool)
		m.inboxDetailMarkedLines = make(map[int]string)
	}
	idx := m.inboxDetailIdx
	if idx >= 0 && idx < len(runs) {
		run := runs[idx]
		pal := m.ansiPalette()
		content := buildRunContent(run, pal, false, 80)
		lines := strings.Split(strings.TrimRight(content, "\n"), "\n")
		cursor := m.inboxDetailCursor
		if cursor < len(lines) {
			if m.inboxDetailMarked[cursor] {
				delete(m.inboxDetailMarked, cursor)
				delete(m.inboxDetailMarkedLines, cursor)
			} else {
				m.inboxDetailMarked[cursor] = true
				m.inboxDetailMarkedLines[cursor] = strings.TrimSpace(stripANSI(lines[cursor]))
			}
		}
	}
	return m
}

func (m Model) handleDown() Model {
	if m.feedFocused {
		if m.feedMarkMode == MarkModeActive {
			m = m.feedToggleMark(m.feedCursor)
		}
		total := totalFeedLines(m.feed, m.feedPanelWidth())
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
		if m.feedMarkMode == MarkModeActive {
			m = m.feedToggleMark(m.feedCursor)
		}
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
		m.agentSchedule.Blur()
		m.agentScheduleErr = ""
		return m, nil

	case "ctrl+s":
		return m.submitAgentJob()

	case "tab":
		m.agentModalFocus = (m.agentModalFocus + 1) % 5
		switch m.agentModalFocus {
		case 2:
			m.agent.prompt.Focus()
			m.agentSchedule.Blur()
		case 4:
			m.agent.prompt.Blur()
			m.agentSchedule.Focus()
		default:
			m.agent.prompt.Blur()
			m.agentSchedule.Blur()
		}
		return m, nil

	case "shift+tab":
		m.agentModalFocus = (m.agentModalFocus + 4) % 5
		switch m.agentModalFocus {
		case 2:
			m.agent.prompt.Focus()
			m.agentSchedule.Blur()
		case 4:
			m.agent.prompt.Blur()
			m.agentSchedule.Focus()
		default:
			m.agent.prompt.Blur()
			m.agentSchedule.Blur()
		}
		return m, nil

	case "up", "k":
		if m.agentModalFocus == 2 || m.agentModalFocus == 4 {
			// Let textarea handle the key when prompt or schedule is focused.
			break
		}
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
		if m.agentModalFocus == 2 || m.agentModalFocus == 4 {
			// Let textarea handle the key when prompt or schedule is focused.
			break
		}
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
	}

	// CWD focus: enter opens the dir picker.
	if m.agentModalFocus == 3 {
		if msg.String() == "enter" {
			m.dirPicker = NewDirPickerModel()
			m.dirPickerOpen = true
			m.dirPickerCtx = "agent"
			return m, DirPickerInit()
		}
		return m, nil
	}

	// Forward key events to the focused textarea.
	if m.agentModalFocus == 2 {
		var cmd tea.Cmd
		m.agent.prompt, cmd = m.agent.prompt.Update(msg)
		return m, cmd
	}
	if m.agentModalFocus == 4 {
		var cmd tea.Cmd
		m.agentSchedule, cmd = m.agentSchedule.Update(msg)
		return m, cmd
	}
	return m, nil
}

// handlePipelineLaunchOverlay routes key events for the pipeline mode-select
// and schedule-input overlays.
func (m Model) handlePipelineLaunchOverlay(msg tea.KeyMsg) (Model, tea.Cmd) {
	key := msg.String()

	switch m.pipelineLaunchMode {
	case plModeSelect:
		switch key {
		case "esc", "ctrl+c":
			m.pipelineLaunchMode = plModeNone
			m.pendingPipelineName = ""
			m.pendingPipelineYAML = ""
			return m, nil
		case "up", "k":
			if m.pipelineModeSelected > 0 {
				m.pipelineModeSelected--
			}
			return m, nil
		case "down", "j":
			if m.pipelineModeSelected < 1 {
				m.pipelineModeSelected++
			}
			return m, nil
		case "enter":
			if m.pipelineModeSelected == 0 {
				// Run now — open dir picker (existing flow).
				m.pipelineLaunchMode = plModeNone
				m.dirPicker = NewDirPickerModel()
				m.dirPickerOpen = true
				m.dirPickerCtx = "pipeline"
				return m, DirPickerInit()
			}
			// Schedule recurring — transition to schedule input.
			m.pipelineLaunchMode = plScheduleInput
			m.pipelineScheduleErr = ""
			m.pipelineScheduleInput.SetValue("")
			m.pipelineScheduleInput.Focus()
			return m, nil
		}

	case plScheduleInput:
		switch key {
		case "esc", "ctrl+c":
			m.pipelineLaunchMode = plModeNone
			m.pendingPipelineName = ""
			m.pendingPipelineYAML = ""
			m.pipelineScheduleInput.Blur()
			m.pipelineScheduleErr = ""
			return m, nil
		case "ctrl+s", "enter":
			schedExpr := strings.TrimSpace(m.pipelineScheduleInput.Value())
			if schedExpr == "" {
				m.pipelineScheduleErr = "cron expression required"
				return m, nil
			}
			parser := robfigcron.NewParser(robfigcron.Minute | robfigcron.Hour | robfigcron.Dom | robfigcron.Month | robfigcron.Dow)
			if _, err := parser.Parse(schedExpr); err != nil {
				m.pipelineScheduleErr = "invalid cron: " + err.Error()
				return m, nil
			}
			m.pipelineScheduleErr = ""

			name := m.pendingPipelineName
			yamlPath := m.pendingPipelineYAML
			entryName := fmt.Sprintf("pipeline-%s", name)
			cronEntry := orcaicron.Entry{
				Name:     entryName,
				Schedule: schedExpr,
				Kind:     "pipeline",
				Target:   yamlPath,
			}
			if werr := orcaicron.WriteEntry(cronEntry); werr != nil {
				m.pipelineScheduleErr = "write error: " + werr.Error()
				return m, nil
			}
			// Auto-start cron daemon if not already running.
			go ensureCronDaemon()

			feedID := fmt.Sprintf("sched-%d", time.Now().UnixNano())
			confirmEntry := feedEntry{
				id:     feedID,
				title:  "scheduled: " + name + " @ " + schedExpr,
				status: FeedDone,
				ts:     time.Now(),
				lines:  []string{"cron entry written to cron.yaml"},
			}
			m.feed = append([]feedEntry{confirmEntry}, m.feed...)
			if len(m.feed) > 200 {
				m.feed = m.feed[:200]
			}

			m.pipelineLaunchMode = plModeNone
			m.pendingPipelineName = ""
			m.pendingPipelineYAML = ""
			m.pipelineScheduleInput.Blur()
			m.pipelineScheduleErr = ""
			return m, nil
		default:
			var cmd tea.Cmd
			m.pipelineScheduleInput, cmd = m.pipelineScheduleInput.Update(msg)
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

	// Launcher: show mode-select overlay before launching pipeline.
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

		m.pendingPipelineName = name
		m.pendingPipelineYAML = yamlPath
		m.pipelineLaunchMode = plModeSelect
		m.pipelineModeSelected = 0
		return m, nil
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

// runMetadataJSON returns a compact JSON blob for run metadata.
// Empty fields are omitted. Returns "" when all are empty.
func runMetadataJSON(pipelineFile, cwd, model string) string {
	if pipelineFile == "" && cwd == "" && model == "" {
		return ""
	}
	m := make(map[string]string, 3)
	if pipelineFile != "" {
		m["pipeline_file"] = pipelineFile
	}
	if cwd != "" {
		m["cwd"] = cwd
	}
	if model != "" {
		m["model"] = model
	}
	b, err := json.Marshal(m)
	if err != nil {
		return ""
	}
	return string(b)
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

	orcaiBin := orcaiBinaryPath()
	shellCmd := orcaiBin + " pipeline run " + yamlPath
	windowName, logFile, doneFile := createJobWindow(feedID, shellCmd, name, cwd)

	ch := make(chan tea.Msg, 256)
	_, cancel := context.WithCancel(context.Background())
	jh := &jobHandle{id: feedID, cancel: cancel, ch: ch, tmuxWindow: windowName, logFile: logFile, pipelineName: name}
	// Feed entry created by the busd RunStarted event — don't create an eager duplicate here.
	m.activeJobs[feedID] = jh

	startLogWatcher(feedID, logFile, doneFile, ch)
	return m, drainChan(ch)
}

// submitAgentJob submits the agent job from the modal overlay.
// If SCHEDULE is non-blank, it writes a cron entry instead of launching immediately.
func (m Model) submitAgentJob() (Model, tea.Cmd) {
	input := strings.TrimSpace(m.agent.prompt.Value())
	if input == "" {
		return m, nil
	}
	prov := m.currentProvider()
	if prov == nil {
		return m, nil
	}

	var modelID string
	models := nonSepModels(prov.Models)
	if len(models) > 0 && m.agent.selectedModel < len(models) {
		modelID = models[m.agent.selectedModel].ID
	}

	agentName := prov.ID
	if modelID != "" {
		agentName += "/" + modelID
	}

	// ── Schedule path ─────────────────────────────────────────────────────────
	schedExpr := strings.TrimSpace(m.agentSchedule.Value())
	if schedExpr != "" {
		parser := robfigcron.NewParser(robfigcron.Minute | robfigcron.Hour | robfigcron.Dom | robfigcron.Month | robfigcron.Dow)
		if _, err := parser.Parse(schedExpr); err != nil {
			m.agentScheduleErr = "invalid cron: " + err.Error()
			return m, nil
		}
		m.agentScheduleErr = ""

		entryName := fmt.Sprintf("agent-%s-%d", prov.ID, time.Now().UnixNano())

		// Generate a minimal single-step pipeline YAML so the scheduled run
		// has the prompt embedded and runs via the standard pipeline executor.
		pipelineFile, pipelineErr := writeSingleStepPipeline(entryName, prov.ID, modelID, input)
		if pipelineErr != nil {
			m.agentScheduleErr = "pipeline write error: " + pipelineErr.Error()
			return m, nil
		}

		cronEntry := orcaicron.Entry{
			Name:     entryName,
			Schedule: schedExpr,
			Kind:     "pipeline",
			Target:   pipelineFile,
		}
		if werr := orcaicron.WriteEntry(cronEntry); werr != nil {
			m.agentScheduleErr = "write error: " + werr.Error()
			return m, nil
		}
		// Auto-start cron daemon if not already running.
		go ensureCronDaemon()

		feedID := fmt.Sprintf("sched-%d", time.Now().UnixNano())
		confirmEntry := feedEntry{
			id:     feedID,
			title:  "scheduled: " + agentName + " @ " + schedExpr,
			status: FeedDone,
			ts:     time.Now(),
			lines:  []string{"cron entry written to cron.yaml"},
		}
		m.feed = append([]feedEntry{confirmEntry}, m.feed...)
		if len(m.feed) > 200 {
			m.feed = m.feed[:200]
		}

		// Close modal and reset.
		m.agentModalOpen = false
		m.agent.prompt.SetValue("")
		m.agent.prompt.Blur()
		m.agentSchedule.SetValue("")
		m.agentSchedule.Blur()
		m.agentScheduleErr = ""
		return m, nil
	}

	// ── Run-now path ──────────────────────────────────────────────────────────

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

	title := "agent: " + agentName

	feedID := fmt.Sprintf("agent-%d", time.Now().UnixNano())

	// Resolve CWD before creating the entry so it can be displayed immediately.
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

	entry := feedEntry{
		id:     feedID,
		title:  title,
		status: FeedRunning,
		ts:     time.Now(),
		cwd:    cwd,
	}
	m.feed = append([]feedEntry{entry}, m.feed...)
	if len(m.feed) > 200 {
		m.feed = m.feed[:200]
	}
	m.feedSelected = 0
	m.feedScrollOffset = 0
	m.feedCursor = 0

	windowName, logFile, _ := createJobWindow(feedID, "", title, cwd)
	entry.tmuxWindow = windowName
	entry.logFile = logFile
	m.feed[0] = entry

	binary := prov.Command
	if binary == "" {
		binary = prov.ID
	}
	adapter := plugin.NewCliAdapter(prov.ID, prov.Label+" CLI adapter", binary, prov.PipelineArgs...)
	vars := map[string]string{}
	if modelID != "" {
		vars["model"] = modelID
	}
	if cwd != "" {
		vars["cwd"] = cwd
	}

	ch := make(chan tea.Msg, 256)
	_, cancel := context.WithCancel(context.Background())
	jh := &jobHandle{id: feedID, cancel: cancel, ch: ch, tmuxWindow: windowName, logFile: logFile}
	if m.store != nil {
		if runID, err := m.store.RecordRunStart("agent", title, runMetadataJSON("", cwd, modelID)); err == nil {
			jh.storeRunID = runID
		}
	}
	m.activeJobs[feedID] = jh

	startPayload := map[string]any{"run_id": feedID, "agent": agentName}
	if jh.storeRunID != 0 {
		startPayload["store_run_id"] = jh.storeRunID
	}
	publishStart := publishAgentEventCmd(topics.AgentRunStarted, startPayload)

	cmd := runAgentCmdCh(adapter, input, vars, feedID, ch, cancel)
	drain := drainChan(ch)

	// Close modal and reset prompt after submission.
	m.agentModalOpen = false
	m.agent.prompt.SetValue("")
	m.agent.prompt.Blur()
	m.agentSchedule.SetValue("")
	m.agentSchedule.Blur()
	m.agentScheduleErr = ""

	return m, tea.Batch(publishStart, cmd, drain)
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
	total := totalFeedLines(m.feed, m.feedPanelWidth())
	maxOffset := max(0, total-visibleH)
	if m.feedScrollOffset > maxOffset {
		m.feedScrollOffset = maxOffset
	}
	if m.feedScrollOffset < 0 {
		m.feedScrollOffset = 0
	}
}

// filteredFeed returns feed entries matching the current signal board filter.
// Archived entries are only shown when filter == "archived"; they are excluded
// from all other views including "all".
func (m Model) filteredFeed() []feedEntry {
	filter := m.signalBoard.activeFilter
	if filter == "archived" {
		var out []feedEntry
		for _, e := range m.feed {
			if e.archived {
				out = append(out, e)
			}
		}
		return out
	}
	var out []feedEntry
	for _, e := range m.feed {
		if e.archived {
			continue
		}
		switch filter {
		case "all", "":
			out = append(out, e)
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

	leftW := m.leftColWidth() - 1
	feedW := max(w-leftW-2, 20)
	contentH := max(h-1, 5) // reserve one line for top bar; hint bars live inside each panel

	// Signal board: grows with entries up to 40% of screen height.
	// Minimum 5 rows so the box is always visible (header+border).
	maxSB := max(contentH*40/100, 10)
	sbHeight := min(len(m.feed)+9, maxSB)
	if sbHeight < 8 {
		sbHeight = 8
	}
	// Clamp sbHeight so feedH is at least 3.
	if sbHeight > contentH-3 {
		sbHeight = max(contentH-3, 8)
	}
	feedH := max(contentH-sbHeight, 3)

	left := m.viewLeftColumn(contentH, leftW)
	sb := m.buildSignalBoard(sbHeight, feedW)
	feed := m.viewActivityFeed(feedH, feedW)

	// Right column: signal board then feed lines, clipped to contentH.
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
		rows = append(rows, padToVis(l, leftW)+"  "+f)
	}

	body := strings.Join(rows, "\n")
	topBar := m.viewTopBar(w)

	// Dir picker overlay — highest priority, shown on top of everything.
	if m.dirPickerOpen {
		base := topBar + "\n" + body
		return overlayCenter(base, m.dirPicker.viewDirPickerBox(w, m.ansiPalette()), w, h)
	}

	if m.helpOpen {
		base := topBar + "\n" + body
		return overlayCenter(base, m.viewHelpModal(w, h), w, h)
	}

	if m.confirmQuit {
		base := topBar + "\n" + body
		return overlayCenter(base, m.viewQuitModalBox(w), w, h)
	}

	// Pipeline mode-select / schedule-input overlay.
	if m.pipelineLaunchMode == plModeSelect {
		base := topBar + "\n" + body
		return overlayCenter(base, m.viewPipelineModeSelect(w), w, h)
	}
	if m.pipelineLaunchMode == plScheduleInput {
		base := topBar + "\n" + body
		return overlayCenter(base, m.viewPipelineScheduleInput(w), w, h)
	}

	// Agent modal — floating overlay on top of the switchboard.
	if m.agentModalOpen {
		base := topBar + "\n" + body
		return overlayCenter(base, m.viewAgentModalBox(w, h), w, h)
	}

	// Delete confirmation — floating overlay on top of the switchboard.
	if m.confirmDelete {
		base := topBar + "\n" + body
		return overlayCenter(base, m.viewDeleteModalBox(w), w, h)
	}

	// Theme picker overlay.
	if m.themePicker.Open && m.registry != nil {
		base := topBar + "\n" + body
		content := viewThemePicker(m)
		return overlayCenter(base, content, w, h)
	}

	// Inbox detail overlay — full-screen ANSI box panel for a run result.
	if m.inboxDetailOpen && len(m.inboxModel.Runs()) > 0 {
		return topBar + "\n" + m.viewInboxDetail(w, h, m.inboxMarkdownMode)
	}

	return topBar + "\n" + body
}

// viewQuitModalBox renders the quit confirmation popup box.
func (m Model) viewQuitModalBox(w int) string {
	pal := m.ansiPalette()
	jobs := len(m.activeJobs)

	message := "Quit ORCAI?"
	if jobs > 0 {
		jobWord := "job"
		if jobs != 1 {
			jobWord = "jobs"
		}
		message = pal.Error + fmt.Sprintf("%d %s still running.", jobs, jobWord) + panelrender.RST
	}

	title := translations.Safe(translations.GlobalProvider(), translations.KeyQuitModalTitle, "ORCAI  Quit?")
	return panelrender.QuitConfirmBox(pal, title, message, w)
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

	boxW := max(min(lipgloss.Width(displayPath)+6, 68), 48)
	if boxW+4 > w {
		boxW = max(w-4, 20)
	}

	pal := m.ansiPalette()

	hints := panelrender.HintBar([]panelrender.Hint{
		{Key: "y", Desc: "confirm delete"},
		{Key: "n / esc", Desc: "cancel"},
	}, boxW-2, pal)

	rows := []string{
		boxTop(boxW, "delete pipeline", pal.Border, pal.Accent),
		boxRow("", boxW, pal.Border),
		boxRow(pal.Accent+panelrender.BLD+"  "+name+aRst, boxW, pal.Border),
		boxRow(pal.Dim+"  "+displayPath+aRst, boxW, pal.Border),
		boxRow("", boxW, pal.Border),
		boxRow(hints, boxW, pal.Border),
		boxBot(boxW, pal.Border),
	}

	return strings.Join(rows, "\n")
}

// viewPipelineModeSelect renders the mode-select overlay for pipeline launch.
func (m Model) viewPipelineModeSelect(w int) string {
	boxW := max(min(w-8, 52), 34)
	pal := m.ansiPalette()

	title := "pipeline  " + m.pendingPipelineName
	if lipgloss.Width(title) > boxW-4 {
		title = title[:boxW-5] + "…"
	}

	options := []string{"Run now", "Schedule recurring"}
	var rows []string
	rows = append(rows, boxTop(boxW, title, pal.Border, pal.Accent))
	rows = append(rows, boxRow("", boxW, pal.Border))
	for i, opt := range options {
		if i == m.pipelineModeSelected {
			rows = append(rows, boxRow(pal.SelBG+pal.Accent+"  > "+opt+aRst, boxW, pal.Border))
		} else {
			rows = append(rows, boxRow(pal.Dim+"    "+opt+aRst, boxW, pal.Border))
		}
	}
	rows = append(rows, boxRow("", boxW, pal.Border))
	hints := panelrender.HintBar([]panelrender.Hint{
		{Key: "j/k", Desc: "select"},
		{Key: "enter", Desc: "confirm"},
		{Key: "esc", Desc: "cancel"},
	}, boxW-2, pal)
	rows = append(rows, boxRow(hints, boxW, pal.Border))
	rows = append(rows, boxBot(boxW, pal.Border))
	return strings.Join(rows, "\n")
}

// viewPipelineScheduleInput renders the schedule-input overlay for pipeline scheduling.
func (m Model) viewPipelineScheduleInput(w int) string {
	boxW := max(min(w-8, 60), 38)
	pal := m.ansiPalette()

	title := "schedule  " + m.pendingPipelineName
	if lipgloss.Width(title) > boxW-4 {
		title = title[:boxW-5] + "…"
	}

	var rows []string
	rows = append(rows, boxTop(boxW, title, pal.Border, pal.Accent))
	rows = append(rows, boxRow("", boxW, pal.Border))

	schedInnerW := max(boxW-6, 10)
	m.pipelineScheduleInput.SetWidth(schedInnerW)
	for _, sLine := range strings.Split(m.pipelineScheduleInput.View(), "\n") {
		sLine = strings.TrimRight(sLine, "\r")
		rows = append(rows, boxRow("  "+sLine, boxW, pal.Border))
	}
	rows = append(rows, boxRow("", boxW, pal.Border))
	if m.pipelineScheduleErr != "" {
		rows = append(rows, boxRow(pal.Error+"  "+m.pipelineScheduleErr+aRst, boxW, pal.Border))
	} else {
		rows = append(rows, boxRow(pal.Dim+"  0 * * * *    every hour"+aRst, boxW, pal.Border))
		rows = append(rows, boxRow(pal.Dim+"  0 9 * * *    daily at 9am"+aRst, boxW, pal.Border))
		rows = append(rows, boxRow(pal.Dim+"  0 9 * * 1-5  weekdays at 9am"+aRst, boxW, pal.Border))
	}
	rows = append(rows, boxRow("", boxW, pal.Border))
	hints := panelrender.HintBar([]panelrender.Hint{
		{Key: "enter", Desc: "confirm"},
		{Key: "esc", Desc: "cancel"},
	}, boxW-2, pal)
	rows = append(rows, boxRow(hints, boxW, pal.Border))
	rows = append(rows, boxBot(boxW, pal.Border))
	return strings.Join(rows, "\n")
}

// viewAgentModal renders the full-screen agent overlay.
// viewAgentModalBox renders the agent modal box content only. overlayCenter
// places it over the base view.
func (m Model) viewAgentModalBox(w, h int) string {
	modalW := max(w*9/10, 60)
	if modalW > w-2 {
		modalW = w - 2
	}
	if modalW < 0 {
		modalW = w
	}

	// Target modal height: 90% of terminal, minimum 24 rows.
	// Fixed overhead rows (all sections except prompt textarea): ~20 rows.
	const fixedOverhead = 20
	const minPromptH = 4
	modalH := max(h*9/10, 24)
	promptH := max(modalH-fixedOverhead, minPromptH)

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
	m.agent.prompt.SetHeight(promptH)
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
	if cwdDisplay == "" {
		cwdDisplay = "(current directory)"
	}
	cwdColor := pal.Dim
	if m.agentModalFocus == 3 {
		cwdColor = pal.Accent
	}
	rows = append(rows, boxRow("  "+cwdColor+cwdDisplay+aRst, modalW, modalBorderColor))
	if m.agentModalFocus == 3 {
		rows = append(rows, boxRow(aDim+"  press enter to browse"+aRst, modalW, modalBorderColor))
	}

	// ── SCHEDULE section ──────────────────────────────────────────────────────
	rows = append(rows, boxRow("", modalW, modalBorderColor))
	schedHeader := "  " + sectionLabel("SCHEDULE (cron expr, blank = run now)", m.agentModalFocus == 4)
	rows = append(rows, boxRow(schedHeader, modalW, modalBorderColor))
	schedInnerW := max(modalW-6, 10)
	m.agentSchedule.SetWidth(schedInnerW)
	for _, sLine := range strings.Split(m.agentSchedule.View(), "\n") {
		sLine = strings.TrimRight(sLine, "\r")
		padded := "  " + sLine
		rows = append(rows, boxRow(padded, modalW, modalBorderColor))
	}
	if m.agentScheduleErr != "" {
		errLine := "  " + pal.Error + m.agentScheduleErr + aRst
		rows = append(rows, boxRow(errLine, modalW, modalBorderColor))
	} else {
		rows = append(rows, boxRow(aDim+"  0 * * * *    every hour"+aRst, modalW, modalBorderColor))
		rows = append(rows, boxRow(aDim+"  0 9 * * *    daily at 9am"+aRst, modalW, modalBorderColor))
		rows = append(rows, boxRow(aDim+"  0 9 * * 1-5  weekdays at 9am"+aRst, modalW, modalBorderColor))
	}

	// ── Hint bar ──────────────────────────────────────────────────────────────
	rows = append(rows, boxRow("", modalW, modalBorderColor))
	hintParts := []string{
		hint("tab", "focus"),
		hint("ctrl+s", "submit"),
		hint("esc", "close"),
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

// viewLeftColumn renders the left column: banner + launcher + agent + inbox sections.
func (m Model) viewLeftColumn(height, width int) string {
	var lines []string

	launcherLines := m.buildLauncherSection(width)
	lines = append(lines, launcherLines...)
	lines = append(lines, "")

	agentLines := m.buildAgentSection(width)
	lines = append(lines, agentLines...)

	// Distribute remaining rows between Inbox (60%) and Cron (40%), min 4 each.
	remaining := height - len(lines)
	if remaining >= 4 {
		lines = append(lines, "")
		remaining--

		inboxRows := remaining * 6 / 10
		cronRows := remaining - inboxRows - 1 // 1 for blank separator
		if inboxRows < 4 {
			inboxRows = 4
		}
		if cronRows < 4 {
			// Can't fit both — give everything to inbox.
			inboxRows = remaining
			cronRows = 0
		}

		lines = append(lines, m.buildInboxSection(width, inboxRows)...)
		if cronRows >= 4 {
			lines = append(lines, "")
			lines = append(lines, m.buildCronSection(width, cronRows)...)
		}
	}

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

// viewTopBar renders a full-terminal-width header bar with the ORCAI title
// centered. Uses theme accent as background (matching panel headers) and the
// theme BG color for text. Respects the translations.KeySwitchboardHeader key.
func (m Model) viewTopBar(w int) string {
	if w <= 0 {
		w = 120
	}

	title := "░▒▓ ORCAI — ABBS Switchboard ▓▒░"
	if p := translations.GlobalProvider(); p != nil {
		title = p.T(translations.KeySwitchboardHeader, title)
	}

	// Accent as background, BG color as text — same as panel header title rows.
	bgColor := "#bd93f9" // Dracula purple fallback
	fgColor := "#282a36" // Dracula bg fallback

	if b := m.activeBundle(); b != nil {
		if b.Palette.Accent != "" {
			bgColor = b.Palette.Accent
		}
		if b.Palette.BG != "" {
			fgColor = b.Palette.BG
		}
	}

	titleStyle := lipgloss.NewStyle().
		Width(w).
		Align(lipgloss.Center).
		Background(lipgloss.Color(bgColor)).
		Foreground(lipgloss.Color(fgColor)).
		Bold(true)

	return titleStyle.Render(title)
}

// buildLauncherSection renders the Pipeline Launcher box.
func (m Model) buildLauncherSection(w int) []string {
	pal := m.ansiPalette()
	borderColor := pal.Border
	titleColor := pal.Accent
	if m.launcher.focused {
		borderColor = pal.Accent
	}

	var rows []string
	if sprite := PanelHeader(m.activeBundle(), "pipelines", w, borderColor, titleColor); sprite != nil {
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
		rows = append(rows, boxTop(w, header, borderColor, titleColor))
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

	// Hint footer row — always present; shows hints when focused, blank when not.
	var launcherHints []panelrender.Hint
	if m.launcher.focused {
		launcherHints = []panelrender.Hint{
			{Key: "enter", Desc: "launch"},
			{Key: "n", Desc: "new"},
			{Key: "e", Desc: "edit"},
			{Key: "d", Desc: "delete"},
			{Key: "j/k", Desc: "nav"},
		}
	}
	rows = append(rows, boxRow(panelrender.HintBar(launcherHints, w-2, pal), w, borderColor))
	rows = append(rows, boxBot(w, borderColor))
	return rows
}

// buildAgentSection renders a compact Agent Runner box showing the provider list.
// The full form (model selection + prompt) lives in the modal overlay.
func (m Model) buildAgentSection(w int) []string {
	pal := m.ansiPalette()
	borderColor := pal.Border
	titleColor := pal.Accent
	if m.agent.focused {
		borderColor = pal.Accent
	}

	var rows []string
	if sprite := PanelHeader(m.activeBundle(), "agent_runner", w, borderColor, titleColor); sprite != nil {
		rows = append(rows, sprite...)
	} else {
		rows = append(rows, boxTop(w, RenderHeader("agent_runner"), borderColor, titleColor))
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

	// Hint footer row — always present; shows hints when focused, blank when not.
	var agentHints []panelrender.Hint
	if m.agent.focused {
		agentHints = []panelrender.Hint{
			{Key: "enter", Desc: "launch"},
			{Key: "j/k", Desc: "nav"},
		}
	}
	rows = append(rows, boxRow(panelrender.HintBar(agentHints, w-2, pal), w, borderColor))
	rows = append(rows, boxBot(w, borderColor))
	return rows
}

// filteredInboxRuns returns inbox runs filtered by read state and search query.
func (m Model) filteredInboxRuns() []store.Run {
	all := m.inboxModel.Runs()
	query := strings.ToLower(m.inboxPanel.filterQuery)
	var out []store.Run
	for _, r := range all {
		if m.inboxReadIDs[r.ID] {
			continue
		}
		if query != "" && !strings.Contains(strings.ToLower(r.Name), query) {
			continue
		}
		out = append(out, r)
	}
	return out
}

// buildInboxSection renders the Inbox panel using the same ANSI box style as
// the Pipelines and Agent Runner panels. height is the maximum number of rows
// the section may occupy (including header and bottom border).
func (m Model) buildInboxSection(w, height int) []string {
	pal := m.ansiPalette()
	borderColor := pal.Border
	titleColor := pal.Accent
	if m.inboxPanel.focused {
		borderColor = pal.Accent
	}

	var rows []string
	if sprite := PanelHeader(m.activeBundle(), "inbox", w, borderColor, titleColor); sprite != nil {
		rows = append(rows, sprite...)
	} else {
		rows = append(rows, boxTop(w, RenderHeader("inbox"), borderColor, titleColor))
	}

	runs := m.filteredInboxRuns()
	// maxRows is remaining content rows: total height minus header lines minus 1 for boxBot
	// minus 1 for the always-present hint footer row.
	maxRows := height - len(rows) - 2
	if maxRows < 0 {
		maxRows = 0
	}

	// Search prompt row.
	if m.inboxPanel.filterActive {
		cursor := "█"
		prompt := fmt.Sprintf("  %s/%s %s%s%s%s", pal.Accent, aRst, pal.FG, m.inboxPanel.filterQuery, cursor, aRst)
		rows = append(rows, boxRow(prompt, w, borderColor))
		maxRows--
		if maxRows < 0 {
			maxRows = 0
		}
	}

	if len(runs) == 0 {
		rows = append(rows, boxRow(pal.Dim+"  (empty)"+aRst, w, borderColor))
	} else {
		sel := m.inboxPanel.selectedIdx
		offset := m.inboxPanel.scrollOffset
		shown := 0
		for i := offset; i < len(runs) && shown < maxRows; i++ {
			run := runs[i]
			var dot string
			switch {
			case run.ExitStatus == nil:
				dot = pal.Accent + "◉" + aRst
			case *run.ExitStatus == 0:
				dot = aGrn + "●" + aRst
			default:
				dot = aRed + "●" + aRst
			}
			ts := time.UnixMilli(run.StartedAt).Format("1/2 3:04 PM")
			tsVis := len(ts) + 1
			maxNameLen := max(w-7-tsVis, 1)
			name := run.Name
			if len(name) > maxNameLen {
				name = name[:maxNameLen-1] + "…"
			}
			inner := w - 2
			focused := m.inboxPanel.focused
			var prefixVis int
			if i == sel && focused {
				prefixVis = 2 + 1 + 1 + len(name)
			} else {
				prefixVis = 2 + 1 + 1 + len(name)
			}
			pad := max(inner-prefixVis-tsVis, 0)
			dimTS := pal.Dim + strings.Repeat(" ", pad) + ts + aRst
			var content string
			if i == sel && focused {
				content = pal.Accent + "> " + aRst + dot + " " + pal.FG + name + aRst + dimTS
			} else {
				content = "  " + dot + " " + pal.Dim + name + aRst + dimTS
			}
			rows = append(rows, boxRow(content, w, borderColor))
			shown++
		}
	}

	// Pad to fill allocated height, leaving room for the always-present hint footer row.
	for len(rows) < height-2 {
		rows = append(rows, boxRow("", w, borderColor))
	}
	// Hint footer row — always present; shows hints when focused, blank when not.
	var inboxHints []panelrender.Hint
	if m.inboxPanel.focused {
		if m.inboxPanel.filterActive {
			inboxHints = []panelrender.Hint{
				{Key: "esc", Desc: "cancel"},
				{Key: "backspace", Desc: "delete"},
			}
		} else {
			inboxHints = []panelrender.Hint{
				{Key: "enter", Desc: "open"},
				{Key: "x", Desc: "mark read"},
				{Key: "/", Desc: "search"},
			}
		}
	}
	rows = append(rows, boxRow(panelrender.HintBar(inboxHints, w-2, pal), w, borderColor))
	rows = append(rows, boxBot(w, borderColor))
	if len(rows) > height {
		rows = rows[:height]
	}
	return rows
}

// feedPanelWidth returns the actual rendered width of the activity feed panel,
// matching the feedW calculation in View(). Used for consistent line-count math.
func (m Model) feedPanelWidth() int {
	w := m.width
	if w <= 0 {
		w = 120
	}
	return max(w-m.leftColWidth()-2, 20)
}

// totalFeedLines computes the total number of content lines the renderer will
// actually produce for a feed, including lines created by word-wrapping long
// output. It mirrors the line-building logic in viewActivityFeed so that
// navigation bounds match the visible output exactly.
func totalFeedLines(feed []feedEntry, width int) int {
	if len(feed) == 0 {
		return 1 // "no activity yet"
	}
	const feedLinesPerEntry = 10
	const maxStepOutputLines = 5
	stepMaxLen := max(width-10, 1)
	entryMaxLen := max(width-6, 1)
	n := 0
	for _, entry := range feed {
		n++ // title line
		if entry.cwd != "" {
			n++ // cwd line
		}
		for _, step := range entry.steps {
			// Mirror suppression logic from viewActivityFeed.
			if step.status == "done" && len(step.lines) == 0 {
				continue
			}
			n++ // step badge line
			stepLines := step.lines
			if len(stepLines) > maxStepOutputLines {
				stepLines = stepLines[len(stepLines)-maxStepOutputLines:]
			}
			for _, sl := range stepLines {
				for _, wl := range strings.Split(wrap.String(sl, stepMaxLen), "\n") {
					if wl != "" {
						n++
					}
				}
			}
		}
		entryLines := entry.lines
		skipped := 0
		if len(entryLines) > feedLinesPerEntry {
			skipped = len(entryLines) - feedLinesPerEntry
			entryLines = entryLines[skipped:]
		}
		if skipped > 0 {
			n++ // "… N earlier lines" message
		}
		for _, outLine := range entryLines {
			for _, wl := range strings.Split(wrap.String(stripANSI(outLine), entryMaxLen), "\n") {
				if wl != "" {
					n++
				}
			}
		}
	}
	return n
}

// feedRawLines returns ANSI-stripped content strings for every line in the feed
// (no box formatting). Used to capture line content when the user marks a line.
func (m Model) feedRawLines(width int) []string {
	pal := m.ansiPalette()
	var lines []string
	add := func(content string) { lines = append(lines, stripANSI(content)) }

	if len(m.feed) == 0 {
		add("  no activity yet")
		return lines
	}
	for _, entry := range m.feed {
		entryBadge, badgeColor := statusBadge(entry.status, pal)
		ts := entry.ts.Format("15:04:05")
		titleLine := fmt.Sprintf("  %s%s%s %s%s%s  %s",
			badgeColor, entryBadge, aRst,
			pal.Dim, ts, aRst,
			pal.Accent+entry.title+aRst)
		add(titleLine)

		if entry.cwd != "" {
			home, _ := os.UserHomeDir()
			cwdDisplay := entry.cwd
			if home != "" && strings.HasPrefix(cwdDisplay, home) {
				cwdDisplay = "~" + cwdDisplay[len(home):]
			}
			add(fmt.Sprintf("  %s  %s%s", pal.Dim, cwdDisplay, aRst))
		}

		if len(entry.steps) > 0 {
			const maxStepOutputLines = 5
			lastVisible := -1
			for i, step := range entry.steps {
				if !(step.status == "done" && len(step.lines) == 0) {
					lastVisible = i
				}
			}
			for i, step := range entry.steps {
				if step.status == "done" && len(step.lines) == 0 {
					continue
				}
				isLast := i == lastVisible
				var col string
				switch step.status {
				case "running":
					col = pal.Warn
				case "done":
					col = pal.Success
				case "failed":
					col = pal.Error
				default:
					col = pal.Dim
				}
				connector := "├ "
				if isLast {
					connector = "└ "
				}
				add("  " + connector + col + stepGlyph(step.status) + " " + step.id + aRst)
				stepLines := step.lines
				if len(stepLines) > maxStepOutputLines {
					stepLines = stepLines[len(stepLines)-maxStepOutputLines:]
				}
				stepMaxLen := max(width-10, 1)
				for _, sl := range stepLines {
					for _, wl := range strings.Split(wrap.String(sl, stepMaxLen), "\n") {
						if wl != "" {
							add("    " + pal.Dim + wl + aRst)
						}
					}
				}
			}
		}

		const feedLinesPerEntry = 10
		entryLines := entry.lines
		skipped := 0
		if len(entryLines) > feedLinesPerEntry {
			skipped = len(entryLines) - feedLinesPerEntry
			entryLines = entryLines[skipped:]
		}
		if skipped > 0 {
			add(pal.Dim + fmt.Sprintf("    … %d earlier lines (press f to scroll)", skipped) + aRst)
		}
		entryMaxLen := max(width-6, 1)
		for _, outLine := range entryLines {
			for _, wl := range strings.Split(wrap.String(stripANSI(outLine), entryMaxLen), "\n") {
				if wl != "" {
					add(pal.Dim + "    " + wl + aRst)
				}
			}
		}
	}
	return lines
}

// viewActivityFeed renders the center activity feed with scroll support.
func (m Model) viewActivityFeed(height, width int) string {
	pal := m.ansiPalette()
	borderColor := pal.Border
	titleColor := pal.Accent
	if m.feedFocused {
		borderColor = pal.Accent
	}

	feedSprite := PanelHeader(m.activeBundle(), "activity_feed", width, borderColor, titleColor)
	headerH := 1
	if feedSprite != nil {
		headerH = len(feedSprite)
	}
	visibleH := height - headerH - 2 // minus header, bottom border, and always-present hint footer

	// feedRowAt appends a content row, applying cursor and/or mark highlights.
	appendRow := func(lines *[]string, content string) {
		idx := len(*lines)
		isMarked := m.feedMarked[idx]
		isCursor := m.feedFocused && idx == m.feedCursor
		var row string
		switch {
		case isCursor && isMarked:
			// Cursor dominates visually; add mark indicator to content.
			row = boxRowCursorColor(pal.Success+"●"+aRst+content, width, borderColor)
		case isCursor:
			row = boxRowCursorColor(content, width, borderColor)
		case isMarked:
			// Marked: ● (success color, no bg) + content (green bg), matching inbox detail style.
			markPrefix := pal.Success + "●" + aRst
			markContent := lipgloss.NewStyle().
				Background(lipgloss.Color("#2d4a35")).
				Render(stripANSI(content))
			row = boxRow(markPrefix+markContent, width, borderColor)
		default:
			row = boxRow(content, width, borderColor)
		}
		*lines = append(*lines, row)
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

			// Show the working directory below the title if set.
			if entry.cwd != "" {
				home, _ := os.UserHomeDir()
				cwdDisplay := entry.cwd
				if home != "" && strings.HasPrefix(cwdDisplay, home) {
					cwdDisplay = "~" + cwdDisplay[len(home):]
				}
				appendRow(&allLines, fmt.Sprintf("  %s  %s%s", pal.Dim, cwdDisplay, aRst))
			}

			// Render per-step status badges with tree connectors; suppress done steps with no output.
			if len(entry.steps) > 0 {
				const maxStepOutputLines = 5
				// Find the last visible step (non-suppressed) index.
				lastVisible := -1
				for i, step := range entry.steps {
					if !(step.status == "done" && len(step.lines) == 0) {
						lastVisible = i
					}
				}
				for i, step := range entry.steps {
					// Suppress done steps that produced no output — they add no information.
					if step.status == "done" && len(step.lines) == 0 {
						continue
					}
					isLast := i == lastVisible
					var col string
					switch step.status {
					case "running":
						col = aYlw
					case "done":
						col = pal.Success
					case "failed":
						col = pal.Error
					default:
						col = pal.Dim
					}
					connector := pal.Dim + "├ " + aRst
					if isLast {
						connector = pal.Dim + "└ " + aRst
					}
					badge := "  " + connector + col + stepGlyph(step.status) + " " + step.id + aRst
					appendRow(&allLines, badge)
					// Per-step output lines (last maxStepOutputLines).
					stepLines := step.lines
					if len(stepLines) > maxStepOutputLines {
						stepLines = stepLines[len(stepLines)-maxStepOutputLines:]
					}
					// Output lines use a tree continuation connector for non-final steps.
					var outPrefix string
					if isLast {
						outPrefix = "      " // 6-char plain indent aligns with step content
					} else {
						outPrefix = "  " + pal.Dim + "│   " + aRst
					}
					stepMaxLen := max(width-10, 1)
					for _, sl := range stepLines {
						wlines := strings.Split(wrap.String(sl, stepMaxLen), "\n")
						for _, wl := range wlines {
							if wl != "" {
								appendRow(&allLines, outPrefix+pal.Dim+wl+aRst)
							}
						}
					}
				}
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
				wrapped := strings.Split(wrap.String(outLine, maxLen), "\n")
				for _, wl := range wrapped {
					if wl != "" {
						appendRow(&allLines, pal.Dim+"    "+wl+aRst)
					}
				}
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
		lines = append(lines, boxTop(width, RenderHeader("activity_feed")+scrollSuffix, borderColor, titleColor))
	}
	lines = append(lines, visible...)

	// Pad to fill the box body, leaving room for hint footer row (always present).
	for len(lines) < height-2 {
		lines = append(lines, boxRow("", width, borderColor))
	}
	// Hint footer row — always present; shows hints when focused, blank when not.
	var feedHints []panelrender.Hint
	if m.feedFocused {
		markDesc := "mark"
		switch m.feedMarkMode {
		case MarkModeActive:
			markDesc = "pause"
		case MarkModePaused:
			markDesc = "resume"
		}
		feedHints = []panelrender.Hint{
			{Key: "j/k", Desc: "nav"},
			{Key: "[", Desc: "page up"},
			{Key: "]", Desc: "page down"},
			{Key: "g", Desc: "top"},
			{Key: "G", Desc: "bottom"},
			{Key: "m", Desc: markDesc},
		}
		if m.feedMarkMode != MarkModeOff {
			feedHints = append(feedHints, panelrender.Hint{Key: "A", Desc: "mark all"})
			feedHints = append(feedHints, panelrender.Hint{Key: "D", Desc: "clear"})
		}
		if markCount := len(m.feedMarked); markCount > 0 {
			feedHints = append(feedHints, panelrender.Hint{Key: "r", Desc: fmt.Sprintf("run (%d)", markCount)})
		}
	}
	lines = append(lines, boxRow(panelrender.HintBar(feedHints, width-2, pal), width, borderColor))
	lines = append(lines, boxBot(width, borderColor))

	// Trim to exact height.
	if len(lines) > height {
		lines = lines[:height]
	}

	return strings.Join(lines, "\n")
}

// ── Box drawing helpers ────────────────────────────────────────────────────────

func boxTop(w int, title, borderColor, labelColor string) string {
	return panelrender.BoxTop(w, title, borderColor, labelColor)
}

func boxBot(w int, borderColor string) string {
	return panelrender.BoxBot(w, borderColor)
}

func boxRow(content string, w int, borderColor string) string {
	return panelrender.BoxRow(content, w, borderColor)
}

// boxRowCursor renders a feed row with a "> " cursor marker prepended to the
// content, keeping the total visible width equal to a normal boxRow.
func boxRowCursor(content string, w int) string {
	return boxRowCursorColor(content, w, aBC)
}

// boxRowCursorColor is the color-aware version of boxRowCursor.
// The cursor indicator overlays the first 2 visible columns of content so that
// row width is identical to a non-cursor boxRow — no layout shift occurs.
// borderColor is used for both the box borders and the "> " cursor mark color,
// so it matches the active theme accent when the feed is focused.
func boxRowCursorColor(content string, w int, borderColor string) string {
	inner := w - 2
	// Work in plain text (strip ANSI) to overlay the first 2 visible columns.
	plain := stripANSI(content)
	runes := []rune(plain)
	var rest string
	if len(runes) > 2 {
		rest = string(runes[2:])
	}
	// "> " occupies exactly 2 visible columns; pad remainder to fill inner width.
	used := 2 + lipgloss.Width(rest)
	pad := max(inner-used, 0)
	cursorMark := borderColor + "> " + aRst
	return borderColor + "│" + aRst + cursorMark + rest + strings.Repeat(" ", pad) + borderColor + "│" + aRst
}

// stepGlyph returns the extended-ASCII glyph for a step status string.
// Glyphs are chosen from the CP437/ANSI 128–255 range for visual variety:
//
//	pending  → · (U+00B7 middle dot)
//	running  → » (U+00BB right double angle quotation)
//	done     → ° (U+00B0 degree sign)
//	failed   → × (U+00D7 multiplication sign)
func stepGlyph(status string) string {
	switch status {
	case "running":
		return "»"
	case "done":
		return "°"
	case "failed":
		return "×"
	default: // "pending" or unknown
		return "·"
	}
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

// overlayCenter draws overlay centered over base, delegating to panelrender.
func overlayCenter(base, overlay string, w, h int) string {
	return panelrender.OverlayCenter(base, overlay, w, h)
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
// It opens the result store and passes it to the model so the Inbox panel
// can display recorded pipeline and agent run history.
func Run() {
	s, _ := store.Open() // nil-safe — inbox renders empty state on failure
	m := NewWithStore(s)
	p := tea.NewProgram(m, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Printf("switchboard error: %v\n", err)
	}
	if s != nil {
		_ = s.Close()
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

// ensureCronDaemon starts the orcai-cron tmux session if it does not already exist.
func ensureCronDaemon() {
	// Check if orcai-cron session exists.
	err := exec.Command("tmux", "has-session", "-t", "orcai-cron").Run()
	if err == nil {
		return // already running
	}
	// Find the orcai binary next to the running binary.
	self, _ := os.Executable()
	bin := self
	if altBin, err := exec.LookPath("orcai"); err == nil {
		bin = altBin
	}
	exec.Command(bin, "cron", "start").Run() //nolint:errcheck
}
