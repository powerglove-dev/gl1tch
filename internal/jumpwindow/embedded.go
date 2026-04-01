package jumpwindow

import (
	tea "github.com/charmbracelet/bubbletea"

	"github.com/8op-org/gl1tch/internal/themes"
)

// EmbeddedModel wraps the jump window for use as an in-process modal overlay
// inside the switchboard or cron TUI. It sends CloseMsg instead of tea.Quit
// when the user closes or selects a window.
type EmbeddedModel struct {
	inner model
}

// NewEmbedded returns an EmbeddedModel initialised with fresh window data.
func NewEmbedded(bundle *themes.Bundle) EmbeddedModel {
	m := newModel()
	m.embedded = true
	_ = bundle // newModel already seeds from GlobalRegistry; bundle unused here
	return EmbeddedModel{inner: m}
}

// Update forwards messages to the inner model.
func (e EmbeddedModel) Update(msg tea.Msg) (EmbeddedModel, tea.Cmd) {
	next, cmd := e.inner.Update(msg)
	if m, ok := next.(model); ok {
		e.inner = m
	}
	return e, cmd
}

// View renders the jump window at the given width. Height is derived from the
// model's stored terminal height; pass a WindowSizeMsg first to set it.
func (e EmbeddedModel) View() string {
	return e.inner.View()
}

// SetSize propagates terminal dimensions into the embedded model so View
// can fill the overlay correctly.
func (e *EmbeddedModel) SetSize(w, h int) {
	e.inner.width = w
	e.inner.height = h
}
