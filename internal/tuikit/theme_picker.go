package tuikit

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/adam-stokes/orcai/internal/busd"
	"github.com/adam-stokes/orcai/internal/panelrender"
	"github.com/adam-stokes/orcai/internal/styles"
	"github.com/adam-stokes/orcai/internal/themes"
)

// ThemePicker is a reusable overlay component for selecting a theme.
// Embed the Open and Cursor fields in your model, then call ViewThemePicker
// and HandleThemePicker from your View/Update methods.
type ThemePicker struct {
	Open   bool
	Cursor int
}

// ViewThemePicker renders the theme picker using the standard ORCAI box style.
func ViewThemePicker(bundles []themes.Bundle, cursor int, active *themes.Bundle, w int) string {
	pal := styles.BundleANSI(active)

	boxW := 56
	if boxW+4 > w {
		boxW = w - 4
	}
	if boxW < 20 {
		boxW = 20
	}

	var rows []string
	rows = append(rows, panelrender.BoxTop(boxW, "SELECT THEME", pal.Border, pal.Accent))

	for i, b := range bundles {
		bp := b.Palette
		swatch := fmt.Sprintf("%s█\x1b[0m%s█\x1b[0m%s█\x1b[0m%s█\x1b[0m%s█\x1b[0m%s█\x1b[0m%s█\x1b[0m",
			styles.FgSeq(bp.BG), styles.FgSeq(bp.FG), styles.FgSeq(bp.Accent),
			styles.FgSeq(bp.Dim), styles.FgSeq(bp.Border), styles.FgSeq(bp.Error), styles.FgSeq(bp.Success),
		)

		name := b.DisplayName + " (" + b.Name + ")"
		if active != nil && b.Name == active.Name {
			name += " \u2713"
		}

		var line string
		if i == cursor {
			line = pal.Accent + "> " + panelrender.RST + swatch + "  " + pal.FG + name + panelrender.RST
		} else {
			line = "  " + swatch + "  " + pal.FG + name + panelrender.RST
		}
		rows = append(rows, panelrender.BoxRow(line, boxW, pal.Border))
	}

	rows = append(rows, panelrender.BoxRow("", boxW, pal.Border))
	hint := pal.Accent + "j/k" + pal.Dim + " navigate  " + pal.Accent + "enter" + pal.Dim + " apply  " + pal.Accent + "esc" + pal.Dim + " cancel" + panelrender.RST
	rows = append(rows, panelrender.BoxRow("  "+hint, boxW, pal.Border))
	rows = append(rows, panelrender.BoxBot(boxW, pal.Border))

	return strings.Join(rows, "\n")
}

// ApplyThemeSelection activates the chosen theme: updates the registry, applies
// tmux colors, and publishes a busd event so other processes pick up the change.
func ApplyThemeSelection(chosen themes.Bundle) {
	if gr := themes.GlobalRegistry(); gr != nil {
		_ = gr.SetActive(chosen.Name)
	}
	themes.ApplyTmux(&chosen)
	if sockPath, err := busd.SocketPath(); err == nil {
		_ = busd.PublishEvent(sockPath, themes.TopicThemeChanged, themes.ThemeChangedPayload{Name: chosen.Name})
	}
}

// HandleThemePicker processes key events for the theme picker.
// Returns the updated cursor, whether to close, the selected bundle (non-nil only on "enter"), and a cmd.
func HandleThemePicker(cursor int, bundles []themes.Bundle, key string) (newCursor int, close bool, selected *themes.Bundle, cmd tea.Cmd) {
	switch key {
	case "esc", "q":
		return cursor, true, nil, nil
	case "j", "down":
		if cursor < len(bundles)-1 {
			cursor++
		}
		return cursor, false, nil, nil
	case "k", "up":
		if cursor > 0 {
			cursor--
		}
		return cursor, false, nil, nil
	case "enter":
		if cursor < len(bundles) {
			ApplyThemeSelection(bundles[cursor])
			return cursor, true, &bundles[cursor], tea.ClearScreen
		}
		return cursor, true, nil, tea.ClearScreen
	}
	return cursor, false, nil, nil
}
