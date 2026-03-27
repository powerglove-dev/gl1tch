package crontui

import (
	"context"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/log"

	"github.com/adam-stokes/orcai/internal/cron"
	"github.com/adam-stokes/orcai/internal/store"
	"github.com/adam-stokes/orcai/internal/themes"
)

// New creates the cron TUI model, wiring up the scheduler with a LogSink so
// that all scheduler log output is captured to the in-app log pane.
// bundle may be nil; in that case GlobalRegistry is tried and, if also nil,
// the Dracula palette is used as fallback.
func New(bundle *themes.Bundle) (Model, error) {
	// Fall back to global registry when no explicit bundle is provided.
	if bundle == nil {
		if gr := themes.GlobalRegistry(); gr != nil {
			bundle = gr.Active()
		}
	}

	logCh := make(chan tea.Msg, 256)
	sink := NewLogSink(logCh)
	logger := log.NewWithOptions(sink, log.Options{
		ReportTimestamp: true,
		Prefix:          "orcai-cron",
	})
	// Open the result store so cron runs appear in the switchboard inbox.
	// A failure is non-fatal — the scheduler runs without persistence.
	s, storeErr := store.Open()
	if storeErr != nil {
		logger.Warn("result store unavailable", "error", storeErr)
	}

	sched := cron.New(logger, s)

	entries, _ := cron.LoadConfig()

	fi := textinput.New()
	fi.Placeholder = "/ filter..."
	fi.CharLimit = 64

	// Subscribe to theme changes via the global registry (if available).
	var themeCh chan string
	if gr := themes.GlobalRegistry(); gr != nil {
		themeCh = gr.SafeSubscribe()
	}

	m := Model{
		scheduler:   sched,
		runStore:    s,
		logger:      logger,
		bundle:      bundle,
		entries:     entries,
		filtered:    entries,
		logCh:       logCh,
		filterInput: fi,
		themeCh:     themeCh,
	}
	return m, nil
}

// Init starts the scheduler, begins the 30-second tick, and starts listening
// for log lines and theme changes.
func (m Model) Init() tea.Cmd {
	cmds := []tea.Cmd{
		func() tea.Msg {
			if err := m.scheduler.Start(context.Background()); err != nil {
				m.logger.Error("failed to start scheduler", "err", err)
				return nil
			}
			m.logger.Info("scheduler started")
			return nil
		},
		tick(),
		listenLogs(m.logCh),
	}
	if m.themeCh != nil {
		cmds = append(cmds, listenTheme(m.themeCh))
	}
	return tea.Batch(cmds...)
}

// tick schedules a reload every 30 seconds.
func tick() tea.Cmd {
	return tea.Tick(30*time.Second, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

// listenLogs blocks until a message arrives on ch, then returns it.
// The Update handler re-schedules this after each message so the listener
// stays active for the lifetime of the program.
func listenLogs(ch chan tea.Msg) tea.Cmd {
	return func() tea.Msg {
		return <-ch
	}
}

// listenTheme blocks until a theme name arrives on ch and returns a
// themeChangedMsg. The Update handler re-schedules this to keep listening.
func listenTheme(ch chan string) tea.Cmd {
	return func() tea.Msg {
		return themeChangedMsg{name: <-ch}
	}
}
