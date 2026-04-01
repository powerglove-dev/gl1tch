package crontui

import (
	"context"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/log"

	"github.com/8op-org/gl1tch/internal/cron"
	"github.com/8op-org/gl1tch/internal/store"
	"github.com/8op-org/gl1tch/internal/themes"
	"github.com/8op-org/gl1tch/internal/tuikit"
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
		Prefix:          "glitch-cron",
	})
	// Open the result store so cron runs appear in the switchboard inbox.
	// A failure is non-fatal — the scheduler runs without persistence.
	s, storeErr := store.Open()
	if storeErr != nil {
		logger.Warn("result store unavailable", "error", storeErr)
	}

	sched := cron.New(logger)

	entries, _ := cron.LoadConfig()

	fi := textinput.New()
	fi.Placeholder = "/ filter..."
	fi.CharLimit = 64

	m := Model{
		scheduler:   sched,
		runStore:    s,
		logger:      logger,
		bundle:      bundle,
		themeState:  tuikit.NewThemeState(bundle),
		entries:     entries,
		filtered:    entries,
		logCh:       logCh,
		filterInput: fi,
	}
	return m, nil
}

// Init starts the scheduler, begins the 30-second tick, and starts listening
// for log lines and theme changes.
func (m Model) Init() tea.Cmd {
	return tea.Batch(
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
		m.themeState.Init(),
	)
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


