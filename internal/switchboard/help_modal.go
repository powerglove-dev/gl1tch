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

	p := translations.GlobalProvider()
	t := func(key, fallback string) string {
		if p == nil {
			return fallback
		}
		return p.T(key, fallback)
	}

	title := translations.Safe(p, translations.KeyHelpModalTitle, "GETTING STARTED")

	// Helpers
	blank := func() string { return panelrender.BoxRow("", boxW, pal.Border) }
	row := func(s string) string { return panelrender.BoxRow(s, boxW, pal.Border) }

	// Section header: "  ── TITLE ──"
	section := func(key, fallback string) string {
		dash := pal.Dim + "──" + panelrender.RST
		label := pal.Accent + panelrender.BLD + " " + t(key, fallback) + " " + panelrender.RST
		return row("  " + dash + label + dash)
	}

	// Key binding row: key left-padded to keyW visible chars, desc right
	const keyW = 20
	bind := func(k, descKey, descFallback string) string {
		pad := keyW - lipgloss.Width(k)
		if pad < 1 {
			pad = 1
		}
		return row("    " + pal.Accent + k + panelrender.RST + strings.Repeat(" ", pad) + pal.FG + t(descKey, descFallback) + panelrender.RST)
	}

	// Note row (dimmed, no key column)
	note := func(s string) string {
		return row("    " + pal.Dim + s + panelrender.RST)
	}

	rows := []string{
		panelrender.BoxTop(boxW, title, pal.Border, pal.Accent),
		blank(),
		note(t(translations.KeyHelpChordNote, "Chord prefix: "+pal.Accent+panelrender.BLD+"^spc"+panelrender.RST+pal.Dim+"  (ctrl+space, then the key below)")),
		blank(),
		section(translations.KeyHelpSectionSystem, "HELP & SYSTEM"),
		bind("^spc h", translations.KeyHelpBindHelp, "this help"),
		bind("^spc q", translations.KeyHelpBindQuit, "quit ORCAI"),
		bind("^spc d", translations.KeyHelpBindDetach, "detach  (session stays running)"),
		bind("^spc r", translations.KeyHelpBindReload, "reload  (hot-swap binary)"),
		blank(),
		section(translations.KeyHelpSectionWorkspace, "WORKSPACE"),
		bind("^spc t", translations.KeyHelpBindThemes, "theme picker"),
		bind("^spc j", translations.KeyHelpBindJump, "jump to any window"),
		blank(),
		section(translations.KeyHelpSectionWindows, "WINDOWS & PANES"),
		bind("^spc c", translations.KeyHelpBindNewWin, "new window"),
		bind("^spc [ / ]", translations.KeyHelpBindPrevWin, "previous / next window"),
		bind("^spc | / -", translations.KeyHelpBindSplitR, "split pane right / down"),
		bind("^spc h/j/k/l", translations.KeyHelpBindNavPane, "navigate panes"),
		bind("^spc x / X", translations.KeyHelpBindKill, "kill pane / window"),
		blank(),
		section(translations.KeyHelpSectionNav, "NAVIGATION"),
		bind("tab / j / k", translations.KeyHelpBindTabNav, "navigate panels & list items"),
		bind("enter", translations.KeyHelpBindEnter, "open / confirm / run selected"),
		bind("esc", translations.KeyHelpBindEsc, "back / close overlay"),
		blank(),
		section(translations.KeyHelpSectionPanels, "PANELS"),
		bind("Pipelines", translations.KeyHelpPanelPipelines, "run and monitor named pipelines"),
		bind("Agent Runner", translations.KeyHelpPanelAgentRunner, "launch and manage AI agents"),
		bind("Signal Board", translations.KeyHelpPanelSignalBoard, "inter-agent message bus"),
		bind("Activity Feed", translations.KeyHelpPanelActivityFeed, "live system events & run history"),
		bind("Cron", translations.KeyHelpPanelCron, "scheduled pipeline and agent jobs"),
		blank(),
		panelrender.BoxBot(boxW, pal.Border),
	}
	return strings.Join(rows, "\n")
}
