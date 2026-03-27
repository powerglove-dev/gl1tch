package switchboard

import (
	"fmt"
	"os/exec"

	"github.com/adam-stokes/orcai/internal/themes"
)

// tmuxStatusRight builds the status-right string with colored key/desc pairs.
// Keys are rendered in accent color, descriptions in dim color.
func tmuxStatusRight(accent, dim string) string {
	key := func(k string) string { return fmt.Sprintf("#[fg=%s]%s", accent, k) }
	desc := func(d string) string { return fmt.Sprintf("#[fg=%s]%s", dim, d) }
	sp := fmt.Sprintf("#[fg=%s]  ", dim)
	grp := fmt.Sprintf("#[fg=%s]  │  ", dim) // group separator
	return " " +
		// Navigation
		key("^spc t") + desc(" switchboard") + sp +
		key("^spc j") + desc(" jump") + grp +
		// Windows
		key("^spc c") + desc(" win") + grp +
		// Appearance
		key("^spc m") + desc(" themes") + grp +
		// Help
		key("^spc h") + desc(" help") + grp +
		// Session
		key("^spc d") + desc(" detach") + sp +
		key("^spc r") + desc(" reload") + sp +
		key("^spc q") + desc(" quit") + sp +
		fmt.Sprintf("#[fg=%s]%%H:%%M ", dim)
}

// applyTmuxTheme pushes theme colors to the running tmux session via set-option.
// Called after the user selects a new theme so the status bar updates immediately
// without needing a session restart.
func applyTmuxTheme(b *themes.Bundle) {
	if b == nil {
		return
	}
	accent := b.Palette.Accent
	bg := b.Palette.BG
	dim := b.Palette.Dim
	border := b.Palette.Border

	// Fall back to Nord defaults if palette fields are empty.
	if accent == "" {
		accent = "#88c0d0"
	}
	if bg == "" {
		bg = "#2e3440"
	}
	if dim == "" {
		dim = "#4c566a"
	}
	if border == "" {
		border = "#3b4252"
	}

	opts := [][]string{
		{"set-option", "-g", "status-style", fmt.Sprintf("fg=%s,bg=%s", accent, bg)},
		{"set-option", "-g", "status-left", fmt.Sprintf("#[fg=%s,bold] ORCAI #[default]", accent)},
		{"set-option", "-g", "status-right-length", "200"},
		{"set-option", "-g", "status-right", tmuxStatusRight(accent, dim)},
		{"set-option", "-g", "pane-border-style", fmt.Sprintf("fg=%s", border)},
		{"set-option", "-g", "pane-active-border-style", fmt.Sprintf("fg=%s", accent)},
	}

	for _, args := range opts {
		_ = exec.Command("tmux", args...).Run()
	}
}
