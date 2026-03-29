package switchboard

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/reflow/wrap"

	"github.com/adam-stokes/orcai/internal/panelrender"
	"github.com/adam-stokes/orcai/internal/store"
	"github.com/adam-stokes/orcai/internal/styles"
)

type runMeta struct {
	PipelineFile string `json:"pipeline_file"`
	CWD          string `json:"cwd"`
	Model        string `json:"model"`
}

func parseRunMetadata(raw string) runMeta {
	if raw == "" {
		return runMeta{}
	}
	var m runMeta
	_ = json.Unmarshal([]byte(raw), &m)
	return m
}

func collapseTilde(path string) string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return path
	}
	if strings.HasPrefix(path, home) {
		return "~" + path[len(home):]
	}
	return path
}

// buildRunContent formats the full detail text for a run using the ANSI palette.
func buildRunContent(run store.Run, pal styles.ANSIPalette, markdownMode bool, innerW int) string {
	dim := func(s string) string { return pal.Dim + s + aRst }
	fg := func(s string) string { return pal.FG + s + aRst }
	ok := func(s string) string { return pal.Success + s + aRst }
	bad := func(s string) string { return pal.Error + s + aRst }

	var sb strings.Builder

	// Started / finished / duration / exit status
	startedStr := time.UnixMilli(run.StartedAt).Format("2006-01-02 3:04:05 PM")
	sb.WriteString(dim("started:  ") + fg(startedStr) + "\n")

	if run.FinishedAt != nil {
		finishedStr := time.UnixMilli(*run.FinishedAt).Format("2006-01-02 3:04:05 PM")
		sb.WriteString(dim("finished: ") + fg(finishedStr) + "\n")
		dur := time.Duration((*run.FinishedAt - run.StartedAt) * int64(time.Millisecond))
		sb.WriteString(dim("duration: ") + fg(dur.Round(time.Second).String()) + "  ")
	} else {
		sb.WriteString(dim("finished: ") + fg("(in progress)") + "\n")
		dur := time.Since(time.UnixMilli(run.StartedAt))
		sb.WriteString(dim("duration: ") + fg(dur.Round(time.Second).String()) + "  ")
	}

	if run.ExitStatus != nil {
		if *run.ExitStatus == 0 {
			sb.WriteString(dim("exit: ") + ok("OK"))
		} else {
			sb.WriteString(dim("exit: ") + bad(fmt.Sprintf("ERROR (%d)", *run.ExitStatus)))
		}
	} else {
		sb.WriteString(dim("exit: ") + fg("(running)"))
	}
	sb.WriteString("\n")

	// Metadata: pipeline file, cwd, model
	meta := parseRunMetadata(run.Metadata)
	if meta.PipelineFile != "" {
		sb.WriteString(dim("pipeline: ") + fg(collapseTilde(meta.PipelineFile)) + "\n")
	}
	if meta.CWD != "" {
		sb.WriteString(dim("cwd:      ") + fg(collapseTilde(meta.CWD)) + "\n")
	}
	if meta.Model != "" {
		sb.WriteString(dim("model:    ") + fg(meta.Model) + "\n")
	}

	// Steps section (if any recorded)
	if len(run.Steps) > 0 {
		sb.WriteString(dim(strings.Repeat("─", 40)) + "\n")
		sb.WriteString(dim("steps:") + "\n")
		lastIdx := len(run.Steps) - 1
		for i, step := range run.Steps {
			connector := "├ "
			if i == lastIdx {
				connector = "└ "
			}
			var badge string
			switch step.Status {
			case "done":
				badge = pal.Success + "✓" + aRst
			case "failed":
				badge = pal.Error + "✗" + aRst
			default:
				badge = pal.Warn + "·" + aRst
			}
			dur := ""
			if step.DurationMs > 0 {
				d := time.Duration(step.DurationMs) * time.Millisecond
				dur = "  " + dim(d.Round(time.Second).String())
			}
			model := ""
			if step.Model != "" {
				model = "  " + dim(step.Model)
			}
			sb.WriteString("  " + dim(connector) + badge + " " + fg(step.ID) + dur + model + "\n")
		}
	}

	// Separator
	sb.WriteString(dim(strings.Repeat("─", 40)) + "\n")

	// Stdout
	if run.Stdout != "" {
		stdout := run.Stdout
		if markdownMode {
			if renderer, err := glamour.NewTermRenderer(
				glamour.WithStandardStyle("dark"),
				glamour.WithWordWrap(innerW),
			); err == nil {
				if rendered, err := renderer.Render(run.Stdout); err == nil {
					stdout = rendered
				}
			}
		} else {
			stdout = wrapContent(run.Stdout, innerW)
		}
		sb.WriteString(stdout)
		if !strings.HasSuffix(stdout, "\n") {
			sb.WriteString("\n")
		}
	} else {
		sb.WriteString(dim("(no stdout)") + "\n")
	}

	// Stderr section (only if non-empty)
	if run.Stderr != "" {
		sb.WriteString(dim(strings.Repeat("─", 40)) + "\n")
		sb.WriteString(bad("stderr:") + "\n")
		sb.WriteString(wrapContent(run.Stderr, innerW))
		if !strings.HasSuffix(run.Stderr, "\n") {
			sb.WriteString("\n")
		}
	}

	return sb.String()
}

// viewInboxDetail renders the inbox run detail as a full-screen ANSI box panel.
func (m Model) viewInboxDetail(w, h int, markdownMode bool) string {
	runs := m.inboxModel.Runs()
	if len(runs) == 0 {
		return ""
	}

	idx := m.inboxDetailIdx
	if idx < 0 {
		idx = 0
	}
	if idx >= len(runs) {
		idx = len(runs) - 1
	}
	run := runs[idx]

	pal := m.ansiPalette()
	boxW := w
	innerW := boxW - 4 // inside borders (2) + one space padding each side (2)

	// Title: "INBOX  [n/total]  kind · name"
	counter := fmt.Sprintf("[%d/%d]", idx+1, len(runs))
	title := "INBOX  " + counter + "  " + run.Kind + " · " + run.Name
	maxTitleW := boxW - 8
	if lipgloss.Width(title) > maxTitleW {
		title = title[:maxTitleW-1] + "…"
	}

	content := buildRunContent(run, pal, markdownMode, innerW)
	lines := strings.Split(strings.TrimRight(content, "\n"), "\n")

	// Height budget: topBar takes 1 row; box uses remainder.
	// Fixed box rows: top(1) + hints(1) + bot(1) = 3.
	visibleH := h - 1 - 3
	if visibleH < 4 {
		visibleH = 4
	}

	offset := m.inboxDetailScroll
	maxOffset := max(len(lines)-visibleH, 0)
	if offset > maxOffset {
		offset = maxOffset
	}
	if offset < 0 {
		offset = 0
	}
	end := offset + visibleH
	if end > len(lines) {
		end = len(lines)
	}
	visible := lines[offset:end]

	var rows []string
	rows = append(rows, boxTop(boxW, title, pal.Border, pal.Accent))
	for i, line := range visible {
		absIdx := offset + i
		prefix := " "
		rendered := line

		isCursor := absIdx == m.inboxDetailCursor
		isMarked := m.inboxDetailMarked != nil && m.inboxDetailMarked[absIdx]

		switch {
		case isCursor && isMarked:
			// Both: cursor style dominates, add a mark glyph
			prefix = pal.Accent + "▶" + aRst
			rendered = lipgloss.NewStyle().
				Background(lipgloss.Color("#44475a")).
				Render(stripANSI(line))
		case isCursor:
			prefix = pal.Accent + "▶" + aRst
			rendered = lipgloss.NewStyle().
				Background(lipgloss.Color("#383a47")).
				Render(stripANSI(line))
		case isMarked:
			prefix = pal.Success + "●" + aRst
			rendered = lipgloss.NewStyle().
				Background(lipgloss.Color("#2d4a35")).
				Render(stripANSI(line))
		}

		rows = append(rows, boxRow(prefix+rendered, boxW, pal.Border))
	}
	// Pad to fill the box height
	for i := len(visible); i < visibleH; i++ {
		rows = append(rows, boxRow("", boxW, pal.Border))
	}

	markCount := len(m.inboxDetailMarked)
	var dispatchHint panelrender.Hint
	if markCount > 0 {
		dispatchHint = panelrender.Hint{Key: "r", Desc: fmt.Sprintf("dispatch (%d)", markCount)}
	} else {
		dispatchHint = panelrender.Hint{Key: "r", Desc: "dispatch"}
	}
	markDesc := "mark"
	switch m.inboxMarkMode {
	case MarkModeActive:
		markDesc = "pause"
	case MarkModePaused:
		markDesc = "resume"
	}
	hintList := []panelrender.Hint{
		{Key: "j/k", Desc: "scroll"},
		{Key: "n/p", Desc: "next/prev"},
		{Key: "m", Desc: markDesc},
	}
	if m.inboxMarkMode != MarkModeOff {
		hintList = append(hintList, panelrender.Hint{Key: "A", Desc: "mark all"})
		hintList = append(hintList, panelrender.Hint{Key: "D", Desc: "clear"})
	}
	hintList = append(hintList, dispatchHint, panelrender.Hint{Key: "M", Desc: "md"}, panelrender.Hint{Key: "q", Desc: "close"})
	hints := panelrender.HintBar(hintList, boxW-2, pal)
	rows = append(rows, boxRow(hints, boxW, pal.Border))
	rows = append(rows, boxBot(boxW, pal.Border))

	return strings.Join(rows, "\n")
}

// wrapContent wraps each line in text to maxWidth, preserving existing newlines.
// ANSI sequences are stripped before wrapping so escape codes don't count toward width.
func wrapContent(text string, maxWidth int) string {
	if maxWidth <= 0 {
		return text
	}
	var out strings.Builder
	for i, line := range strings.Split(text, "\n") {
		if i > 0 {
			out.WriteByte('\n')
		}
		out.WriteString(wrap.String(stripANSI(line), maxWidth))
	}
	return out.String()
}
