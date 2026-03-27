package switchboard

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/glamour"

	"github.com/adam-stokes/orcai/internal/modal"
	"github.com/adam-stokes/orcai/internal/translations"
)

// readmeContent returns the README.md content, falling back to inline text.
func readmeContent() string {
	// Try reading from the binary's directory, then repo root.
	self, _ := os.Executable()
	if resolved, err := filepath.EvalSymlinks(self); err == nil {
		self = resolved
	}
	for _, candidate := range []string{
		filepath.Join(filepath.Dir(self), "README.md"),
		"README.md",
	} {
		if data, err := os.ReadFile(candidate); err == nil {
			return string(data)
		}
	}
	return fallbackReadme
}

// viewHelpModal renders the getting-started guide as a centered overlay.
func (m Model) viewHelpModal(w, h int) string {
	innerW := w - 4
	if innerW < 40 {
		innerW = 40
	}

	// Render markdown with glamour.
	rendered := renderMarkdown(readmeContent(), innerW)
	lines := strings.Split(strings.TrimRight(rendered, "\n"), "\n")

	cfg := modal.Config{
		Bundle: m.activeBundle(),
		Title:  translations.Safe(translations.GlobalProvider(), translations.KeyHelpModalTitle, "ORCAI  getting started"),
	}
	return modal.RenderScroll(cfg, lines, m.helpScrollOffset, w, h)
}

// renderMarkdown renders md using glamour, falling back to plain text on error.
func renderMarkdown(md string, width int) string {
	r, err := glamour.NewTermRenderer(
		glamour.WithAutoStyle(),
		glamour.WithWordWrap(width-4),
	)
	if err != nil {
		return md
	}
	out, err := r.Render(md)
	if err != nil {
		return md
	}
	return out
}

const fallbackReadme = `# ORCAI — Getting Started

Press **^spc** (ctrl+space) to access chord shortcuts.

**Navigation:** tab · j/k · enter

**Panels:** Pipelines · Agent Runner · Signal Board · Activity Feed

**Chord shortcuts:**
- ^spc h  this help
- ^spc q  quit
- ^spc d  detach
- ^spc r  reload
- ^spc m  themes
- ^spc j  jump to window
`
