// Package tuikit provides shared BubbleTea helpers for glitch sub-TUIs.
//
// The primary export is [ThemeState], which any BubbleTea model can embed to
// get correct theme initialisation, live cross-process updates via busd, and
// automatic retry if the daemon is not yet running.
//
// Minimal usage:
//
//	type Model struct {
//	    themeState tuikit.ThemeState
//	    // ...other fields
//	}
//
//	func (m Model) Init() tea.Cmd {
//	    return tea.Batch(m.themeState.Init(), /* other cmds */)
//	}
//
//	func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
//	    if ts, cmd, ok := m.themeState.Handle(msg); ok {
//	        m.themeState = ts
//	        return m, cmd
//	    }
//	    // ... rest of Update
//	}
//
//	func (m Model) View() string {
//	    bundle := m.themeState.Bundle()
//	    // use bundle for rendering
//	}
package tuikit

import (
	"bufio"
	"encoding/json"
	"net"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/8op-org/gl1tch/internal/busd"
	"github.com/8op-org/gl1tch/internal/themes"
)

// ── Public message types ──────────────────────────────────────────────────────

// ThemeChangedMsg is the BubbleTea message delivered when the active theme
// changes via a busd event. Name contains the new theme name.
type ThemeChangedMsg struct {
	Name string
}

// ── Internal message types ────────────────────────────────────────────────────

// themeRetryMsg is returned by subscribeCmd when busd is unreachable.
// ThemeState.Handle schedules a retry after Wait.
type themeRetryMsg struct {
	wait time.Duration
}

// ── Wire types ────────────────────────────────────────────────────────────────

type registrationFrame struct {
	Name      string   `json:"name"`
	Subscribe []string `json:"subscribe"`
}

type eventFrame struct {
	Event   string          `json:"event"`
	Payload json.RawMessage `json:"payload"`
}

// ── ThemeState ────────────────────────────────────────────────────────────────

// ThemeState encapsulates theme initialisation, busd subscription, and retry
// logic for standalone BubbleTea sub-TUIs. It is a plain value type — embed it
// in your model or hold it as a field, then delegate Init and Update to it.
type ThemeState struct {
	bundle    *themes.Bundle
	retryWait time.Duration // 0 = not retrying; grows on consecutive failures
}

// NewThemeState creates a ThemeState seeded with the given bundle.
// bundle may be nil; Bundle() will return nil until the first successful
// theme event or until the global registry provides one.
func NewThemeState(bundle *themes.Bundle) ThemeState {
	return ThemeState{bundle: bundle}
}

// Bundle returns the currently active theme bundle. May be nil when no bundle
// has been loaded yet.
func (ts ThemeState) Bundle() *themes.Bundle {
	return ts.bundle
}

// Init returns the tea.Cmd that starts the busd subscription. Call it from
// your model's Init() method.
func (ts ThemeState) Init() tea.Cmd {
	return ts.subscribeCmd()
}

// Handle processes incoming BubbleTea messages for theme-related types.
// Returns (updatedState, nextCmd, true) when the message was consumed, or
// (ts, nil, false) when the message is not theme-related.
//
// Call this at the top of your Update() before your own switch:
//
//	if ts, cmd, ok := m.themeState.Handle(msg); ok {
//	    m.themeState = ts
//	    return m, cmd
//	}
func (ts ThemeState) Handle(msg tea.Msg) (ThemeState, tea.Cmd, bool) {
	switch msg := msg.(type) {
	case ThemeChangedMsg:
		ts2 := ts
		ts2.retryWait = 0 // reset retry back-off on success
		// Look up the new bundle from the global registry.
		if gr := themes.GlobalRegistry(); gr != nil {
			if b, ok := gr.Get(msg.Name); ok {
				ts2.bundle = b
			} else if name := gr.RefreshActive(); name != "" {
				ts2.bundle = gr.Active()
			}
		}
		// Re-subscribe to receive the next theme change.
		return ts2, ts2.subscribeCmd(), true

	case themeRetryMsg:
		ts2 := ts
		ts2.retryWait = msg.wait
		// Sleep then attempt subscription again.
		return ts2, retryAfter(msg.wait, ts2), true
	}
	return ts, nil, false
}

// ── Internal helpers ──────────────────────────────────────────────────────────

// nextRetryWait returns the wait to use on the next failure, doubling from the
// current value (or starting at 2s) up to a 30s cap.
func (ts ThemeState) nextRetryWait() time.Duration {
	if ts.retryWait == 0 {
		return 2 * time.Second
	}
	w := ts.retryWait * 2
	if w > 30*time.Second {
		return 30 * time.Second
	}
	return w
}

// subscribeCmd returns a tea.Cmd that dials busd and blocks until a
// theme.changed event arrives, returning ThemeChangedMsg. If busd is
// unreachable it returns themeRetryMsg so Handle can schedule a retry.
func (ts ThemeState) subscribeCmd() tea.Cmd {
	retryWait := ts.nextRetryWait() // capture for the closure
	return func() tea.Msg {
		sockPath, err := busd.SocketPath()
		if err != nil {
			return themeRetryMsg{wait: retryWait}
		}

		conn, err := net.Dial("unix", sockPath)
		if err != nil {
			// busd not running — schedule retry.
			return themeRetryMsg{wait: retryWait}
		}
		defer conn.Close()

		// Register as a subscriber for theme.changed.
		reg := registrationFrame{
			Name:      "tuikit-theme",
			Subscribe: []string{themes.TopicThemeChanged},
		}
		b, _ := json.Marshal(reg)
		b = append(b, '\n')
		if _, err := conn.Write(b); err != nil {
			return themeRetryMsg{wait: retryWait}
		}

		// Block until the first matching event arrives.
		scanner := bufio.NewScanner(conn)
		for scanner.Scan() {
			var frame eventFrame
			if err := json.Unmarshal(scanner.Bytes(), &frame); err != nil {
				continue
			}
			if frame.Event != themes.TopicThemeChanged {
				continue
			}
			var payload themes.ThemeChangedPayload
			if err := json.Unmarshal(frame.Payload, &payload); err != nil {
				continue
			}
			return ThemeChangedMsg{Name: payload.Name}
		}
		// Connection dropped — retry.
		return themeRetryMsg{wait: retryWait}
	}
}

// retryAfter returns a tea.Cmd that sleeps for wait then runs ts.subscribeCmd.
func retryAfter(wait time.Duration, ts ThemeState) tea.Cmd {
	return func() tea.Msg {
		time.Sleep(wait)
		return ts.subscribeCmd()()
	}
}

// ThemeSubscribeCmd is the low-level subscription cmd for callers that manage
// their own retry logic. Prefer [ThemeState] for new code.
//
// Returns ThemeChangedMsg on success. Returns themeRetryMsg on failure (busd
// unavailable); since themeRetryMsg is unexported, callers using this directly
// should use ThemeState instead.
func ThemeSubscribeCmd() tea.Cmd {
	return NewThemeState(nil).subscribeCmd()
}
