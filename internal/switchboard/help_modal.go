package switchboard

import (
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/adam-stokes/orcai/internal/panelrender"
	"github.com/adam-stokes/orcai/internal/translations"
)

// viewHelpModal renders the switchboard keybinding reference as an 80%-wide ANSI box overlay.
func (m Model) viewHelpModal(w, _ int) string {
	pal := m.ansiPalette()
	boxW := w * 4 / 5
	if boxW < 48 {
		boxW = 48
	}

	title := translations.Safe(translations.GlobalProvider(), translations.KeyHelpModalTitle, "ORCAI  getting started")

	// Helpers
	blank := func() string { return panelrender.BoxRow("", boxW, pal.Border) }
	row := func(s string) string { return panelrender.BoxRow(s, boxW, pal.Border) }

	// Section header: "  ── TITLE ──"
	section := func(t string) string {
		dash := pal.Dim + "──" + panelrender.RST
		label := pal.Accent + panelrender.BLD + " " + t + " " + panelrender.RST
		return row("  " + dash + label + dash)
	}

	// Key binding row: key left-padded to keyW visible chars, desc right
	const keyW = 20
	bind := func(k, desc string) string {
		pad := keyW - lipgloss.Width(k)
		if pad < 1 {
			pad = 1
		}
		return row("    " + pal.Accent + k + panelrender.RST + strings.Repeat(" ", pad) + pal.FG + desc + panelrender.RST)
	}

	// Note row (dimmed, no key column)
	note := func(s string) string {
		return row("    " + pal.Dim + s + panelrender.RST)
	}

	rows := []string{
		panelrender.BoxTop(boxW, title, pal.Border, pal.Accent),
		blank(),
		note("Chord prefix: " + pal.Accent + panelrender.BLD + "^spc" + panelrender.RST + pal.Dim + "  (ctrl+space, then the key below)"),
		blank(),
		section("HELP & SYSTEM"),
		bind("^spc h", "this help"),
		bind("^spc q", "quit ORCAI"),
		bind("^spc d", "detach  (session stays running)"),
		bind("^spc r", "reload  (hot-swap binary)"),
		blank(),
		section("WORKSPACE"),
		bind("^spc m", "theme picker"),
		bind("^spc j", "jump to any window"),
		bind("^spc t", "go to switchboard"),
		blank(),
		section("WINDOWS & PANES"),
		bind("^spc c", "new window"),
		bind("^spc [ / ]", "previous / next window"),
		bind("^spc | / -", "split pane right / down"),
		bind("^spc h/j/k/l", "navigate panes"),
		bind("^spc x / X", "kill pane / window"),
		blank(),
		section("NAVIGATION"),
		bind("tab / j / k", "navigate panels & list items"),
		bind("enter", "open / confirm / run selected"),
		bind("esc", "back / close overlay"),
		blank(),
		section("PANELS"),
		bind("Pipelines", "run and monitor named pipelines"),
		bind("Agent Runner", "launch and manage AI agents"),
		bind("Signal Board", "inter-agent message bus"),
		bind("Activity Feed", "live system events & run history"),
		bind("Cron", "scheduled pipeline and agent jobs"),
		blank(),
		panelrender.BoxBot(boxW, pal.Border),
	}
	return strings.Join(rows, "\n")
}
