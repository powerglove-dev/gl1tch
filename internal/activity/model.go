package activity

import (
	"context"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	tea "github.com/charmbracelet/bubbletea"
)

// ─── ANSI constants ───────────────────────────────────────────────────────────

const (
	aReset  = "\033[0m"
	aBold   = "\033[1m"
	aDim    = "\033[2m"
	aGreen  = "\033[32m"
	aYellow = "\033[33m"
	aRed    = "\033[31m"
)

// statusANSI returns the ANSI color code for a status string.
func statusANSI(status string) string {
	switch status {
	case "done":
		return aGreen
	case "running":
		return aYellow
	case "paused":
		return aYellow
	case "failed":
		return aRed
	default: // "scheduled" and anything else
		return aDim
	}
}

// ─── internal messages ────────────────────────────────────────────────────────

type loadedMsg struct{ events []ActivityEvent }
type newEventMsg struct{ e ActivityEvent }

// ─── Model ────────────────────────────────────────────────────────────────────

// maxEvents is the in-memory cap for the event list.
const maxEvents = 200

// Model is the BubbleTea activity-feed component.
// It renders a reverse-chronological timeline of agent activity events loaded
// from a JSONL file and kept live via fsnotify.
type Model struct {
	path         string
	events       []ActivityEvent // newest first
	scrollOffset int
	width        int
	height       int
	ch           chan ActivityEvent
	cancel       context.CancelFunc
}

// New returns an activity feed model backed by path. If path is empty the
// default path (~/.orcai/activity.jsonl) is used.
func New(path string) Model {
	if path == "" {
		path = DefaultPath()
	}
	return Model{path: path}
}

// ─── Init ─────────────────────────────────────────────────────────────────────

func (m Model) Init() tea.Cmd {
	ch := make(chan ActivityEvent, 64)
	ctx, cancel := context.WithCancel(context.Background())
	m.ch = ch
	m.cancel = cancel
	go WatchFeed(m.path, ch, ctx)
	return tea.Batch(
		func() tea.Msg { return m }, // store updated ch/cancel back into the running model
		loadCmd(m.path),
		drainActivityChan(ch),
	)
}

func loadCmd(path string) tea.Cmd {
	return func() tea.Msg {
		events, _ := LoadRecentEvents(path, 50)
		return loadedMsg{events: events}
	}
}

// drainActivityChan returns a tea.Cmd that blocks until one event arrives on ch.
func drainActivityChan(ch chan ActivityEvent) tea.Cmd {
	return func() tea.Msg {
		e, ok := <-ch
		if !ok {
			return nil
		}
		return newEventMsg{e: e}
	}
}

// ─── Update ───────────────────────────────────────────────────────────────────

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {

	// Init returns the model-with-channel as its first message so the running
	// model instance picks up the ch and cancel fields.
	case Model:
		m.ch = msg.ch
		m.cancel = msg.cancel
		return m, nil

	case loadedMsg:
		// Reverse so newest is at index 0.
		ev := msg.events
		for i, j := 0, len(ev)-1; i < j; i, j = i+1, j-1 {
			ev[i], ev[j] = ev[j], ev[i]
		}
		m.events = ev
		return m, nil

	case newEventMsg:
		m.events = append([]ActivityEvent{msg.e}, m.events...)
		if len(m.events) > maxEvents {
			m.events = m.events[:maxEvents]
		}
		var cmd tea.Cmd
		if m.ch != nil {
			cmd = drainActivityChan(m.ch)
		}
		return m, cmd

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "up", "k":
			if m.scrollOffset > 0 {
				m.scrollOffset--
			}
		case "down", "j":
			maxScroll := len(m.events) - 1
			if maxScroll < 0 {
				maxScroll = 0
			}
			if m.scrollOffset < maxScroll {
				m.scrollOffset++
			}
		case "g":
			m.scrollOffset = 0
		case "G":
			if len(m.events) > 0 {
				m.scrollOffset = len(m.events) - 1
			}
		}
		return m, nil
	}
	return m, nil
}

// Close cancels the watcher goroutine. Call when the component is unmounted.
func (m *Model) Close() {
	if m.cancel != nil {
		m.cancel()
		m.cancel = nil
	}
}

// ─── View ─────────────────────────────────────────────────────────────────────

func (m Model) View() string {
	if len(m.events) == 0 {
		return aDim + "  no activity yet" + aReset + "\n"
	}

	// Visible window: cardHeight=3 (2 lines + blank), fit as many as possible.
	cardHeight := 3
	visible := max(m.height/cardHeight, 10)

	start := m.scrollOffset
	end := min(start+visible, len(m.events))
	if start >= len(m.events) {
		start = max(len(m.events)-1, 0)
		end = len(m.events)
	}

	var sb strings.Builder
	for i := start; i < end; i++ {
		sb.WriteString(renderCard(m.events[i]))
		if i < end-1 {
			sb.WriteByte('\n')
		}
	}
	return sb.String()
}

// renderCard returns the two-line ANSI card for a single event.
//
// Line 1:  \033[1m● agent\033[0m          \033[2mHH:MM\033[0m  \033[3Xm status\033[0m
// Line 2:  \033[2m  └─ label  [kind]\033[0m
func renderCard(e ActivityEvent) string {
	color := statusANSI(e.Status)

	// ── Parse timestamp ──────────────────────────────────────────────────────
	tsDisplay := ""
	if t, err := time.Parse(time.RFC3339, e.TS); err == nil {
		tsDisplay = t.Local().Format("15:04")
	} else if len(e.TS) >= 16 {
		tsDisplay = e.TS[11:16] // best-effort slice HH:MM
	}

	// ── Line 1 ───────────────────────────────────────────────────────────────
	// color●  bold-agent   dim-time   color-status
	dot := color + "●" + aReset
	agentStr := aBold + truncate(e.Agent, 24) + aReset
	timeStr := aDim + tsDisplay + aReset
	statusStr := color + e.Status + aReset

	line1 := fmt.Sprintf("%s %s  %s  %s", dot, agentStr, timeStr, statusStr)

	// ── Line 2 ───────────────────────────────────────────────────────────────
	label := truncate(e.Label, 48)
	line2 := fmt.Sprintf("%s  └─ %s  [%s]%s", aDim, label, e.Kind, aReset)

	return line1 + "\n" + line2
}

// truncate shortens s to at most n runes, appending "…" when clipped.
func truncate(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	runes := []rune(s)
	return string(runes[:n-1]) + "…"
}
