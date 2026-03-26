// Package styles provides Dracula-themed lipgloss styles for orcai UIs.
package styles

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/adam-stokes/orcai/internal/themes"
)

// Dracula colour palette
var (
	Purple  = lipgloss.Color("#bd93f9")
	Pink    = lipgloss.Color("#ff79c6")
	Cyan    = lipgloss.Color("#8be9fd")
	Green   = lipgloss.Color("#50fa7b")
	Yellow  = lipgloss.Color("#f1fa8c")
	Red     = lipgloss.Color("#ff5555")
	Comment = lipgloss.Color("#6272a4")
	Fg      = lipgloss.Color("#f8f8f2")
	Bg      = lipgloss.Color("#282a36")
	SelBg   = lipgloss.Color("#44475a")
)

// Pre-built styles
// Deprecated: use bundle-aware factory functions instead.
var (
	Title    = lipgloss.NewStyle().Foreground(Purple).Bold(true)
	Subtitle = lipgloss.NewStyle().Foreground(Cyan)
	Selected = lipgloss.NewStyle().Background(SelBg).Foreground(Pink)
	Dimmed   = lipgloss.NewStyle().Foreground(Comment)
	Normal   = lipgloss.NewStyle().Foreground(Fg)
	Success  = lipgloss.NewStyle().Foreground(Green)
	Error    = lipgloss.NewStyle().Foreground(Red)
	Warning  = lipgloss.NewStyle().Foreground(Yellow)
	Divider  = lipgloss.NewStyle().Foreground(Comment)
	Border   = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(Purple)
)

// ── Bundle-aware factory functions ────────────────────────────────────────────

// TitleStyle returns a bold title style using the bundle's accent color.
func TitleStyle(b *themes.Bundle) lipgloss.Style {
	if b == nil {
		return Title
	}
	return lipgloss.NewStyle().Foreground(lipgloss.Color(b.Palette.Accent)).Bold(true)
}

// SubtitleStyle returns a subtitle style using the bundle's FG color.
func SubtitleStyle(b *themes.Bundle) lipgloss.Style {
	if b == nil {
		return Subtitle
	}
	return lipgloss.NewStyle().Foreground(lipgloss.Color(b.Palette.FG))
}

// SelectedStyle returns a selected item style using the bundle's border (selection bg) and accent.
func SelectedStyle(b *themes.Bundle) lipgloss.Style {
	if b == nil {
		return Selected
	}
	return lipgloss.NewStyle().
		Background(lipgloss.Color(b.Palette.Border)).
		Foreground(lipgloss.Color(b.Palette.Accent))
}

// DimmedStyle returns a dimmed style using the bundle's dim color.
func DimmedStyle(b *themes.Bundle) lipgloss.Style {
	if b == nil {
		return Dimmed
	}
	return lipgloss.NewStyle().Foreground(lipgloss.Color(b.Palette.Dim))
}

// NormalStyle returns a normal style using the bundle's FG color.
func NormalStyle(b *themes.Bundle) lipgloss.Style {
	if b == nil {
		return Normal
	}
	return lipgloss.NewStyle().Foreground(lipgloss.Color(b.Palette.FG))
}

// SuccessStyle returns a success style using the bundle's success color.
func SuccessStyle(b *themes.Bundle) lipgloss.Style {
	if b == nil {
		return Success
	}
	return lipgloss.NewStyle().Foreground(lipgloss.Color(b.Palette.Success))
}

// ErrorStyle returns an error style using the bundle's error color.
func ErrorStyle(b *themes.Bundle) lipgloss.Style {
	if b == nil {
		return Error
	}
	return lipgloss.NewStyle().Foreground(lipgloss.Color(b.Palette.Error))
}

// WarningStyle returns a warning style using the bundle's FG color (yellow fallback).
func WarningStyle(b *themes.Bundle) lipgloss.Style {
	if b == nil {
		return Warning
	}
	return lipgloss.NewStyle().Foreground(lipgloss.Color(b.Palette.FG))
}

// BorderStyle returns a rounded border style using the bundle's accent color.
func BorderStyle(b *themes.Bundle) lipgloss.Style {
	if b == nil {
		return Border
	}
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(b.Palette.Accent))
}

// ── ANSI palette ──────────────────────────────────────────────────────────────

// ANSIPalette holds pre-formatted 24-bit ANSI foreground escape sequences for a bundle.
type ANSIPalette struct {
	Accent  string
	Dim     string
	Success string
	Error   string
	FG      string
	BG      string
	Border  string
	SelBG   string
}

// BundleANSI builds an ANSIPalette from a themes.Bundle.
// Colors are hex strings like "#bd93f9"; converted to ESC[38;2;R;G;Bm sequences.
func BundleANSI(b *themes.Bundle) ANSIPalette {
	toFG := func(hex string) string {
		r, g, bv := hexToRGB(hex)
		return fmt.Sprintf("\x1b[38;2;%d;%d;%dm", r, g, bv)
	}
	toBG := func(hex string) string {
		r, g, bv := hexToRGB(hex)
		return fmt.Sprintf("\x1b[48;2;%d;%d;%dm", r, g, bv)
	}
	p := b.Palette
	return ANSIPalette{
		Accent:  toFG(p.Accent),
		Dim:     toFG(p.Dim),
		Success: toFG(p.Success),
		Error:   toFG(p.Error),
		FG:      toFG(p.FG),
		BG:      toFG(p.BG),
		Border:  toFG(p.Border),
		SelBG:   toBG(p.Border),
	}
}

// hexToRGB parses a "#rrggbb" hex string to R, G, B uint8 values.
func hexToRGB(hex string) (uint8, uint8, uint8) {
	hex = strings.TrimPrefix(hex, "#")
	if len(hex) != 6 {
		return 189, 147, 249 // Dracula purple fallback
	}
	r, _ := strconv.ParseUint(hex[0:2], 16, 8)
	g, _ := strconv.ParseUint(hex[2:4], 16, 8)
	b, _ := strconv.ParseUint(hex[4:6], 16, 8)
	return uint8(r), uint8(g), uint8(b)
}
