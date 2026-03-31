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
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	robfigcron "github.com/robfig/cron/v3"

	"github.com/adam-stokes/orcai/internal/activity"
	orcaicron "github.com/adam-stokes/orcai/internal/cron"
	"github.com/adam-stokes/orcai/internal/busd/topics"
	"github.com/adam-stokes/orcai/internal/inbox"
	"github.com/adam-stokes/orcai/internal/jumpwindow"
	"github.com/adam-stokes/orcai/internal/modal"
	"github.com/adam-stokes/orcai/internal/panelrender"
	"github.com/adam-stokes/orcai/internal/picker"
	"github.com/adam-stokes/orcai/internal/pipelineeditor"
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
	FeedPaused
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
	providers  []picker.ProviderDef
	agentPicker modal.AgentPickerModel
	prompt     textarea.Model
	focused    bool
}

type jobHandle struct {
	id           string
	cancel       context.CancelFunc
	ch           chan tea.Msg
	tmuxWindow   string
	logFile      string // /tmp/orcai-<feedID>.log — tailed in the tmux window
	storeRunID   int64  // non-zero when run was recorded in the result store
	pipelineName string // non-empty for pipeline jobs; matched against busd RunStarted payload
	actAgent     string // activity feed agent name; empty for non-agent jobs
	actLabel     string // activity feed label (truncated prompt)
	executorID   string // executor type ("claude", "opencode", etc.) — for clarification follow-up
	modelID      string // model slug — for clarification follow-up
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

// inboxEditorDoneMsg is dispatched when the $EDITOR process launched from the
// inbox detail view exits.
type inboxEditorDoneMsg struct{ err error }

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
	activeFilter string // "unread" | "read" | "attention" | "all"
}

// inboxFilterCycle returns the next filter in the rotation.
func inboxFilterCycle(current string) string {
	switch current {
	case "unread":
		return "read"
	case "read":
		return "attention"
	case "attention":
		return "all"
	default:
		return "unread"
	}
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
	agentsGridRow         int  // selected row in the agents grid (center panel)
	agentsGridCol         int  // selected column in the agents grid (center panel)
	agentsCenterFocused   bool // true when center (agents) panel has keyboard focus
	confirmDelete         bool
	pendingDeletePipeline string
	agentModalOpen       bool
	agentModalFocus      int                    // 0=provider/model, 1=saved-prompt, 2=prompt, 3=use_brain, 4=cwd, 5=schedule, 6=name
	agentPrompts         []store.Prompt         // loaded from store when modal opens
	agentPromptIdx       int                    // selected saved-prompt index; 0 = none
	savedPromptPicker    modal.FuzzyPickerModel // inline fuzzy picker for saved prompts
	agentSchedule         textarea.Model
	agentScheduleErr      string
	agentUseBrain         bool
	agentNameInput        textinput.Model
	helpOpen              bool
	jumpOpen              bool
	jumpModal             jumpwindow.EmbeddedModel
	registry              *themes.Registry
	themeState            tuikit.ThemeState
	themePicker           tuikit.ThemePicker
	previewBundle         *themes.Bundle // non-nil while theme picker is open (live preview)
	// CWD / dir picker
	launchCWD           string         // CWD at orcai startup (immutable after New())
	agentCWD            string         // current agent session CWD (user-editable)
	dirPicker           DirPickerModel // reusable dir picker overlay
	dirPickerOpen       bool           // whether the dir picker overlay is visible
	dirPickerCtx        string         // "agent" or "pipeline"
	pendingPipelineName string         // pipeline waiting for CWD selection
	pendingPipelineYAML string         // YAML path for pendingPipelineName
	pendingActAgent     string         // activity feed agent name for the next launch
	pendingActLabel     string         // activity feed label for the next launch
	pendingExecutorID   string         // executor ID for the next launch (for clarification wiring)
	pendingModelID      string         // model ID for the next launch
	activityFeed        activity.Model // JSONL-backed activity timeline
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
	inboxEditorTempFile     string // path to temp file open in $EDITOR
	inboxEditorClipSnapshot string // clipboard state before editor launch
	inboxEditorOrigContent  string // original content written to temp file
	// Pipeline editor (full-screen two-column TUI).
	pipelineEditorOpen bool
	pipelineEditor     pipelineeditor.Model
	// Re-run modal
	rerunModal    modal.RerunModal
	showRerun     bool
	// Cron panel
	cronPanel CronPanel
	// Pipeline bus subscription (tasks 7.2–7.8)
	pipelineBusCh chan pipelineBusEventMsg
	// Inline clarification reply (agent.run.clarification)
	pendingClarification       *store.ClarificationRequest // non-nil when awaiting reply
	pendingClarificationOutput string                      // partial output up to the question
	clarifyInput               textinput.Model
	clarifyActive              bool
	clarifyNeedsResume         bool // true only when loaded from DB on startup (subprocess may be dead)
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

	clarifyTI := textinput.New()
	clarifyTI.Placeholder = "type your answer…"
	clarifyTI.CharLimit = 2000

	agentProviders := picker.BuildProviders()
	m := Model{
		launcher: launcherSection{
			pipelines: ScanPipelines(pipelinesDir()),
			focused:   true,
		},
		agent: agentSection{
			providers:   agentProviders,
			agentPicker: modal.NewAgentPickerModel(agentProviders),
			prompt:      ta,
		},
		agentSchedule:         schedTA,
		agentNameInput:        func() textinput.Model { ti := textinput.New(); ti.Placeholder = "agent-<provider>-<timestamp>"; ti.CharLimit = 128; return ti }(),
		savedPromptPicker:     modal.NewFuzzyPickerModel(8),
		pipelineEditor:        pipelineeditor.New(agentProviders, pipelinesDir(), s),
		pipelineScheduleInput: pipeSchedTA,
		signalBoard:           SignalBoard{activeFilter: "running"},
		inboxPanel:            InboxPanel{activeFilter: "unread"},
		activeJobs:            make(map[string]*jobHandle),
		launchCWD:             cwd,
		agentCWD:              cwd,
		inboxModel:   inbox.New(s, nil), // bundle set after registry loads
		store:        s,
		clarifyInput: clarifyTI,
		activityFeed: activity.New(""),
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
	testProviders := []picker.ProviderDef{
		{
			ID:    "test-provider",
			Label: "Test Provider",
			Models: []picker.ModelOption{
				{ID: "model-a", Label: "Model A"},
				{ID: "model-b", Label: "Model B"},
			},
		},
	}
	m.agent.providers = testProviders
	m.agent.agentPicker = modal.NewAgentPickerModel(testProviders)
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

// AgentPromptIdx returns the selected saved-prompt index — used in tests.
func (m Model) AgentPromptIdx() int { return m.agentPromptIdx }

// SavedPromptPickerOpen reports whether the saved-prompt inline picker is open — used in tests.
func (m Model) SavedPromptPickerOpen() bool { return m.savedPromptPicker.IsOpen() }

// DirPickerOpen reports whether the dir picker is open — used in tests.
func (m Model) DirPickerOpen() bool { return m.dirPickerOpen }

// DirPickerCtx returns the dir picker context string — used in tests.
func (m Model) DirPickerCtx() string { return m.dirPickerCtx }

// WithAgentPrompts returns a copy of the model with the given prompts loaded — used in tests.
func (m Model) WithAgentPrompts(prompts []store.Prompt) Model {
	m.agentPrompts = prompts
	return m
}

// AgentScheduleErr returns the agent schedule error string — used in tests.
func (m Model) AgentScheduleErr() string { return m.agentScheduleErr }

// AgentUseBrain returns whether the agent should use the brain — used in tests.
func (m Model) AgentUseBrain() bool { return m.agentUseBrain }

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

// AgentsCenterFocused returns whether the agents grid (center panel) is focused — used in tests.
func (m Model) AgentsCenterFocused() bool { return m.agentsCenterFocused }

// AgentsGridRow returns the agents grid cursor row — used in tests.
func (m Model) AgentsGridRow() int { return m.agentsGridRow }

// AgentsGridCol returns the agents grid cursor column — used in tests.
func (m Model) AgentsGridCol() int { return m.agentsGridCol }

// SetAgentsCenterFocused sets the agents center panel focus state — used in tests.
func (m Model) SetAgentsCenterFocused(v bool) Model { m.agentsCenterFocused = v; return m }

// RightColWidth returns the right column width — used in tests.
func (m Model) RightColWidth() int { return m.rightColWidth() }

// MidColWidth returns the center column width — used in tests.
func (m Model) MidColWidth() int { return m.midColWidth() }

// LeftColWidth returns the left column width — used in tests.
func (m Model) LeftColWidth() int { return m.leftColWidth() }

// ViewAgentModalBox is an exported wrapper for tests.
func (m Model) ViewAgentModalBox(w, h int) string { return m.viewAgentModalBox(w, h) }

// BuildCronSection is an exported wrapper for tests.
func (m Model) BuildCronSection(w int) []string { return m.buildCronSection(w, 10) }

// ViewActivityFeed renders the activity feed panel — used in tests.
func (m Model) ViewActivityFeed(h, w int) string { return m.viewActivityFeed(h, w) }

// FeedLineCount returns the navigable feed line count — used in tests.
func (m Model) FeedLineCount() int { return m.feedLineCount() }

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

// FeedLen returns the number of entries in the feed ring buffer — used in tests.
func (m Model) FeedLen() int { return len(m.feed) }

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
func (m Model) AgentFocused() bool        { return m.agent.focused }

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
// During theme picker navigation, previewBundle overrides the registry value so
// panels update in real time without persisting the selection.
func (m Model) activeBundle() *themes.Bundle {
	if m.previewBundle != nil {
		return m.previewBundle
	}
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
		recoverOrphanedRunsCmd(m.store), // seed runs AFTER recovery completes
		tickCmd(),
		m.inboxModel.Init(),
		m.themeState.Init(),
		tryPipelineBusSubscribeCmd(),
		m.activityFeed.Init(),
		loadPendingClarificationsCmd(m.store),
	)
}

func tickCmd() tea.Cmd {
	return tea.Tick(3*time.Second, func(t time.Time) tea.Msg { return tickMsg(t) })
}

// ── Pipeline helpers ──────────────────────────────────────────────────────────

func pipelinesDir() string {
	if d := os.Getenv("ORCAI_PIPELINES_DIR"); d != "" {
		return d
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".config", "orcai", "pipelines")
}

// WriteSingleStepPipeline generates a minimal single-step pipeline YAML for a
// scheduled agent run and writes it to the .agents/ subdirectory of the pipelines
// directory so it does not appear in the PIPELINES launcher panel. Returns the
// absolute path of the written file so the caller can reference it in a cron entry.
// Exported so tests can call it directly.
func WriteSingleStepPipeline(name, providerID, modelID, prompt string, useBrain bool) (string, error) {
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

	useBrainYAML := ""
	if useBrain {
		useBrainYAML = "\n    use_brain: true"
	}

	content := fmt.Sprintf("name: %s\nversion: \"1\"\nsteps:\n  - id: run\n    executor: %s%s%s\n    prompt: |\n%s",
		name, providerID, model, useBrainYAML, promptLines.String())

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
	// ── Forward non-key messages to activity feed (handles its unexported msg types) ──
	// Skip KeyMsg — activity model navigation is driven explicitly via handleDown/handleUp
	// when feedFocused. Forwarding all keys caused j/k in other panes to scroll the feed.
	if _, isKey := msg.(tea.KeyMsg); !isKey {
		newAF, afCmd := m.activityFeed.Update(msg)
		if af, ok := newAF.(activity.Model); ok {
			m.activityFeed = af
		}
		if afCmd != nil {
			switch msg.(type) {
			case tea.WindowSizeMsg, tickMsg:
				// shared messages: fall through to main switch
			default:
				return m, afCmd
			}
		}
	}

	// ── Theme change from another process (e.g. cron TUI) ────────────────────
	if ts, cmd, ok := m.themeState.Handle(msg); ok {
		m.themeState = ts
		if m.themePicker.Open {
			// Theme picker is navigating — use the bundle from ThemeState for
			// live preview without writing to disk or updating the registry.
			m.previewBundle = ts.Bundle()
		} else {
			m.previewBundle = nil
			if m.registry != nil {
				m.registry.RefreshActive()
			}
		}
		return m, cmd
	}

	switch msg := msg.(type) {

	// ── Agent prompt pre-selection ────────────────────────────────────────────

	case agentPromptsLoadedMsg:
		m.agentPrompts = msg.prompts
		return m, nil

	// ── Pending clarifications loaded from DB on startup ─────────────────────

	case pendingClarificationsMsg:
		if len(msg.reqs) > 0 {
			m.pendingClarification = &msg.reqs[0]
			m.pendingClarificationOutput = msg.reqs[0].Output
			m.clarifyInput.Reset()
			m.clarifyInput.Focus()
			m.clarifyActive = true
			m.clarifyNeedsResume = true // loaded from DB on startup; subprocess may be dead
		}
		return m, nil

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
		// Reclamp feed scroll so bounds stay accurate when entries are appended
		// between navigation events (e.g. new steps arriving while idle).
		m.clampFeedScroll()
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
		wasClarifyActive := m.clarifyActive
		m = m.appendFeedLine(msg.ID, msg.Line)
		// For in-process (agent) jobs the log file is written here.
		// Window-mode (pipeline) jobs write via tee in the shell — skip.
		for _, e := range m.feed {
			if e.id == msg.ID && e.logFile != "" && e.doneFile == "" {
				appendToFile(e.logFile, stripANSI(msg.Line)+"\n")
				break
			}
		}
		// If clarification was just triggered, start cursor blink alongside the drain.
		if m.clarifyActive && !wasClarifyActive {
			if jh, ok := m.activeJobs[msg.ID]; ok {
				return m, tea.Batch(drainChan(jh.ch), textinput.Blink)
			}
			return m, textinput.Blink
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

	// ── orphan recovery ───────────────────────────────────────────────────

	case orphanRecoveryMsg:
		for _, id := range msg.recoveredIDs {
			_ = activity.AppendEvent(activity.DefaultPath(), activity.ActivityEvent{
				TS:     time.Now().Format(time.RFC3339),
				Kind:   "pipeline_failed",
				Agent:  fmt.Sprintf("run-%d", id),
				Label:  "interrupted",
				Status: "failed",
			})
		}
		// Seed the signal board feed now that orphans are marked — ensures
		// they appear as failed rather than running.
		return m, seedFeedFromStoreCmd(m.store)

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
		// Intercept clarification requests before the general event dispatcher.
		if msg.topic == topics.ClarificationRequested {
			var req store.ClarificationRequest
			if json.Unmarshal(msg.payload, &req) == nil {
				m.pendingClarification = &req
				m.pendingClarificationOutput = req.Output
				m.clarifyInput.Reset()
				m.clarifyInput.Focus()
				m.clarifyActive = true
				m.clarifyNeedsResume = false // pipeline subprocess is alive; DB write is enough
				// Update the signal board to show the run as paused.
				if req.RunID != "" {
					m = m.updateFeedEntryStatusByRunID(req.RunID, FeedPaused)
				}
			}
			if m.pipelineBusCh != nil {
				return m, tea.Batch(textinput.Blink, waitForPipelineBusEvent(m.pipelineBusCh))
			}
			return m, textinput.Blink
		}
		m = m.handlePipelineBusEvent(msg)
		// Also trigger an inbox refresh so new/updated runs appear immediately.
		var inboxCmd tea.Cmd
		m.inboxModel, inboxCmd = m.inboxModel.Update(inbox.RunCompletedMsg{})
		if m.pipelineBusCh != nil {
			return m, tea.Batch(inboxCmd, waitForPipelineBusEvent(m.pipelineBusCh))
		}
		return m, inboxCmd

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
				"run_id":       msg.id,
				"store_run_id": jh.storeRunID,
				"exit_status":  exitCode,
			})
			if jh.actAgent != "" {
				doneCmd = tea.Batch(doneCmd, appendActivityCmd(jh.actAgent, jh.actLabel, "agent_run_finished", "done"))
			}
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
			if jh.actAgent != "" {
				failCmd = tea.Batch(failCmd, appendActivityCmd(jh.actAgent, jh.actLabel, "agent_run_failed", "failed"))
			}
		}
		delete(m.activeJobs, msg.id)
		return m, failCmd

	case inboxEditorDoneMsg:
		tmpFile := m.inboxEditorTempFile
		origContent := m.inboxEditorOrigContent
		snapClip := m.inboxEditorClipSnapshot
		m.inboxEditorTempFile = ""
		m.inboxEditorOrigContent = ""
		m.inboxEditorClipSnapshot = ""

		var injected string
		// Prefer clipboard if it changed since before the editor launched.
		if clip := readClipboard(); clip != "" && clip != snapClip {
			injected = strings.TrimSpace(clip)
		} else if tmpFile != "" {
			// Fall back to saved file content if it differs from what we wrote.
			if raw, err := os.ReadFile(tmpFile); err == nil {
				if saved := strings.TrimSpace(string(raw)); saved != origContent && saved != "" {
					injected = saved
				}
			}
		}
		// Clean up temp file regardless.
		if tmpFile != "" {
			_ = os.Remove(tmpFile)
		}
		if injected != "" {
			m.inboxDetailOpen = false
			m.agent.prompt.SetValue(injected)
			m.agentModalOpen = true
			m.agentModalFocus = 2
			m.agentPromptIdx = 0
			m.agent.prompt.Focus()
			m.agentNameInput.SetValue(fmt.Sprintf("agent-%s-%d", m.agent.agentPicker.SelectedProviderID(), time.Now().Unix()))
			return m, loadAgentPromptsCmd(m.store)
		}
		return m, nil

	case tea.KeyMsg:
		// These keys always go through handleKey regardless of which panel is focused.
		switch msg.String() {
		case "tab", "ctrl+c", "ctrl+q":
			return m.handleKey(msg)
		}
		// When any global overlay is active, all keys must go through handleKey
		// so ESC / y / n can dismiss it regardless of which panel is focused.
		if m.confirmQuit || m.helpOpen || m.jumpOpen || m.agentModalOpen || m.themePicker.Open || m.dirPickerOpen || m.confirmDelete || m.pipelineLaunchMode != plModeNone || m.showRerun || m.pipelineEditorOpen {
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

	case jumpwindow.CloseMsg:
		m.jumpOpen = false
		return m, nil

	case jumpwindow.PipelinesMsg:
		// Jump window selected the pipelines entry: close jump window and open editor.
		m.jumpOpen = false
		m.pipelineEditor.SetProviders(m.agent.providers)
		m.pipelineEditor.SetPalette(m.ansiPalette())
		m.pipelineEditor.SetSize(m.width, m.height-1)
		m.pipelineEditorOpen = true
		return m, nil

	case pipelineeditor.CloseMsg:
		m.pipelineEditorOpen = false
		m.launcher.pipelines = ScanPipelines(pipelinesDir())
		return m, nil

	case pipelineeditor.RunLineMsg, pipelineeditor.RunDoneMsg, pipelineeditor.ClarifyPollMsg:
		if m.pipelineEditorOpen {
			updated, cmd := m.pipelineEditor.HandleMsg(msg)
			m.pipelineEditor = updated
			return m, cmd
		}
		return m, nil

	case inbox.RunCompletedMsg:
		// Immediate inbox refresh when a run completes in-process.
		var inboxCmd tea.Cmd
		m.inboxModel, inboxCmd = m.inboxModel.Update(msg)
		return m, inboxCmd

	case modal.RerunConfirmedMsg:
		m.showRerun = false
		return m.submitRerun(msg)

	case modal.RerunCancelledMsg:
		m.showRerun = false
		return m, nil
	}

	// Forward all other messages to the inbox model (poll ticks, etc.).
	var inboxCmd tea.Cmd
	m.inboxModel, inboxCmd = m.inboxModel.Update(msg)

	return m, inboxCmd
}

func (m Model) handleKey(msg tea.KeyMsg) (Model, tea.Cmd) {
	key := msg.String()

	// Clarification reply input — capture keys only when detail view is open
	// for the pending run. When the user is navigating other panels, clarifyActive
	// just drives the ? badge; it must not steal global navigation shortcuts.
	if m.clarifyActive && m.inboxDetailOpen {
		switch key {
		case "esc":
			m.clarifyActive = false
			m.clarifyNeedsResume = false
			m.pendingClarification = nil
			m.clarifyInput.Blur()
			m.clarifyInput.Reset()
			return m, nil
		case "enter":
			answer := strings.TrimSpace(m.clarifyInput.Value())
			if answer == "" {
				return m, nil
			}
			runID := ""
			if m.pendingClarification != nil {
				runID = m.pendingClarification.RunID
			}
			m.clarifyActive = false
			m.clarifyNeedsResume = false
			m.pendingClarification = nil
			m.clarifyInput.Blur()
			m.clarifyInput.Reset()

			// Update the signal board to show the run as resuming.
			if runID != "" {
				m = m.updateFeedEntryStatusByRunID(runID, FeedRunning)
			}

			// Persist the answer to the DB so the polling pipeline subprocess
			// can pick it up even if busd is not available.
			if m.store != nil && runID != "" {
				_ = m.store.AnswerClarification(runID, answer)
			}

			// Launch a resume job window only when the clarification was loaded
			// from DB on startup (subprocess may be dead). When the pipeline is
			// live (clarifyNeedsResume=false), the original subprocess is still
			// polling the DB and will pick up the answer itself.
			if runID != "" && m.clarifyNeedsResume {
				cwd := ""
				for _, r := range m.inboxModel.Runs() {
					if strconv.FormatInt(r.ID, 10) == runID {
						var rmeta struct {
							CWD string `json:"cwd"`
						}
						_ = json.Unmarshal([]byte(r.Metadata), &rmeta)
						cwd = rmeta.CWD
						break
					}
				}
				bin, _ := os.Executable()
				shellCmd := fmt.Sprintf("%s pipeline resume --run-id %s", bin, runID)
				feedID := "resume-" + runID
				createJobWindow(feedID, shellCmd, "resume-"+runID, cwd)
			}

			// Publish reply so the blocking AskClarification call in the pipeline
			// subprocess can unblock and continue execution with the user's answer.
			reply := store.ClarificationReply{RunID: runID, Answer: answer}
			return m, publishClarificationReplyCmd(reply)
		default:
			var cmd tea.Cmd
			m.clarifyInput, cmd = m.clarifyInput.Update(msg)
			return m, cmd
		}
	}

	// Dir picker overlay — capture all keys when open.
	if m.dirPickerOpen {
		var cmd tea.Cmd
		m.dirPicker, cmd = m.dirPicker.Update(msg)
		return m, cmd
	}

	// Re-run modal — capture all keys when open.
	if m.showRerun {
		var cmd tea.Cmd
		m.rerunModal, cmd = m.rerunModal.Update(msg)
		return m, cmd
	}

	// Jump window modal — route all keys to the embedded model.
	if m.jumpOpen {
		var cmd tea.Cmd
		m.jumpModal, cmd = m.jumpModal.Update(msg)
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
			// Keep cursor in view: scroll down only if cursor exceeds bottom edge.
			visibleH := max(m.height-4, 4)
			if m.inboxDetailCursor >= m.inboxDetailScroll+visibleH {
				m.inboxDetailScroll = m.inboxDetailCursor - visibleH + 1
			}
		case "k", "up":
			if m.inboxMarkMode == MarkModeActive {
				m = m.inboxDetailToggleMark()
			}
			if m.inboxDetailCursor > 0 {
				m.inboxDetailCursor--
			}
			// Keep cursor in view: scroll up only if cursor goes above top edge.
			if m.inboxDetailCursor < m.inboxDetailScroll {
				m.inboxDetailScroll = m.inboxDetailCursor
			}
		case "pgup", "[":
			step := max(m.height-4, 4)
			m.inboxDetailCursor = max(m.inboxDetailCursor-step, 0)
			m.inboxDetailScroll = max(m.inboxDetailScroll-step, 0)
		case "pgdown", "]":
			step := max(m.height-4, 4)
			m.inboxDetailCursor += step
			m.inboxDetailScroll += step
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
		case "e":
			// Open run content in $EDITOR; inject yanked/clipboard text into agent runner on return.
			editor, ok := editorCmd()
			if !ok {
				m = m.AddFeedEntry("inbox-editor-err", "set $EDITOR to use the editor feature", FeedDone, nil)
				return m, nil
			}
			runs := m.filteredInboxRuns()
			if idx := m.inboxDetailIdx; idx >= 0 && idx < len(runs) {
				run := runs[idx]
				pal := m.ansiPalette()
				plain := stripANSI(buildRunContent(run, pal, false, 80))
				tmp, err := os.CreateTemp("", "orcai-inbox-*.txt")
				if err == nil {
					_, _ = tmp.WriteString(plain)
					_ = tmp.Close()
					m.inboxEditorTempFile = tmp.Name()
					m.inboxEditorOrigContent = plain
					m.inboxEditorClipSnapshot = clipboardSnapshot()
					cmd := exec.Command(editor, tmp.Name()) //nolint:gosec
					return m, tea.ExecProcess(cmd, func(err error) tea.Msg {
						return inboxEditorDoneMsg{err: err}
					})
				}
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
				m.agentPromptIdx = 0
				m.agent.prompt.Focus()
				m.agentNameInput.SetValue(fmt.Sprintf("agent-%s-%d", m.agent.agentPicker.SelectedProviderID(), time.Now().Unix()))
				return m, loadAgentPromptsCmd(m.store)
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

	// Pipeline editor — captured before panel handlers.
	if m.pipelineEditorOpen {
		updatedEditor, cmd := m.pipelineEditor.HandleKey(msg)
		m.pipelineEditor = updatedEditor
		return m, cmd
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
		case "f":
			m.inboxPanel.activeFilter = inboxFilterCycle(m.inboxPanel.activeFilter)
			m.inboxPanel.selectedIdx = 0
			m.inboxPanel.scrollOffset = 0
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
		case "r":
			runs := m.filteredInboxRuns()
			if len(runs) > 0 && m.inboxPanel.selectedIdx >= 0 && m.inboxPanel.selectedIdx < len(runs) {
				run := runs[m.inboxPanel.selectedIdx]
				rm := modal.NewRerunModal(run, m.agent.providers, m.launchCWD)
				// agentRunModelSlug looks in .agents/; returns "" for regular pipelines.
				if slug := agentRunModelSlug(run.Name); slug != "" {
					rm = rm.WithModelSlug(slug)
				}
				m.rerunModal = rm
				m.showRerun = true
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

	case "tab", "shift+tab":
		fwd := key == "tab"
		// Tab order follows the three-column layout, left→right top→bottom:
		//   LEFT col:   launcher → inbox → cron
		//   CENTER col: agentsCenter → agent runner (provider cards)
		//   RIGHT col:  signalBoard → feed
		//   wrap back to launcher
		//
		// Agent runner: tab/shift+tab cycles the provider selection instead of
		// leaving the panel — only a second tab press wraps to the next panel.
		if m.agent.focused {
			// Agent runner: cycle provider selection within the panel.
			direction := "j"
			if !fwd {
				direction = "k"
			}
			newPicker, _ := m.agent.agentPicker.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(direction)})
			// If the picker wrapped (no movement), leave agent runner panel.
			if newPicker.SelectedProviderID() == m.agent.agentPicker.SelectedProviderID() {
				if fwd {
					m.agent.focused = false
					m.signalBoardFocused = true
				} else {
					m.agent.focused = false
					m.agentsCenterFocused = true
				}
			} else {
				m.agent.agentPicker = newPicker
			}
			return m, nil
		}
		if fwd {
			switch {
			case m.launcher.focused:
				m.launcher.focused = false
				m.inboxPanel.focused = true
				m.inboxModel.SetFocused(true)
			case m.inboxPanel.focused:
				m.inboxPanel.focused = false
				m.inboxModel.SetFocused(false)
				m.cronPanel.focused = true
			case m.cronPanel.focused:
				m.cronPanel.focused = false
				m.agentsCenterFocused = true
			case m.agentsCenterFocused:
				m.agentsCenterFocused = false
				m.agent.focused = true
			case m.signalBoardFocused:
				m.signalBoardFocused = false
				m.signalBoard.clearSearch()
				m.feedFocused = true
				m.feedCursor = 0
			case m.feedFocused:
				m = m.clearFeedMarkMode()
				m.feedFocused = false
				m.launcher.focused = true
			default:
				m.launcher.focused = true
			}
		} else {
			switch {
			case m.launcher.focused:
				m.launcher.focused = false
				m = m.clearFeedMarkMode()
				m.feedFocused = true
				m.feedCursor = 0
			case m.inboxPanel.focused:
				m.inboxPanel.focused = false
				m.inboxModel.SetFocused(false)
				m.launcher.focused = true
			case m.cronPanel.focused:
				m.cronPanel.focused = false
				m.inboxPanel.focused = true
				m.inboxModel.SetFocused(true)
			case m.agentsCenterFocused:
				m.agentsCenterFocused = false
				m.cronPanel.focused = true
			case m.signalBoardFocused:
				m.signalBoardFocused = false
				m.signalBoard.clearSearch()
				m.agent.focused = true
			case m.feedFocused:
				m = m.clearFeedMarkMode()
				m.feedFocused = false
				m.signalBoardFocused = true
			default:
				m.launcher.focused = true
			}
		}
		return m, nil

	case "f":
		if m.signalBoardFocused || m.agentsCenterFocused {
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
			m.agentPromptIdx = 0
			m.agent.prompt.Focus()
			m.agentNameInput.SetValue(fmt.Sprintf("agent-%s-%d", m.agent.agentPicker.SelectedProviderID(), time.Now().Unix()))
			return m, loadAgentPromptsCmd(m.store)
		}
		// Global refresh: reload pipelines and providers.
		m.launcher.pipelines = ScanPipelines(pipelinesDir())
		if m.launcher.selected >= len(m.launcher.pipelines) && m.launcher.selected > 0 {
			m.launcher.selected = max(len(m.launcher.pipelines)-1, 0)
		}
		newProviders := picker.BuildProviders()
		m.agent.providers = newProviders
		m.agent.agentPicker = modal.NewAgentPickerModel(newProviders)
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

	case "J":
		if !m.agentModalOpen && !m.confirmDelete && !m.confirmQuit && !m.jumpOpen {
			jm := jumpwindow.NewEmbedded(m.themeState.Bundle())
			jm.SetSize(m.width, m.height-2)
			m.jumpModal = jm
			m.jumpOpen = true
		}
		return m, nil

	case "pgdown", "]":
		if m.feedFocused {
			if m.feedMarkMode == MarkModeActive {
				m = m.feedToggleMark(m.feedCursor)
			}
			total := m.feedLineCount()
			step := m.feedVisibleHeight()
			m.feedCursor = min(m.feedCursor+step, max(0, total-1))
			m.feedScrollOffset += step
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
			m.feedScrollOffset -= step
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
			total := m.feedLineCount()
			m.feedCursor = max(0, total-1)
			visibleH := m.feedVisibleHeight()
			m.feedScrollOffset = max(0, m.feedCursor-visibleH+1)
			m.clampFeedScroll()
			return m, nil
		}

	case "h":
		// h/l navigate the agents grid columns when center panel is focused.
		if m.agentsCenterFocused {
			if m.agentsGridCol > 0 {
				m.agentsGridCol--
			}
			return m, nil
		}

	case "l":
		if m.agentsCenterFocused {
			entries := fuzzyFeed(m.signalBoard.query, m.filteredFeed())
			gridCols := max(1, m.midColWidth()/24)
			gridRows := (len(entries) + gridCols - 1) / gridCols
			if gridRows == 0 {
				return m, nil
			}
			maxColInRow := min(gridCols, len(entries)-m.agentsGridRow*gridCols) - 1
			if maxColInRow < 0 {
				maxColInRow = 0
			}
			if m.agentsGridCol < maxColInRow {
				m.agentsGridCol++
			}
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
		} else if m.agentsCenterFocused {
			m.agentsCenterFocused = false
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
						if jh.storeRunID != 0 && m.store != nil {
							var out string
							for _, e := range m.feed {
								if e.id == entry.id {
									out = strings.Join(e.lines, "\n")
									break
								}
							}
							_ = m.store.RecordRunComplete(jh.storeRunID, 1, out, "")
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

// feedVisibleHeight returns the number of visible lines in the feed panel,
// using the same formula as clampFeedScroll and View().
func (m Model) feedVisibleHeight() int {
	h := m.height
	if h <= 0 {
		h = 40
	}
	contentH := max(h-2, 5)
	sbH := m.signalBoardPanelHeight(contentH)
	feedH := max(contentH-sbH, 3)
	v := feedH - 2
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
	if m.agentsCenterFocused {
		entries := fuzzyFeed(m.signalBoard.query, m.filteredFeed())
		gridCols := max(1, m.midColWidth()/24)
		gridRows := (len(entries) + gridCols - 1) / gridCols
		if gridRows > 0 && m.agentsGridRow < gridRows-1 {
			m.agentsGridRow++
			// Clamp column to valid range in new row.
			maxColInRow := min(gridCols, len(entries)-m.agentsGridRow*gridCols) - 1
			if maxColInRow < 0 {
				maxColInRow = 0
			}
			if m.agentsGridCol > maxColInRow {
				m.agentsGridCol = maxColInRow
			}
		}
		return m
	}
	if m.feedFocused {
		if m.feedMarkMode == MarkModeActive {
			m = m.feedToggleMark(m.feedCursor)
		}
		total := m.feedLineCount()
		m.feedCursor = min(m.feedCursor+1, max(0, total-1))
		// Keep cursor in view: scroll down only if cursor moves below the bottom edge.
		visibleH := m.feedVisibleHeight()
		if m.feedCursor >= m.feedScrollOffset+visibleH {
			m.feedScrollOffset = m.feedCursor - visibleH + 1
		}
		m.clampFeedScroll()
		if newAF, _ := m.activityFeed.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}}); newAF != nil {
			if af, ok := newAF.(activity.Model); ok {
				m.activityFeed = af
			}
		}
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
		newPicker, _ := m.agent.agentPicker.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
		m.agent.agentPicker = newPicker
	}
	return m
}

func (m Model) handleUp() Model {
	if m.agentsCenterFocused {
		if m.agentsGridRow > 0 {
			m.agentsGridRow--
			// Clamp column to valid range in new row.
			entries := fuzzyFeed(m.signalBoard.query, m.filteredFeed())
			gridCols := max(1, m.midColWidth()/24)
			maxColInRow := min(gridCols, len(entries)-m.agentsGridRow*gridCols) - 1
			if maxColInRow < 0 {
				maxColInRow = 0
			}
			if m.agentsGridCol > maxColInRow {
				m.agentsGridCol = maxColInRow
			}
		}
		return m
	}
	if m.feedFocused {
		if m.feedMarkMode == MarkModeActive {
			m = m.feedToggleMark(m.feedCursor)
		}
		m.feedCursor = max(m.feedCursor-1, 0)
		// Keep cursor in view: scroll up only if cursor moves above the top edge.
		if m.feedCursor < m.feedScrollOffset {
			m.feedScrollOffset = m.feedCursor
		}
		m.clampFeedScroll()
		if newAF, _ := m.activityFeed.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}}); newAF != nil {
			if af, ok := newAF.(activity.Model); ok {
				m.activityFeed = af
			}
		}
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
		newPicker, _ := m.agent.agentPicker.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
		m.agent.agentPicker = newPicker
	}
	return m
}

// agentPromptsLoadedMsg carries the result of loading saved prompts for the agent modal.
type agentPromptsLoadedMsg struct{ prompts []store.Prompt }

// loadAgentPromptsCmd loads all saved prompts from the store asynchronously.
// Used when the agent modal opens so the user can pick a saved prompt.
func loadAgentPromptsCmd(st *store.Store) tea.Cmd {
	if st == nil {
		return nil
	}
	return func() tea.Msg {
		prompts, _ := st.ListPrompts(context.Background())
		return agentPromptsLoadedMsg{prompts: prompts}
	}
}

// agentModalNextFocus advances the agent modal focus slot forward.
// Focus slots: 0=picker(provider+model), 1=saved-prompt, 2=prompt, 3=use_brain, 4=cwd, 5=schedule, 6=name.
func agentModalNextFocus(cur int) int {
	switch cur {
	case 0:
		return 1
	case 1:
		return 2
	case 2:
		return 3
	case 3:
		return 4
	case 4:
		return 5
	case 5:
		return 6
	default: // 6 or anything else
		return 0
	}
}

// agentModalPrevFocus advances the agent modal focus slot backward.
func agentModalPrevFocus(cur int) int {
	switch cur {
	case 0:
		return 6
	case 1:
		return 0
	case 2:
		return 1
	case 3:
		return 2
	case 4:
		return 3
	case 5:
		return 4
	default: // 6
		return 5
	}
}

// handleAgentModal routes key events when the agent modal overlay is open.
func (m Model) handleAgentModal(msg tea.KeyMsg) (Model, tea.Cmd) {
	key := msg.String()

	// Saved prompt picker captures all keys when open.
	if m.savedPromptPicker.IsOpen() {
		newPicker, ev, cmd := m.savedPromptPicker.Update(msg)
		m.savedPromptPicker = newPicker
		switch ev {
		case modal.FuzzyPickerConfirmed:
			m.agentPromptIdx = m.savedPromptPicker.SelectedOriginalIdx()
			// Advance focus to the prompt textarea so the user can add context.
			m.agentModalFocus = 2
			m.agent.prompt.Focus()
		case modal.FuzzyPickerCancelled:
			// picker closed, agentPromptIdx unchanged
		}
		return m, cmd
	}

	switch key {
	case "esc", "ctrl+c":
		m.agentModalOpen = false
		m.agent.prompt.Blur()
		m.agentSchedule.Blur()
		m.agentNameInput.Blur()
		m.agentScheduleErr = ""
		return m, nil

	case "ctrl+s":
		return m.submitAgentJob()

	case "tab":
		if m.agentModalFocus == 0 {
			// If picker's internal focus is on model list, tab advances outer focus to saved-prompt.
			if m.agent.agentPicker.Focus() == 1 {
				m.agentModalFocus = 1
				m.agent.prompt.Blur()
				m.agentSchedule.Blur()
				return m, nil
			}
			// Otherwise route tab to picker (switches provider↔model internally).
			newPicker, _ := m.agent.agentPicker.Update(msg)
			m.agent.agentPicker = newPicker
			return m, nil
		}
		m.agentModalFocus = agentModalNextFocus(m.agentModalFocus)
		switch m.agentModalFocus {
		case 2:
			m.agent.prompt.Focus()
			m.agentSchedule.Blur()
			m.agentNameInput.Blur()
		case 5:
			m.agent.prompt.Blur()
			m.agentSchedule.Focus()
			m.agentNameInput.Blur()
		case 6:
			m.agent.prompt.Blur()
			m.agentSchedule.Blur()
			m.agentNameInput.Focus()
		default:
			m.agent.prompt.Blur()
			m.agentSchedule.Blur()
			m.agentNameInput.Blur()
		}
		return m, nil

	case "shift+tab":
		m.agentModalFocus = agentModalPrevFocus(m.agentModalFocus)
		switch m.agentModalFocus {
		case 2:
			m.agent.prompt.Focus()
			m.agentSchedule.Blur()
			m.agentNameInput.Blur()
		case 5:
			m.agent.prompt.Blur()
			m.agentSchedule.Focus()
			m.agentNameInput.Blur()
		case 6:
			m.agent.prompt.Blur()
			m.agentSchedule.Blur()
			m.agentNameInput.Focus()
		default:
			m.agent.prompt.Blur()
			m.agentSchedule.Blur()
			m.agentNameInput.Blur()
		}
		return m, nil

	case "up", "k":
		if m.agentModalFocus == 2 || m.agentModalFocus == 5 || m.agentModalFocus == 6 {
			// Let textarea/textinput handle the key when prompt, schedule, or name is focused.
			break
		}
		if m.agentModalFocus == 0 {
			newPicker, _ := m.agent.agentPicker.Update(msg)
			m.agent.agentPicker = newPicker
			return m, nil
		}
		return m, nil

	case "down", "j":
		if m.agentModalFocus == 2 || m.agentModalFocus == 5 || m.agentModalFocus == 6 {
			// Let textarea/textinput handle the key when prompt, schedule, or name is focused.
			break
		}
		if m.agentModalFocus == 0 {
			newPicker, _ := m.agent.agentPicker.Update(msg)
			m.agent.agentPicker = newPicker
			return m, nil
		}
		return m, nil

	case "enter":
		if m.agentModalFocus == 0 {
			newPicker, ev := m.agent.agentPicker.Update(msg)
			m.agent.agentPicker = newPicker
			if ev == modal.AgentPickerConfirmed {
				m.agentModalFocus = 2
				m.agent.prompt.Focus()
			}
			return m, nil
		}
		if m.agentModalFocus == 1 {
			// Open inline fuzzy picker for saved prompts.
			titles := make([]string, 0, len(m.agentPrompts)+1)
			titles = append(titles, "(none)")
			for _, p := range m.agentPrompts {
				titles = append(titles, p.Title)
			}
			m.savedPromptPicker.Open(titles)
			return m, nil
		}
		// fall through to default handling below

	default:
	}

	// USE BRAIN toggle: space or enter toggles the flag.
	if m.agentModalFocus == 3 {
		if key == " " || key == "enter" {
			m.agentUseBrain = !m.agentUseBrain
		}
		return m, nil
	}

	// CWD focus: enter opens the dir picker.
	if m.agentModalFocus == 4 {
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
	if m.agentModalFocus == 5 {
		var cmd tea.Cmd
		m.agentSchedule, cmd = m.agentSchedule.Update(msg)
		return m, cmd
	}
	if m.agentModalFocus == 6 {
		if key == "enter" {
			// Enter in the name field submits the job.
			return m.submitAgentJob()
		}
		var cmd tea.Cmd
		m.agentNameInput, cmd = m.agentNameInput.Update(msg)
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

// handleNewPipeline opens the full-screen pipeline editor for a new pipeline.
func (m Model) handleNewPipeline() (Model, tea.Cmd) {
	m.pipelineEditor.SetProviders(m.agent.providers)
	m.pipelineEditor.SetPalette(m.ansiPalette())
	m.pipelineEditor.SetSize(m.width, m.height-1)
	updated := m.pipelineEditor.OpenNew()
	m.pipelineEditor = updated
	m.pipelineEditorOpen = true
	return m, nil
}

// handleEditPipeline opens the full-screen pipeline editor for the selected pipeline.
func (m Model) handleEditPipeline() (Model, tea.Cmd) {
	if len(m.launcher.pipelines) == 0 {
		return m, nil
	}
	m.pipelineEditor.SetProviders(m.agent.providers)
	m.pipelineEditor.SetPalette(m.ansiPalette())
	m.pipelineEditor.SetSize(m.width, m.height-1)
	name := m.launcher.pipelines[m.launcher.selected]
	updated := m.pipelineEditor.OpenEdit(name)
	m.pipelineEditor = updated
	m.pipelineEditorOpen = true
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
	if m.feedFocused {
		return m, nil
	}

	// Agent runner panel: open the agent runner modal.
	if m.agent.focused {
		m.agentModalOpen = true
		m.agentModalFocus = 0
		m.agentNameInput.SetValue(fmt.Sprintf("agent-%s-%d", m.agent.agentPicker.SelectedProviderID(), time.Now().Unix()))
		return m, loadAgentPromptsCmd(m.store)
	}

	// Agents grid: open inbox detail for the selected agent's run.
	if m.agentsCenterFocused {
		entries := fuzzyFeed(m.signalBoard.query, m.filteredFeed())
		gridCols := max(1, m.midColWidth()/24)
		idx := m.agentsGridRow*gridCols + m.agentsGridCol
		if idx < len(entries) {
			entry := entries[idx]
			// Feed entries for pipeline runs have ID "run-<storeRunID>".
			if runID := strings.TrimPrefix(entry.id, "run-"); runID != entry.id {
				m.agentsCenterFocused = false
				return m.navigateToInboxRun(runID)
			}
		}
		return m, nil
	}

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
				title:  "max parallel jobs reached (8)",
				status: FeedFailed,
				ts:     time.Now(),
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
			m.agentModalFocus = 0 // start at picker (provider+model) so user confirms selection
			m.agentPromptIdx = 0
			m.agent.prompt.Blur()
			m.agentNameInput.SetValue(fmt.Sprintf("agent-%s-%d", m.agent.agentPicker.SelectedProviderID(), time.Now().Unix()))
			return m, loadAgentPromptsCmd(m.store)
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
	jh := &jobHandle{
		id: feedID, cancel: cancel, ch: ch,
		tmuxWindow: windowName, logFile: logFile, pipelineName: name,
		actAgent: m.pendingActAgent, actLabel: m.pendingActLabel,
		executorID: m.pendingExecutorID, modelID: m.pendingModelID,
	}
	m.pendingActAgent = ""
	m.pendingActLabel = ""
	m.pendingExecutorID = ""
	m.pendingModelID = ""
	// Feed entry created by the busd RunStarted event — don't create an eager duplicate here.
	m.activeJobs[feedID] = jh

	startLogWatcher(feedID, logFile, doneFile, ch)
	var startedCmd tea.Cmd
	if jh.actAgent != "" {
		startedCmd = appendActivityCmd(jh.actAgent, jh.actLabel, "agent_run_started", "running")
	}
	return m, tea.Batch(drainChan(ch), startedCmd)
}

// submitAgentJob submits the agent job from the modal overlay.
// If SCHEDULE is non-blank, it writes a cron entry instead of launching immediately.
func (m Model) submitAgentJob() (Model, tea.Cmd) {
	input := strings.TrimSpace(m.agent.prompt.Value())
	// Merge saved prompt + additional context. Either one is sufficient.
	if m.agentPromptIdx > 0 && m.agentPromptIdx-1 < len(m.agentPrompts) {
		p := m.agentPrompts[m.agentPromptIdx-1]
		if input != "" {
			input = p.Body + "\n\n" + input
		} else {
			input = p.Body
		}
	}
	if input == "" {
		return m, nil
	}

	provID := m.agent.agentPicker.SelectedProviderID()
	modelID := m.agent.agentPicker.SelectedModelID()

	// Find the ProviderDef for pipeline args.
	var prov *picker.ProviderDef
	for i := range m.agent.providers {
		if m.agent.providers[i].ID == provID {
			prov = &m.agent.providers[i]
			break
		}
	}
	if prov == nil {
		return m, nil
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

		entryName := strings.TrimSpace(m.agentNameInput.Value())
		if entryName == "" {
			entryName = fmt.Sprintf("agent-%s-%d", prov.ID, time.Now().UnixNano())
		}

		// Generate a minimal single-step pipeline YAML so the scheduled run
		// has the prompt embedded and runs via the standard pipeline executor.
		pipelineFile, pipelineErr := WriteSingleStepPipeline(entryName, prov.ID, modelID, input, m.agentUseBrain)
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
		m.agentNameInput.SetValue("")
		m.agentNameInput.Blur()
		m.agentPromptIdx = 0
		return m, nil
	}

	// ── Run-now path ──────────────────────────────────────────────────────────

	// Enforce parallel job cap.
	if len(m.activeJobs) >= maxParallelJobs {
		feedID := fmt.Sprintf("warn-%d", time.Now().UnixNano())
		warnEntry := feedEntry{
			id:     feedID,
			title:  "max parallel jobs reached (8)",
			status: FeedFailed,
			ts:     time.Now(),
		}
		m.feed = append([]feedEntry{warnEntry}, m.feed...)
		if len(m.feed) > 200 {
			m.feed = m.feed[:200]
		}
		return m, nil
	}

	entryName := strings.TrimSpace(m.agentNameInput.Value())
	if entryName == "" {
		entryName = fmt.Sprintf("agent-%s-%d", prov.ID, time.Now().UnixNano())
	}

	// Resolve CWD and optionally create a git worktree for isolation.
	cwd := m.agentCWD
	if cwd == "" {
		cwd = m.launchCWD
	}
	if worktreePath, _ := picker.GetOrCreateWorktreeFrom(cwd, entryName); worktreePath != "" {
		picker.CopyDotEnv(cwd, worktreePath)
		cwd = worktreePath
	}

	// Write a single-step pipeline so the run goes through the pipeline
	// runner, which records step prompts, outputs, and durations in the store.
	pipelineFile, pipelineErr := WriteSingleStepPipeline(entryName, prov.ID, modelID, input, m.agentUseBrain)
	if pipelineErr != nil {
		feedID := fmt.Sprintf("warn-%d", time.Now().UnixNano())
		warnEntry := feedEntry{
			id:     feedID,
			title:  "warning",
			status: FeedFailed,
			ts:     time.Now(),
			lines:  []string{"pipeline write error: " + pipelineErr.Error()},
		}
		m.feed = append([]feedEntry{warnEntry}, m.feed...)
		if len(m.feed) > 200 {
			m.feed = m.feed[:200]
		}
		return m, nil
	}

	m.pendingPipelineName = entryName
	m.pendingPipelineYAML = pipelineFile
	m.pendingActAgent = agentName
	m.pendingActLabel = activityLabel(input)
	m.pendingExecutorID = prov.ID
	m.pendingModelID = modelID

	// Close modal and reset prompt after submission.
	m.agentModalOpen = false
	m.agent.prompt.SetValue("")
	m.agent.prompt.Blur()
	m.agentSchedule.SetValue("")
	m.agentSchedule.Blur()
	m.agentScheduleErr = ""
	m.agentNameInput.SetValue("")
	m.agentNameInput.Blur()
	m.agentPromptIdx = 0

	return m.launchPendingPipeline(cwd)
}

// submitRerun handles a confirmed re-run from the RerunModal.
// Agent runs are reconstructed as a new single-step pipeline with the (optionally
// appended) original prompt. Pipeline runs re-launch the original YAML file.
func (m Model) submitRerun(msg modal.RerunConfirmedMsg) (Model, tea.Cmd) {
	run := msg.Run
	additionalContext := msg.AdditionalContext

	var meta struct {
		PipelineFile string `json:"pipeline_file"`
		CWD          string `json:"cwd"`
	}
	_ = json.Unmarshal([]byte(run.Metadata), &meta)
	cwd := msg.CWD
	if cwd == "" {
		cwd = m.launchCWD
	}

	switch run.Kind {
	case "agent":
		// Build the prompt: original from the first step, with any additional context appended.
		originalPrompt := ""
		if len(run.Steps) > 0 {
			originalPrompt = run.Steps[0].Prompt
		}
		fullPrompt := originalPrompt
		if additionalContext != "" {
			if fullPrompt != "" {
				fullPrompt += "\n\n---\nAdditional context:\n" + additionalContext
			} else {
				fullPrompt = additionalContext
			}
		}
		if fullPrompt == "" {
			return m, nil
		}

		entryName := fmt.Sprintf("agent-%s-%d", msg.ProviderID, time.Now().UnixNano())
		pipelineFile, err := WriteSingleStepPipeline(entryName, msg.ProviderID, msg.ModelID, fullPrompt, false)
		if err != nil {
			return m, nil
		}
		m.pendingPipelineName = entryName
		m.pendingPipelineYAML = pipelineFile
		m.pendingActAgent = msg.ProviderID + "/" + msg.ModelID
		m.pendingActLabel = activityLabel(fullPrompt)
		return m.launchPendingPipeline(cwd)

	default: // "pipeline" and any future kinds
		// Prefer the stored path; fall back to the conventional location derived
		// from the run name (pipeline runner only stores cwd, not the file path).
		// Resolve YAML path: prefer stored metadata, then root pipelines dir,
		// then .agents/ subdir (agent-panel runs stored as "pipeline" kind).
		yamlPath := meta.PipelineFile
		if yamlPath == "" {
			root := filepath.Join(pipelinesDir(), run.Name+".pipeline.yaml")
			agents := filepath.Join(pipelinesDir(), ".agents", run.Name+".pipeline.yaml")
			if _, err := os.Stat(root); err == nil {
				yamlPath = root
			} else {
				yamlPath = agents
			}
		}
		if _, err := os.Stat(yamlPath); err != nil {
			m = m.prependFeedEntry(feedEntry{
				id:     fmt.Sprintf("err-%d", time.Now().UnixNano()),
				title:  "rerun: pipeline file not found: " + run.Name,
				status: FeedFailed,
				ts:     time.Now(),
			})
			return m, nil
		}
		feedID := fmt.Sprintf("pipe-%d", time.Now().UnixNano())
		orcaiBin := orcaiBinaryPath()
		shellCmd := orcaiBin + " pipeline run " + yamlPath
		if additionalContext != "" {
			// Pass additional context as an env var the pipeline can reference.
			escaped := strings.ReplaceAll(additionalContext, "'", `'\''`)
			shellCmd = "ORCAI_CONTEXT='" + escaped + "' " + shellCmd
		}
		windowName, logFile, doneFile := createJobWindow(feedID, shellCmd, run.Name, cwd)
		ch := make(chan tea.Msg, 256)
		_, cancel := context.WithCancel(context.Background())
		jh := &jobHandle{id: feedID, cancel: cancel, ch: ch, tmuxWindow: windowName, logFile: logFile, pipelineName: run.Name}
		m.activeJobs[feedID] = jh
		startLogWatcher(feedID, logFile, doneFile, ch)
		return m, drainChan(ch)
	}
}

// agentRunModelSlug reads the YAML for an agent run and returns the
// "providerID/modelID" slug used to pre-seed the RerunModal picker.
// Returns "" when the file is missing or the executor field is absent.
func agentRunModelSlug(runName string) string {
	yamlPath := filepath.Join(pipelinesDir(), ".agents", runName+".pipeline.yaml")
	data, err := os.ReadFile(yamlPath)
	if err != nil {
		return ""
	}
	var executor, model string
	for _, line := range strings.Split(string(data), "\n") {
		trimmed := strings.TrimSpace(line)
		if after, ok := strings.CutPrefix(trimmed, "executor: "); ok {
			executor = after
		} else if after, ok := strings.CutPrefix(trimmed, "model: "); ok {
			model = after
		}
	}
	if executor == "" {
		return ""
	}
	if model == "" {
		return executor
	}
	return executor + "/" + model
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
			break
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
// Uses the same height formula as View() via signalBoardPanelHeight to avoid
// scroll-bound drift that lets the cursor escape the visible viewport.
func (m *Model) clampFeedScroll() {
	h := m.height
	if h <= 0 {
		h = 40
	}
	contentH := max(h-2, 5) // match View(): reserve 1 for topBar + 1 for padding
	sbHeight := m.signalBoardPanelHeight(contentH)
	feedH := max(contentH-sbHeight, 3)
	visibleH := feedH - 2
	if visibleH <= 0 {
		visibleH = 1
	}
	total := m.feedLineCount()
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
			if e.status == FeedRunning || e.status == FeedPaused {
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

// ── View ──────────────────────────────────────────────────────────────────────

// View renders the full-screen switchboard layout (three-column).
//
// Left column (30%): pipeline launcher, inbox, cron.
// Center column (remainder): agents grid (top) + agent runner (bottom).
// Right column (25%): activity feed.
//
// Below 80 chars total width, the right column is hidden (two-column fallback).
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
	contentH := max(h-2, 5) // reserve 1 line for top bar + 1 for padding row

	left := m.viewLeftColumn(contentH, leftW)
	leftLines := strings.Split(left, "\n")

	// topBar includes the padding row so all join sites below are consistent.
	topBar := m.viewTopBar(w) + "\n"

	buildBody := func() string {
		if w < 80 {
			// Narrow terminal: two-column fallback (left + center only).
			midW := m.midColWidth()
			center := m.viewCenterColumn(contentH, midW)
			totalRows := max(len(leftLines), len(center))
			var rows []string
			for i := range totalRows {
				l := ""
				if i < len(leftLines) {
					l = leftLines[i]
				}
				c := ""
				if i < len(center) {
					c = center[i]
				}
				rows = append(rows, padToVis(l, leftW)+"  "+c)
			}
			return strings.Join(rows, "\n")
		}

		midW := m.midColWidth()
		rightW := m.rightColWidth()

		center := m.viewCenterColumn(contentH, midW)
		feed := m.viewActivityFeed(contentH, rightW)
		feedLines := strings.Split(feed, "\n")

		totalRows := max(len(leftLines), len(center), len(feedLines))
		var rows []string
		for i := range totalRows {
			l := ""
			if i < len(leftLines) {
				l = leftLines[i]
			}
			c := ""
			if i < len(center) {
				c = center[i]
			}
			f := ""
			if i < len(feedLines) {
				f = feedLines[i]
			}
			rows = append(rows, padToVis(l, leftW)+"  "+padToVis(c, midW)+"  "+f)
		}
		return strings.Join(rows, "\n")
	}

	body := buildBody()

	// Dir picker overlay — used for pipeline context only.
	// The agent context renders the dir picker inline within the agent modal.
	if m.dirPickerOpen && m.dirPickerCtx != "agent" {
		base := topBar + "\n" + body
		return overlayCenter(base, m.dirPicker.ViewDirPickerBox(w, m.ansiPalette()), w, h)
	}

	if m.jumpOpen {
		base := topBar + "\n" + body
		m.jumpModal.SetSize(w, h-2)
		return overlayCenter(base, m.jumpModal.View(), w, h)
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

	// Pipeline editor — full-screen two-column panel.
	if m.pipelineEditorOpen {
		return m.pipelineEditor.View(w, h)
	}

	// Agent modal — full-screen panel like inbox detail.
	if m.agentModalOpen {
		return topBar + "\n" + m.viewAgentModalBox(w, h)
	}

	// Re-run modal — full-screen panel like inbox detail.
	if m.showRerun {
		return topBar + "\n" + m.rerunModal.ViewBox(w, h, m.ansiPalette())
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
	modalW := w

	// Full-screen height: topBar takes 1 row; box top+hints+bot = 3.
	// Fixed overhead: boxTop(1)+pickerViewRows(12:provHdr+4prov+blank+modelHdr+4model+blank)+
	// blank(1)+savedPromptRow(1)+blank(1)+promptHdr(1)+blank(1)+useBrain(1)+blank(1)+cwdHdr(1)+cwd(1)+
	// blank(1)+schedHdr(1)+schedTA(1)+3cronHints(3)+blank(1)+hintStr(1)+boxBot(1) = 32
	const fixedOverhead = 32
	const minPromptH = 4
	modalH := max(h-1, 24)
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

	// ── PROVIDER + MODEL picker ───────────────────────────────────────────────
	for _, r := range m.agent.agentPicker.ViewRows(modalW-4, pal) {
		rows = append(rows, boxRow("  "+r, modalW, modalBorderColor))
	}

	// ── SAVED PROMPT picker ───────────────────────────────────────────────────
	rows = append(rows, boxRow("", modalW, modalBorderColor))
	savedPromptLabel := "(none)"
	if m.agentPromptIdx > 0 && m.agentPromptIdx-1 < len(m.agentPrompts) {
		savedPromptLabel = m.agentPrompts[m.agentPromptIdx-1].Title
	}
	spActive := m.agentModalFocus == 1
	spLabelColor := pal.Dim
	if spActive {
		spLabelColor = pal.Accent + aBld
	}
	enterHint := pal.Dim + "  [enter to pick]" + aRst
	savedPromptRow := "  " + spLabelColor + "Saved Prompt" + aRst + "  " + pal.FG + savedPromptLabel + aRst + enterHint
	rows = append(rows, boxRow(savedPromptRow, modalW, modalBorderColor))

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

	// ── USE BRAIN toggle ──────────────────────────────────────────────────────
	rows = append(rows, boxRow("", modalW, modalBorderColor))
	useBrainCheck := "[ ]"
	if m.agentUseBrain {
		useBrainCheck = "[x]"
	}
	useBrainColor := pal.Dim
	if m.agentModalFocus == 3 {
		useBrainColor = pal.Accent + aBld
	}
	useBrainRow := "  " + useBrainColor + useBrainCheck + " use brain context" + aRst
	rows = append(rows, boxRow(useBrainRow, modalW, modalBorderColor))

	// ── WORKING DIRECTORY section ─────────────────────────────────────────────
	rows = append(rows, boxRow("", modalW, modalBorderColor))
	cwdHeader := "  " + sectionLabel("WORKING DIRECTORY", m.agentModalFocus == 4)
	rows = append(rows, boxRow(cwdHeader, modalW, modalBorderColor))
	cwdDisplay := m.agentCWD
	if cwdDisplay == "" {
		cwdDisplay = m.launchCWD
	}
	if cwdDisplay == "" {
		cwdDisplay = "(current directory)"
	}
	cwdColor := pal.Dim
	if m.agentModalFocus == 4 {
		cwdColor = pal.Accent
	}
	rows = append(rows, boxRow("  "+cwdColor+cwdDisplay+aRst, modalW, modalBorderColor))
	if m.agentModalFocus == 4 && !m.dirPickerOpen {
		rows = append(rows, boxRow(aDim+"  press enter to browse"+aRst, modalW, modalBorderColor))
	}

	// ── SCHEDULE section ──────────────────────────────────────────────────────
	rows = append(rows, boxRow("", modalW, modalBorderColor))
	schedHeader := "  " + sectionLabel("SCHEDULE (cron expr, blank = run now)", m.agentModalFocus == 5)
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

	// ── NAME section ─────────────────────────────────────────────────────────
	rows = append(rows, boxRow("", modalW, modalBorderColor))
	nameHeader := "  " + sectionLabel("SESSION NAME", m.agentModalFocus == 6)
	rows = append(rows, boxRow(nameHeader, modalW, modalBorderColor))
	m.agentNameInput.Width = max(modalW-6, 10)
	nameView := "  " + m.agentNameInput.View()
	rows = append(rows, boxRow(nameView, modalW, modalBorderColor))

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

	base := strings.Join(rows, "\n")
	modalH = len(rows)

	// Saved prompt picker — floating overlay.
	if m.savedPromptPicker.IsOpen() {
		overlay := m.savedPromptPicker.ViewBox(modalW, pal)
		return panelrender.OverlayCenter(base, overlay, modalW, modalH)
	}

	// Dir picker — floating overlay for agent context.
	if m.dirPickerOpen && m.dirPickerCtx == "agent" {
		overlay := m.dirPicker.ViewDirPickerBox(modalW, pal)
		return panelrender.OverlayCenter(base, overlay, modalW, modalH)
	}

	return base
}

// sectionLabel returns a section header with focus indicator.
func sectionLabel(title string, focused bool) string {
	if focused {
		return aBrC + aBld + title + aRst
	}
	return aDim + title + aRst
}

// leftColWidth returns the width of the left column (30%, min 28).
// In the three-column layout, the left column holds the pipeline launcher,
// inbox, and cron panels (no agent runner).
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

// rightColWidth returns the width of the right (activity feed) column (25%, min 20).
func (m Model) rightColWidth() int {
	w := m.width
	if w <= 0 {
		w = 120
	}
	rw := w * 25 / 100
	if rw < 20 {
		rw = 20
	}
	return rw
}

// midColWidth returns the width of the center (agents grid + agent runner) column.
// Two 2-char gutters separate the three columns, so midW = w - leftW - rightW - 4.
func (m Model) midColWidth() int {
	w := m.width
	if w <= 0 {
		w = 120
	}
	lw := m.leftColWidth()
	rw := m.rightColWidth()
	mw := w - lw - rw - 4
	if mw < 10 {
		mw = 10
	}
	return mw
}

// viewLeftColumn renders the left column: banner + launcher + inbox + cron sections.
func (m Model) viewLeftColumn(height, width int) string {
	var lines []string

	launcherLines := m.buildLauncherSection(width)
	lines = append(lines, launcherLines...)

	// Distribute remaining rows between Inbox (60%) and Cron (40%), min 4 each.
	// Agent runner is now in the center column.
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

// viewCenterColumn renders the center column: agents grid (top) + agent runner (bottom).
// The combined output is clamped to height rows.
// The agents grid gets the upper ~60% of available height; agent runner gets the rest.
func (m Model) viewCenterColumn(height, width int) []string {
	// Measure the agent runner height so we can give it exactly the space it needs.
	agentRunnerLines := m.buildAgentSection(width)
	agentRunnerH := len(agentRunnerLines)

	// Grid gets remaining height after agent runner (no separator — panels sit flush).
	gridH := height - agentRunnerH
	if gridH < 6 {
		gridH = 6
	}

	agentGridLines := m.buildAgentsGrid(gridH, width)

	var lines []string
	lines = append(lines, agentGridLines...)
	lines = append(lines, agentRunnerLines...)

	// Pad up to height.
	for len(lines) < height {
		lines = append(lines, "")
	}
	// Clamp to height.
	if len(lines) > height {
		lines = lines[:height]
	}
	return lines
}

// buildAgentsGrid renders the Agents panel as a 2-D grid of boxed agent cards.
// Each card is 3 lines tall (top border, content, bottom border). The selected
// card's border is rendered in accent color; unselected cards use dim border.
// gridCols = max(1, width/24).  Source: filteredFeed() (same as signal board).
func (m Model) buildAgentsGrid(height, width int) []string {
	pal := m.ansiPalette()
	panelBorder := pal.Border
	titleColor := pal.Accent
	if m.agentsCenterFocused {
		panelBorder = pal.Accent
	}

	var lines []string
	if sprite := PanelHeader(m.activeBundle(), "agents", width, panelBorder, titleColor); sprite != nil {
		lines = append(lines, sprite...)
	} else {
		lines = append(lines, boxTop(width, RenderHeader("agents"), panelBorder, titleColor))
	}

	entries := fuzzyFeed(m.signalBoard.query, m.filteredFeed())

	// Column count: at least 1, one card per ~24 cols of inner panel width.
	innerW := width - 2 // panel body (between outer │ chars)
	gridCols := max(1, innerW/24)

	// Card width: divide inner width evenly; gap of 1 between cards, 1 margin each side.
	// innerW = 2 (margins) + gridCols*cardW + (gridCols-1) (gaps)
	cardW := max((innerW-2-(gridCols-1))/gridCols, 10)

	// Recompute cols so cards actually fit (may shrink if innerW was narrow).
	for gridCols > 1 && 2+(gridCols*cardW)+(gridCols-1) > innerW {
		gridCols--
	}

	// Grid dimensions.
	gridRows := (len(entries) + gridCols - 1) / gridCols
	if len(entries) == 0 {
		gridRows = 0
	}

	// Clamp cursor.
	selRow := m.agentsGridRow
	selCol := m.agentsGridCol
	if gridRows > 0 {
		if selRow >= gridRows {
			selRow = gridRows - 1
		}
		maxColInRow := min(gridCols, len(entries)-selRow*gridCols) - 1
		if selCol > maxColInRow {
			selCol = max(0, maxColInRow)
		}
	}

	headerH := len(lines)
	bodyH := height - headerH - 2 // reserve hint footer + boxBot
	if bodyH < 1 {
		bodyH = 1
	}
	// Each card row occupies 3 panel lines (top, content, bottom).
	maxCardRows := bodyH / 3

	if len(entries) == 0 {
		lines = append(lines, boxRow(pal.Dim+"  no agents"+aRst, width, panelBorder))
	} else {
		for row := 0; row < gridRows && row < maxCardRows; row++ {
			// Accumulate 3 strings: top borders, content lines, bottom borders.
			var topRow, midRow, botRow strings.Builder

			// Leading margin (1 space).
			topRow.WriteByte(' ')
			midRow.WriteByte(' ')
			botRow.WriteByte(' ')

			for col := 0; col < gridCols; col++ {
				// Inter-card gap (skip for first column).
				if col > 0 {
					topRow.WriteByte(' ')
					midRow.WriteByte(' ')
					botRow.WriteByte(' ')
				}

				idx := row*gridCols + col
				if idx >= len(entries) {
					// Empty placeholder card (blank space same width).
					topRow.WriteString(strings.Repeat(" ", cardW))
					midRow.WriteString(strings.Repeat(" ", cardW))
					botRow.WriteString(strings.Repeat(" ", cardW))
					continue
				}

				e := entries[idx]
				isSelected := row == selRow && col == selCol && m.agentsCenterFocused

				// Card border color: accent when selected, dim otherwise.
				cBorder := pal.Dim
				if isSelected {
					cBorder = pal.Accent
				}

				// LED glyph + status label.
				var led, statusLabel string
				switch e.status {
				case FeedRunning:
					if m.signalBoard.blinkOn {
						led = pal.Accent + "●" + aRst
					} else {
						led = pal.Dim + "●" + aRst
					}
					statusLabel = pal.Accent + "run" + aRst
				case FeedPaused:
					led = pal.Warn + "?" + aRst
					statusLabel = pal.Warn + "paused" + aRst
				case FeedDone:
					led = pal.Success + "✓" + aRst
					statusLabel = pal.Success + "done" + aRst
				case FeedFailed:
					led = pal.Error + "✗" + aRst
					statusLabel = pal.Error + "fail" + aRst
				default:
					led = pal.Dim + "●" + aRst
					statusLabel = pal.Dim + "idle" + aRst
				}

				statusVis := len(stripANSI(statusLabel))
				// cardW = 2 borders + innerCardW; content: " led title…pad status "
				innerCardW := cardW - 2
				// Fixed visible chars: 1 (left space) + 1 (led) + 1 (space) + 1 (right space) + statusVis = 4 + statusVis
				maxTitleVis := max(innerCardW-4-statusVis, 1)
				title := e.title
				titleRunes := []rune(title)
				if len(titleRunes) > maxTitleVis {
					title = string(titleRunes[:maxTitleVis-1]) + "…"
				}
				titleVis := len([]rune(title))
				padLen := max(innerCardW-1-1-1-titleVis-statusVis-1, 0)

				// Top border: ╭─────────────╮
				topRow.WriteString(cBorder + "╭" + strings.Repeat("─", innerCardW) + "╮" + aRst)

				// Content line: │ led title   status │
				midRow.WriteString(cBorder + "│" + aRst)
				midRow.WriteString(" " + led + " " + pal.FG + title + aRst)
				midRow.WriteString(strings.Repeat(" ", padLen))
				midRow.WriteString(statusLabel)
				midRow.WriteString(" " + cBorder + "│" + aRst)

				// Bottom border: ╰─────────────╯
				botRow.WriteString(cBorder + "╰" + strings.Repeat("─", innerCardW) + "╯" + aRst)
			}

			lines = append(lines, boxRow(topRow.String(), width, panelBorder))
			lines = append(lines, boxRow(midRow.String(), width, panelBorder))
			lines = append(lines, boxRow(botRow.String(), width, panelBorder))
		}
	}

	// Pad body to fill available height.
	for len(lines) < height-2 {
		lines = append(lines, boxRow("", width, panelBorder))
	}

	// Hint footer.
	var gridHints []panelrender.Hint
	if m.agentsCenterFocused {
		if m.signalBoard.searching {
			gridHints = []panelrender.Hint{{Key: "esc", Desc: "cancel"}}
		} else {
			gridHints = []panelrender.Hint{
				{Key: "h/j/k/l", Desc: "nav"},
				{Key: "/", Desc: "search"},
				{Key: "enter", Desc: "open"},
			}
		}
	}
	lines = append(lines, boxRow(panelrender.HintBar(gridHints, width-2, pal), width, panelBorder))
	lines = append(lines, boxBot(width, panelBorder))

	if len(lines) > height {
		lines = lines[:height]
	}
	return lines
}

// BuildAgentsGrid is an exported wrapper for tests.
func (m Model) BuildAgentsGrid(height, width int) []string { return m.buildAgentsGrid(height, width) }

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

// viewTopBar renders the full-width ORCAI header bar for the Switchboard.
func (m Model) viewTopBar(w int) string {
	title := "░▒▓ ORCAI — ABBS Switchboard ▓▒░"
	if p := translations.GlobalProvider(); p != nil {
		title = p.T(translations.KeySwitchboardHeader, title)
	}
	return TopBar(m.activeBundle(), title, w)
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
		// Render providers as a horizontal row of inline boxed cards (same style as agents grid).
		selectedProvID := m.agent.agentPicker.SelectedProviderID()
		selectedProvIdx := 0
		for i, p := range m.agent.providers {
			if p.ID == selectedProvID {
				selectedProvIdx = i
				break
			}
		}

		innerW := w - 2 // panel body between outer │ chars
		gridCols := max(1, innerW/20)
		cardW := max((innerW-2-(gridCols-1))/gridCols, 8)
		// Shrink cols until cards fit.
		for gridCols > 1 && 2+(gridCols*cardW)+(gridCols-1) > innerW {
			gridCols--
		}

		// Scrolling: keep selected card visible.
		offset := (selectedProvIdx / gridCols) * gridCols
		windowRows := agentInnerHeight / 3
		if windowRows < 1 {
			windowRows = 1
		}
		selectedRow := selectedProvIdx / gridCols
		startRow := offset / gridCols
		if selectedRow >= startRow+windowRows {
			startRow = selectedRow - windowRows + 1
		}
		offset = startRow * gridCols

		end := min(offset+windowRows*gridCols, len(m.agent.providers))
		providers := m.agent.providers[offset:end]

		for rowStart := 0; rowStart < windowRows*gridCols; rowStart += gridCols {
			var topRow, midRow, botRow strings.Builder
			topRow.WriteByte(' ')
			midRow.WriteByte(' ')
			botRow.WriteByte(' ')
			for col := 0; col < gridCols; col++ {
				if col > 0 {
					topRow.WriteByte(' ')
					midRow.WriteByte(' ')
					botRow.WriteByte(' ')
				}
				idx := offset + rowStart + col
				if rowStart+col >= len(providers) || idx >= len(m.agent.providers) {
					topRow.WriteString(strings.Repeat(" ", cardW))
					midRow.WriteString(strings.Repeat(" ", cardW))
					botRow.WriteString(strings.Repeat(" ", cardW))
					continue
				}
				prov := m.agent.providers[idx]
				isSelected := idx == selectedProvIdx
				cBorder := pal.Dim
				if isSelected && m.agent.focused {
					cBorder = pal.Accent
				} else if isSelected {
					cBorder = pal.FG
				}
				label := prov.Label
				if label == "" {
					label = prov.ID
				}
				innerCardW := cardW - 2
				maxLabelVis := max(innerCardW-2, 1)
				labelRunes := []rune(label)
				if len(labelRunes) > maxLabelVis {
					label = string(labelRunes[:maxLabelVis-1]) + "…"
				}
				labelVis := len([]rune(label))
				padLen := max(innerCardW-2-labelVis, 0)
				labelColor := pal.Dim
				if isSelected {
					labelColor = pal.FG
				}
				topRow.WriteString(cBorder + "╭" + strings.Repeat("─", innerCardW) + "╮" + aRst)
				midRow.WriteString(cBorder + "│" + aRst + " " + labelColor + label + aRst + strings.Repeat(" ", padLen) + " " + cBorder + "│" + aRst)
				botRow.WriteString(cBorder + "╰" + strings.Repeat("─", innerCardW) + "╯" + aRst)
			}
			rows = append(rows, boxRow(topRow.String(), w, borderColor))
			rows = append(rows, boxRow(midRow.String(), w, borderColor))
			rows = append(rows, boxRow(botRow.String(), w, borderColor))
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

// navigateToInboxRun focuses the inbox panel and opens the detail view for the
// run matching runIDStr (decimal store run ID). Returns the updated model and a
// reload command if the run was found.
func (m Model) navigateToInboxRun(runIDStr string) (Model, tea.Cmd) {
	// Switch inbox filter to "all" so running/paused entries are always visible.
	m.inboxPanel.activeFilter = "all"
	m.inboxPanel.filterQuery = ""
	m.inboxPanel.scrollOffset = 0

	runs := m.filteredInboxRuns()
	for i, run := range runs {
		if fmt.Sprintf("%d", run.ID) == runIDStr {
			m.inboxPanel.focused = true
			m.inboxModel.SetFocused(true)
			m.inboxPanel.selectedIdx = i
			m.inboxDetailIdx = i
			m.inboxDetailOpen = true
			m.inboxDetailScroll = 0
			m.inboxDetailCursor = 0
			m.inboxMarkdownMode = false
			// clear other focus
			m.launcher.focused = false
			m.agent.focused = false
			m.feedFocused = false
			m.signalBoardFocused = false
			m.agentsCenterFocused = false
			return m, nil
		}
	}
	return m, nil
}

// filteredInboxRuns returns inbox runs filtered by read state and search query.
func (m Model) filteredInboxRuns() []store.Run {
	all := m.inboxModel.Runs()
	query := strings.ToLower(m.inboxPanel.filterQuery)
	filter := m.inboxPanel.activeFilter
	if filter == "" {
		filter = "unread"
	}
	var out []store.Run
	for _, r := range all {
		isRead := m.inboxReadIDs[r.ID]
		needsAttention := r.ExitStatus != nil && *r.ExitStatus != 0
		switch filter {
		case "unread":
			if isRead {
				continue
			}
		case "read":
			if !isRead {
				continue
			}
		case "attention":
			if !needsAttention {
				continue
			}
		case "all":
			// no exclusion
		default:
			if isRead {
				continue
			}
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

	// Active filter label row (shown whenever filter is not the default "unread").
	activeFilter := m.inboxPanel.activeFilter
	if activeFilter == "" {
		activeFilter = "unread"
	}
	if activeFilter != "unread" {
		label := fmt.Sprintf("  %sfilter:%s %s%s%s", pal.Dim, aRst, pal.Accent, activeFilter, aRst)
		rows = append(rows, boxRow(label, w, borderColor))
		maxRows--
		if maxRows < 0 {
			maxRows = 0
		}
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
			// ⚠ attention marker for failed runs; ? for runs awaiting clarification.
			warn := ""
			warnVis := 0
			// Yellow ? for runs awaiting clarification.
			awaitsClarification := m.pendingClarification != nil &&
				m.pendingClarification.RunID == fmt.Sprintf("%d", run.ID)
			if awaitsClarification {
				warn = " " + aYlw + "?" + aRst
				warnVis = 2
			} else if run.ExitStatus != nil && *run.ExitStatus != 0 {
				warn = " " + aRed + "⚠" + aRst
				warnVis = 2 // " ⚠" = 2 visible columns
			}
			ts := time.UnixMilli(run.StartedAt).Format("1/2 3:04 PM")
			tsVis := len(ts) + 1
			maxNameLen := max(w-7-tsVis-warnVis, 1)
			name := run.Name
			if len(name) > maxNameLen {
				name = name[:maxNameLen-1] + "…"
			}
			inner := w - 2
			focused := m.inboxPanel.focused
			prefixVis := 2 + 1 + 1 + len(name) + warnVis
			pad := max(inner-prefixVis-tsVis, 0)
			dimTS := pal.Dim + strings.Repeat(" ", pad) + ts + aRst
			var content string
			if i == sel && focused {
				content = pal.Accent + "> " + aRst + dot + " " + pal.FG + name + aRst + warn + dimTS
			} else {
				content = "  " + dot + " " + pal.Dim + name + aRst + warn + dimTS
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
			filterLabel := m.inboxPanel.activeFilter
			if filterLabel == "" {
				filterLabel = "unread"
			}
			inboxHints = []panelrender.Hint{
				{Key: "enter", Desc: "open"},
				{Key: "x", Desc: "mark read"},
				{Key: "/", Desc: "search"},
				{Key: "r", Desc: "re-run"},
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
// matching the rightW calculation in View(). Used for consistent line-count math.
func (m Model) feedPanelWidth() int {
	return m.rightColWidth()
}

// feedLineCount returns the total number of navigable content lines in the feed.
// It delegates to feedRawLines which is the single source of truth for line
// count — the same parser pipeline runs in feedRawLines and viewActivityFeed.
func (m Model) feedLineCount() int {
	n := len(m.feedRawLines(m.feedPanelWidth()))
	if n == 0 {
		return 1
	}
	return n
}

// feedRawLines returns ANSI-stripped content strings for every navigable line
// in the feed (no box formatting). The logical line indices produced here must
// match the cursor positions used by viewActivityFeed so that cursor/mark/JSON-
// expand operations reference the correct lines.
func (m Model) feedRawLines(width int) []string {
	pal := m.ansiPalette()
	var lines []string
	logicalIdx := 0
	add := func(content string) {
		lines = append(lines, stripANSI(content))
		logicalIdx++
	}

	if len(m.feed) == 0 {
		add("  no activity yet")
		return lines
	}
	for _, entry := range m.feed {
		entryBadge, badgeColor := statusBadge(entry.status, pal)
		ts := strings.ToLower(entry.ts.Format("3:04 PM"))
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
			// Suppress done steps with no output — matches viewActivityFeed logic.
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
				connector := "├─ "
				if isLast {
					connector = "└─ "
				}
				add("  " + connector + col + stepGlyph(step.status) + " " + step.id + aRst)
				// Step output lines (for JSON expand/collapse and cursor navigation).
				const maxStepOutputLines = 5
				stepLines := step.lines
				if len(stepLines) > maxStepOutputLines {
					stepLines = stepLines[len(stepLines)-maxStepOutputLines:]
				}
				stepMaxLen := max(width-10, 1)
				for _, sl := range stepLines {
					sl = stripANSI(sl)
					if pLines, ok := runFeedLineParsers(sl, stepMaxLen, pal, false); ok {
						add("      " + pLines[0])
					} else {
						runes := []rune(sl)
						if len(runes) > stepMaxLen {
							sl = string(runes[:stepMaxLen])
						}
						if sl != "" {
							add("      " + sl)
						}
					}
				}
			}
		}

		// Top-level output lines for the entry (e.g. plain output or JSON).
		const feedLinesPerEntry = 10
		entryLines := entry.lines
		skipped := 0
		if len(entryLines) > feedLinesPerEntry {
			skipped = len(entryLines) - feedLinesPerEntry
			entryLines = entryLines[skipped:]
		}
		if skipped > 0 {
			add(fmt.Sprintf("    … %d earlier lines", skipped))
		}
		entryMaxLen := max(width-6, 1)
		for _, outLine := range entryLines {
			outLine = stripANSI(outLine)
			if pLines, ok := runFeedLineParsers(outLine, entryMaxLen, pal, false); ok {
				add("    " + pLines[0])
			} else {
				runes := []rune(outLine)
				if len(runes) > entryMaxLen {
					outLine = string(runes[:entryMaxLen])
				}
				if outLine != "" {
					add("    " + outLine)
				}
			}
		}
	}
	return lines
}

// viewActivityFeed renders the activity feed (right column) with full cursor,
// mark, scroll, and hint-bar support.  Content is sourced from the in-memory
// m.feed ring buffer; the JSONL-backed activityFeed model is used only when
// the in-memory feed is empty.
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

	// logicalIdx tracks the cursor-navigable line index — matches feedRawLines indices.
	// logicalToVisual maps each logical line index to its visual row index in allLines.
	logicalIdx := 0
	var logicalToVisual []int
	appendRow := func(lines *[]string, content string) {
		logicalToVisual = append(logicalToVisual, len(*lines))
		idx := logicalIdx
		logicalIdx++
		isMarked := m.feedMarked[idx]
		isCursor := m.feedFocused && idx == m.feedCursor
		var row string
		switch {
		case isCursor && isMarked:
			row = boxRowCursorColor(pal.Success+"●"+aRst+content, width, borderColor)
		case isCursor:
			row = boxRowCursorColor(content, width, borderColor)
		case isMarked:
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
	// appendExtra adds a visual-only box row without advancing logicalIdx.
	appendExtra := func(lines *[]string, content string) {
		*lines = append(*lines, boxRow(content, width, borderColor))
	}

	var allLines []string

	if len(m.feed) == 0 {
		// Fall back to JSONL-backed activity model when in-memory feed is empty.
		actContent := strings.TrimRight(m.activityFeed.View(), "\n")
		if actContent == "" || actContent == aDim+"  no activity yet"+aRst {
			appendRow(&allLines, pal.Dim+"  no activity yet"+aRst)
		} else {
			for _, line := range strings.Split(actContent, "\n") {
				appendRow(&allLines, line)
			}
		}
	} else {
		// Render in-memory feed entries with per-step status badges.
		for _, entry := range m.feed {
			badge, badgeColor := statusBadge(entry.status, pal)
			ts := strings.ToLower(entry.ts.Format("3:04 PM"))
			titleLine := fmt.Sprintf("  %s%s%s %s%s%s  %s",
				badgeColor, badge, aRst,
				pal.Dim, ts, aRst,
				pal.Accent+entry.title+aRst)
			appendRow(&allLines, titleLine)

			if entry.cwd != "" {
				home, _ := os.UserHomeDir()
				cwdDisplay := entry.cwd
				if home != "" && strings.HasPrefix(cwdDisplay, home) {
					cwdDisplay = "~" + cwdDisplay[len(home):]
				}
				appendRow(&allLines, fmt.Sprintf("  %s  %s%s", pal.Dim, cwdDisplay, aRst))
			}

			if len(entry.steps) > 0 {
				const maxStepOutputLines = 5
				// Find last visible step (suppress done steps with no output).
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
					connector := "├─ "
					if isLast {
						connector = "└─ "
					}
					badge := "  " + connector + col + stepGlyph(step.status) + " " + step.id + aRst
					appendRow(&allLines, badge)
					// Per-step output lines (last maxStepOutputLines only).
					stepLines := step.lines
					if len(stepLines) > maxStepOutputLines {
						stepLines = stepLines[len(stepLines)-maxStepOutputLines:]
					}
					var outPrefix string
					if isLast {
						outPrefix = "      "
					} else {
						outPrefix = "  " + pal.Dim + "│   " + aRst
					}
					stepMaxLen := max(width-10, 1)
					for _, sl := range stepLines {
						sl = stripANSI(sl)
						if pLines, ok := runFeedLineParsers(sl, stepMaxLen, pal, false); ok {
							appendRow(&allLines, outPrefix+pLines[0])
							for _, xl := range pLines[1:] {
								appendExtra(&allLines, outPrefix+"  "+xl)
							}
						} else {
							// Truncate long lines to fit within the column.
							runes := []rune(sl)
							if len(runes) > stepMaxLen {
								sl = string(runes[:stepMaxLen])
							}
							if sl != "" {
								appendRow(&allLines, outPrefix+pal.Dim+sl+aRst)
							}
						}
					}
				}
			}

			// Cap top-level output lines per entry.
			const feedLinesPerEntry = 10
			entryLines := entry.lines
			skipped := 0
			if len(entryLines) > feedLinesPerEntry {
				skipped = len(entryLines) - feedLinesPerEntry
				entryLines = entryLines[skipped:]
			}
			if skipped > 0 {
				skipMsg := fmt.Sprintf("    … %d earlier lines", skipped)
				appendRow(&allLines, pal.Dim+skipMsg+aRst)
			}
			entryMaxLen := max(width-6, 1)
			for _, outLine := range entryLines {
				outLine = stripANSI(outLine)
				if pLines, ok := runFeedLineParsers(outLine, entryMaxLen, pal, false); ok {
					appendRow(&allLines, "    "+pLines[0])
					for _, xl := range pLines[1:] {
						appendExtra(&allLines, "      "+xl)
					}
				} else {
					runes := []rune(outLine)
					if len(runes) > entryMaxLen {
						outLine = string(runes[:entryMaxLen])
					}
					if outLine != "" {
						appendRow(&allLines, pal.Dim+"    "+outLine+aRst)
					}
				}
			}
		}
	}

	// Clamp offset and compute visible window using logicalToVisual map.
	offset := 0
	if m.feedScrollOffset < len(logicalToVisual) {
		offset = logicalToVisual[m.feedScrollOffset]
	} else if len(logicalToVisual) > 0 {
		offset = logicalToVisual[len(logicalToVisual)-1]
	}
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

	// Scroll indicators in header.
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
	case FeedPaused:
		return "? awaiting reply", pal.Warn
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
