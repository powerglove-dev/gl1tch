// Package modal provides shared modal overlay rendering for ORCAI TUI components.
// It extracts the common confirm/alert/scroll modal patterns used by switchboard,
// crontui, and other packages into a single, reusable location.
package modal

import (
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/adam-stokes/orcai/internal/themes"
)

// Config holds all configuration for rendering a modal.
type Config struct {
	Bundle       *themes.Bundle
	Title        string
	Message      string
	ConfirmLabel string
	DismissLabel string
}

// Colors holds resolved lipgloss-compatible color strings.
type Colors struct {
	Border  string
	TitleBG string
	TitleFG string
	FG      string
	Accent  string
	Dim     string
	Error   string
}

// ResolveColors derives modal colors from cfg.Bundle with Dracula fallbacks.
func ResolveColors(cfg Config) Colors {
	c := Colors{
		Border:  "#bd93f9",
		TitleBG: "#bd93f9",
		TitleFG: "#282a36",
		FG:      "#f8f8f2",
		Accent:  "#8be9fd",
		Dim:     "#6272a4",
		Error:   "#ff5555",
	}
	b := cfg.Bundle
	if b == nil {
		return c
	}
	if v := b.ResolveRef(b.Modal.Border); v != "" {
		c.Border = v
		c.TitleBG = v
	}
	if v := b.ResolveRef(b.Modal.TitleBG); v != "" {
		c.TitleBG = v
	}
	if v := b.ResolveRef(b.Modal.TitleFG); v != "" {
		c.TitleFG = v
	}
	if v := b.Palette.FG; v != "" {
		c.FG = v
	}
	if v := b.Palette.Accent; v != "" {
		c.Accent = v
	}
	if v := b.Palette.Dim; v != "" {
		c.Dim = v
	}
	if v := b.Palette.Error; v != "" {
		c.Error = v
	}
	return c
}

// RenderConfirm renders a centered bordered confirm modal with a title bar and
// action hints. It returns the unsized box string; the caller is responsible
// for centering/overlaying it on the screen.
//
// Layout:
//
//	╭──────────────────────────────────────────────────╮
//	│ <Title>                                          │
//	│                                                  │
//	│  <Message>                                       │
//	│                                                  │
//	│  [y]es   [n]o / esc                              │
//	╰──────────────────────────────────────────────────╯
func RenderConfirm(cfg Config, w, _ int) string {
	c := ResolveColors(cfg)

	innerW := 44
	if innerW+4 > w {
		innerW = max(w-4, 10)
	}
	outerW := innerW + 2

	headerStyle := lipgloss.NewStyle().
		Background(lipgloss.Color(c.TitleBG)).
		Foreground(lipgloss.Color(c.TitleFG)).
		Bold(true).
		Width(innerW).
		Align(lipgloss.Center)

	rowStyle := func(fg lipgloss.Color) lipgloss.Style {
		return lipgloss.NewStyle().Foreground(fg).Width(innerW).Padding(0, 1)
	}

	title := cfg.Title
	if title == "" {
		title = "Confirm"
	}
	message := cfg.Message
	if message == "" {
		message = cfg.Title
	}

	confirmLabel := cfg.ConfirmLabel
	if confirmLabel == "" {
		confirmLabel = lipgloss.NewStyle().Foreground(lipgloss.Color(c.Accent)).Bold(true).Render("[y]") + "es"
	}
	dismissLabel := cfg.DismissLabel
	if dismissLabel == "" {
		dismissLabel = lipgloss.NewStyle().Foreground(lipgloss.Color(c.Dim)).Render("[n]") + "o / esc"
	}

	rows := []string{headerStyle.Render(title)}
	rows = append(rows, rowStyle(lipgloss.Color(c.FG)).Render(message))
	rows = append(rows, "")
	rows = append(rows, rowStyle(lipgloss.Color(c.FG)).Render(confirmLabel+"   "+dismissLabel))

	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(c.Border)).
		Width(outerW).
		Render(strings.Join(rows, "\n"))
}

// RenderAlert renders a simple alert modal with only a dismiss hint.
// It is similar to RenderConfirm but without a confirm action.
func RenderAlert(cfg Config, message string, w, _ int) string {
	c := ResolveColors(cfg)

	innerW := 44
	if innerW+4 > w {
		innerW = max(w-4, 10)
	}
	outerW := innerW + 2

	headerStyle := lipgloss.NewStyle().
		Background(lipgloss.Color(c.TitleBG)).
		Foreground(lipgloss.Color(c.TitleFG)).
		Bold(true).
		Width(innerW).
		Align(lipgloss.Center)

	rowStyle := func(fg lipgloss.Color) lipgloss.Style {
		return lipgloss.NewStyle().Foreground(fg).Width(innerW).Padding(0, 1)
	}

	title := cfg.Title
	if title == "" {
		title = "Alert"
	}

	dismissLabel := cfg.DismissLabel
	if dismissLabel == "" {
		dismissLabel = lipgloss.NewStyle().Foreground(lipgloss.Color(c.Dim)).Render("[esc]") + " close"
	}

	rows := []string{headerStyle.Render(title)}
	if message != "" {
		rows = append(rows, rowStyle(lipgloss.Color(c.FG)).Render(message))
	}
	rows = append(rows, "")
	rows = append(rows, rowStyle(lipgloss.Color(c.FG)).Render(dismissLabel))

	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(c.Border)).
		Width(outerW).
		Render(strings.Join(rows, "\n"))
}

// RenderScroll renders a scrollable content modal with a scroll indicator.
// lines is the pre-rendered content to display; offset is the current scroll
// position. The modal returns a positioned box string; callers overlay it.
func RenderScroll(cfg Config, lines []string, offset, w, h int) string {
	c := ResolveColors(cfg)

	innerW := w - 4
	if innerW < 40 {
		innerW = 40
	}
	outerW := innerW + 2

	headerStyle := lipgloss.NewStyle().
		Background(lipgloss.Color(c.TitleBG)).
		Foreground(lipgloss.Color(c.TitleFG)).
		Bold(true).
		Width(innerW).
		Align(lipgloss.Center)

	title := cfg.Title
	if title == "" {
		title = "Help"
	}

	// Clamp scroll offset.
	visibleH := h - 6
	if visibleH < 4 {
		visibleH = 4
	}
	if offset > len(lines)-visibleH {
		offset = max(len(lines)-visibleH, 0)
	}
	if offset < 0 {
		offset = 0
	}
	end := offset + visibleH
	if end > len(lines) {
		end = len(lines)
	}
	visible := lines[offset:end]

	// Scroll indicator.
	total := len(lines)
	var scrollInfo string
	if total > visibleH {
		scrollInfo = lipgloss.NewStyle().Foreground(lipgloss.Color(c.Dim)).
			Render(strings.Repeat(" ", innerW-12) +
				lipgloss.NewStyle().Foreground(lipgloss.Color(c.Accent)).Render("j/k  [/]") +
				lipgloss.NewStyle().Foreground(lipgloss.Color(c.Dim)).Render(" scroll  esc close"))
	} else {
		scrollInfo = lipgloss.NewStyle().Foreground(lipgloss.Color(c.Dim)).
			Width(innerW).Align(lipgloss.Right).Padding(0, 1).
			Render("esc close")
	}

	body := lipgloss.NewStyle().
		Width(innerW).
		Padding(0, 1).
		Render(strings.Join(visible, "\n"))

	content := strings.Join([]string{
		headerStyle.Render(title),
		body,
		scrollInfo,
	}, "\n")

	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(c.Border)).
		Width(outerW).
		Render(content)
}

// API is the modal interface exposed for use by plugin host contexts.
type API interface {
	RenderConfirm(title, message string) string
	RenderAlert(title, message string) string
}

// apiImpl is the concrete implementation of API.
type apiImpl struct {
	bundle *themes.Bundle
	w, h   int
}

// NewAPI creates a new API instance with the given theme bundle and screen dimensions.
func NewAPI(bundle *themes.Bundle, w, h int) API {
	return &apiImpl{bundle: bundle, w: w, h: h}
}

func (a *apiImpl) RenderConfirm(title, message string) string {
	return RenderConfirm(Config{Bundle: a.bundle, Title: title, Message: message}, a.w, a.h)
}

func (a *apiImpl) RenderAlert(title, message string) string {
	return RenderAlert(Config{Bundle: a.bundle, Title: title}, message, a.w, a.h)
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
