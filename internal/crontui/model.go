package crontui

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/log"

	"github.com/adam-stokes/orcai/internal/cron"
	"github.com/adam-stokes/orcai/internal/store"
	"github.com/adam-stokes/orcai/internal/themes"
	"github.com/adam-stokes/orcai/internal/tuikit"
)

// Dracula palette
const (
	draculaBg      = "#282a36"
	draculaCurrent = "#44475a"
	draculaFg      = "#f8f8f2"
	draculaComment = "#6272a4"
	draculaCyan    = "#8be9fd"
	draculaGreen   = "#50fa7b"
	draculaOrange  = "#ffb86c"
	draculaPink    = "#ff79c6"
	draculaPurple  = "#bd93f9"
	draculaRed     = "#ff5555"
	draculaYellow  = "#f1fa8c"
)

// Messages
type tickMsg time.Time
type logLineMsg struct{ line string }
type runDoneMsg struct {
	name string
	err  error
}
type entriesReloadedMsg struct{ entries []cron.Entry }

// LogSink implements io.Writer and posts each line as logLineMsg to a channel.
type LogSink struct {
	ch chan tea.Msg
}

// NewLogSink creates a LogSink that writes to the given channel.
func NewLogSink(ch chan tea.Msg) *LogSink {
	return &LogSink{ch: ch}
}

// Write implements io.Writer. Each call posts a logLineMsg to the channel.
func (s *LogSink) Write(p []byte) (int, error) {
	line := string(p)
	// Non-blocking send: drop if buffer is full to avoid deadlock.
	select {
	case s.ch <- logLineMsg{line: line}:
	default:
	}
	return len(p), nil
}

// EditOverlay holds state for the edit form.
type EditOverlay struct {
	fields   [5]textinput.Model // name, schedule, kind, target, timeout
	focusIdx int
	errMsg   string
	original cron.Entry // the entry being edited (needed to remove old name if renamed)
}

// DeleteConfirm holds state for the delete confirmation.
type DeleteConfirm struct {
	entry cron.Entry
}

const logBufMax = 500

// Model is the BubbleTea model for the orcai-cron TUI.
type Model struct {
	scheduler  *cron.Scheduler
	runStore   *store.Store   // result store for recording run history; may be nil
	logger     *log.Logger    // structured logger wired to the log pane
	bundle     *themes.Bundle // active theme; nil = Dracula fallback
	themeState tuikit.ThemeState
	entries    []cron.Entry
	filtered   []cron.Entry // after fuzzy filter
	logBuf     []string     // ring buffer, max logBufMax lines
	logCh      chan tea.Msg  // LogSink writes here

	filterInput textinput.Model
	filtering   bool // true when filter input is active

	selectedIdx     int
	scrollOffset    int
	logScrollOffset int
	activePane      int // 0=jobs, 1=logs

	editOverlay   *EditOverlay
	deleteConfirm *DeleteConfirm
	quitConfirm   bool

	width  int
	height int
}
