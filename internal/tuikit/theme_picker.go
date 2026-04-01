package tuikit

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/8op-org/gl1tch/internal/busd"
	"github.com/8op-org/gl1tch/internal/panelrender"
	"github.com/8op-org/gl1tch/internal/styles"
	"github.com/8op-org/gl1tch/internal/themes"
	"github.com/8op-org/gl1tch/internal/translations"
)

// ThemePicker holds the state for the tabbed 2-column theme picker overlay.
type ThemePicker struct {
	Open             bool
	Tab              int            // 0=dark, 1=light
	DarkLeftCursor   int            // cursor within dark tab left column
	DarkRightCursor  int            // cursor within dark tab right column
	LightLeftCursor  int            // cursor within light tab left column
	LightRightCursor int            // cursor within light tab right column
	ColFocus         int            // 0=left, 1=right
	OriginalTheme    *themes.Bundle // theme at open time; restored on esc
}

// currentBundle returns the currently highlighted bundle given dark/light slices.
func (tp ThemePicker) currentBundle(dark, light []themes.Bundle) *themes.Bundle {
	bundles := dark
	if tp.Tab == 1 {
		bundles = light
	}
	if len(bundles) == 0 {
		return nil
	}
	half := (len(bundles) + 1) / 2
	var idx int
	if tp.ColFocus == 0 {
		cur := tp.DarkLeftCursor
		if tp.Tab == 1 {
			cur = tp.LightLeftCursor
		}
		if cur >= half {
			cur = half - 1
		}
		idx = cur
	} else {
		cur := tp.DarkRightCursor
		if tp.Tab == 1 {
			cur = tp.LightRightCursor
		}
		rightLen := len(bundles) - half
		if rightLen <= 0 {
			return &bundles[0]
		}
		if cur >= rightLen {
			cur = rightLen - 1
		}
		idx = half + cur
	}
	if idx < 0 {
		idx = 0
	}
	if idx >= len(bundles) {
		idx = len(bundles) - 1
	}
	return &bundles[idx]
}

// PreviewTheme applies a theme's colors to tmux and publishes a busd event
// WITHOUT persisting to disk. Used for live preview during picker navigation.
func PreviewTheme(bundle themes.Bundle) {
	themes.ApplyTmux(&bundle)
	if sockPath, err := busd.SocketPath(); err == nil {
		_ = busd.PublishEvent(sockPath, themes.TopicThemeChanged, themes.ThemeChangedPayload{Name: bundle.Name})
	}
}

// ApplyThemeSelection activates the chosen theme: updates the registry, applies
// tmux colors, publishes a busd event, persists to disk, and rebuilds the
// global translation chain so theme-bundled strings take effect immediately.
func ApplyThemeSelection(chosen themes.Bundle) {
	if gr := themes.GlobalRegistry(); gr != nil {
		_ = gr.SetActive(chosen.Name)
	}
	themes.ApplyTmux(&chosen)
	if sockPath, err := busd.SocketPath(); err == nil {
		_ = busd.PublishEvent(sockPath, themes.TopicThemeChanged, themes.ThemeChangedPayload{Name: chosen.Name})
	}
	translations.RebuildChain(chosen.Strings)
}

// themeRow renders a single theme row with swatch and name.
func themeRow(b themes.Bundle, selected bool, active *themes.Bundle, pal styles.ANSIPalette, colW int) string {
	bp := b.Palette
	swatch := fmt.Sprintf("%s█\x1b[0m%s█\x1b[0m%s█\x1b[0m%s█\x1b[0m%s█\x1b[0m%s█\x1b[0m%s█\x1b[0m",
		styles.FgSeq(bp.BG), styles.FgSeq(bp.FG), styles.FgSeq(bp.Accent),
		styles.FgSeq(bp.Dim), styles.FgSeq(bp.Border), styles.FgSeq(bp.Error), styles.FgSeq(bp.Success),
	)
	name := b.DisplayName
	if active != nil && b.Name == active.Name {
		name += " ✓"
	}
	var prefix string
	if selected {
		prefix = pal.Accent + ">" + panelrender.RST
	} else {
		prefix = " "
	}
	// Truncate name if too long
	maxNameW := colW - 12 // 1 prefix + 1 space + 7 swatch blocks + 2 spaces + some margin
	if maxNameW < 5 {
		maxNameW = 5
	}
	if len(name) > maxNameW {
		name = name[:maxNameW-1] + "…"
	}
	if selected {
		return prefix + " " + swatch + "  " + pal.FG + name + panelrender.RST
	}
	return prefix + " " + swatch + "  " + name
}

// ViewThemePicker renders the tabbed 2-column theme picker overlay.
func ViewThemePicker(dark, light []themes.Bundle, picker ThemePicker, active *themes.Bundle, w int) string {
	pal := styles.BundleANSI(active)

	boxW := 84
	if boxW+4 > w {
		boxW = w - 4
	}
	if boxW < 40 {
		boxW = 40
	}

	tp := translations.GlobalProvider()
	tStr := func(key, fallback string) string {
		if tp == nil {
			return fallback
		}
		return tp.T(key, fallback)
	}

	pickerTitle := tStr(translations.KeyThemePickerTitle, "SELECT THEME")
	darkTab := tStr(translations.KeyThemePickerDarkTab, "Dark")
	lightTab := tStr(translations.KeyThemePickerLightTab, "Light")

	var rows []string
	rows = append(rows, panelrender.BoxTop(boxW, pickerTitle, pal.Border, pal.FG))

	// Tab bar
	var darkLabel, lightLabel string
	if picker.Tab == 0 {
		darkLabel = pal.Accent + "[ " + darkTab + " ]" + panelrender.RST
		lightLabel = pal.Dim + "  " + lightTab + "  " + panelrender.RST
	} else {
		darkLabel = pal.Dim + "  " + darkTab + "  " + panelrender.RST
		lightLabel = pal.Accent + "[ " + lightTab + " ]" + panelrender.RST
	}
	tabBar := darkLabel + "  " + lightLabel
	rows = append(rows, panelrender.BoxRow("  "+tabBar, boxW, pal.Border))
	rows = append(rows, panelrender.BoxRow("", boxW, pal.Border))

	// Determine active bundles and cursors
	bundles := dark
	leftCursor := picker.DarkLeftCursor
	rightCursor := picker.DarkRightCursor
	if picker.Tab == 1 {
		bundles = light
		leftCursor = picker.LightLeftCursor
		rightCursor = picker.LightRightCursor
	}

	half := (len(bundles) + 1) / 2
	leftBundles := bundles
	if half < len(bundles) {
		leftBundles = bundles[:half]
	}
	rightBundles := bundles[half:]

	colW := (boxW - 6) / 2

	// Build column lines
	maxRows := len(leftBundles)
	if len(rightBundles) > maxRows {
		maxRows = len(rightBundles)
	}

	for i := 0; i < maxRows; i++ {
		var leftLine, rightLine string

		if i < len(leftBundles) {
			sel := picker.ColFocus == 0 && i == leftCursor
			leftLine = themeRow(leftBundles[i], sel, active, pal, colW)
		} else {
			leftLine = strings.Repeat(" ", colW)
		}

		if i < len(rightBundles) {
			sel := picker.ColFocus == 1 && i == rightCursor
			rightLine = themeRow(rightBundles[i], sel, active, pal, colW)
		} else {
			rightLine = strings.Repeat(" ", colW)
		}

		leftStyle := lipgloss.NewStyle().Width(colW)
		rightStyle := lipgloss.NewStyle().Width(colW)
		combined := lipgloss.JoinHorizontal(lipgloss.Top,
			leftStyle.Render(leftLine),
			"  ",
			rightStyle.Render(rightLine),
		)
		rows = append(rows, panelrender.BoxRow(combined, boxW, pal.Border))
	}

	rows = append(rows, panelrender.BoxRow("", boxW, pal.Border))
	hint := pal.Accent + "tab" + pal.Dim + " mode  " +
		pal.Accent + "h/l" + pal.Dim + " col  " +
		pal.Accent + "j/k" + pal.Dim + " nav  " +
		pal.Accent + "enter" + pal.Dim + " apply  " +
		pal.Accent + "esc" + pal.Dim + " cancel" + panelrender.RST
	rows = append(rows, panelrender.BoxRow("  "+hint, boxW, pal.Border))
	rows = append(rows, panelrender.BoxBot(boxW, pal.Border))

	return strings.Join(rows, "\n")
}

// HandleThemePicker processes key events for the theme picker.
// Returns the updated picker, whether to close, the selected bundle (non-nil only on "enter"), and a cmd.
// dark and light are the split bundle slices from the registry.
func HandleThemePicker(picker ThemePicker, dark, light []themes.Bundle, key string) (ThemePicker, bool, *themes.Bundle, tea.Cmd) {
	bundles := dark
	if picker.Tab == 1 {
		bundles = light
	}
	half := 0
	if len(bundles) > 0 {
		half = (len(bundles) + 1) / 2
	}
	rightLen := len(bundles) - half

	// Helper to get/set active column cursor
	getLeftCursor := func() int {
		if picker.Tab == 0 {
			return picker.DarkLeftCursor
		}
		return picker.LightLeftCursor
	}
	getRightCursor := func() int {
		if picker.Tab == 0 {
			return picker.DarkRightCursor
		}
		return picker.LightRightCursor
	}
	setLeftCursor := func(v int) {
		if picker.Tab == 0 {
			picker.DarkLeftCursor = v
		} else {
			picker.LightLeftCursor = v
		}
	}
	setRightCursor := func(v int) {
		if picker.Tab == 0 {
			picker.DarkRightCursor = v
		} else {
			picker.LightRightCursor = v
		}
	}

	var previewBundle *themes.Bundle

	switch key {
	case "esc", "q":
		if picker.OriginalTheme != nil {
			PreviewTheme(*picker.OriginalTheme)
		}
		return picker, true, nil, nil

	case "tab":
		picker.Tab = 1 - picker.Tab
		previewBundle = picker.currentBundle(dark, light)

	case "h", "left":
		picker.ColFocus = 0
		previewBundle = picker.currentBundle(dark, light)

	case "l", "right":
		if rightLen > 0 {
			picker.ColFocus = 1
		}
		previewBundle = picker.currentBundle(dark, light)

	case "j", "down":
		if picker.ColFocus == 0 {
			cur := getLeftCursor()
			maxLeft := half - 1
			if maxLeft < 0 {
				maxLeft = 0
			}
			if cur < maxLeft {
				cur++
			}
			setLeftCursor(cur)
		} else {
			cur := getRightCursor()
			if rightLen > 0 && cur < rightLen-1 {
				cur++
			}
			setRightCursor(cur)
		}
		previewBundle = picker.currentBundle(dark, light)

	case "k", "up":
		if picker.ColFocus == 0 {
			cur := getLeftCursor()
			if cur > 0 {
				cur--
			}
			setLeftCursor(cur)
		} else {
			cur := getRightCursor()
			if cur > 0 {
				cur--
			}
			setRightCursor(cur)
		}
		previewBundle = picker.currentBundle(dark, light)

	case "enter":
		b := picker.currentBundle(dark, light)
		if b != nil {
			ApplyThemeSelection(*b)
			return picker, true, b, tea.ClearScreen
		}
		return picker, true, nil, tea.ClearScreen
	}

	// Trigger live preview for any navigation
	if previewBundle != nil {
		PreviewTheme(*previewBundle)
	}

	return picker, false, nil, nil
}
