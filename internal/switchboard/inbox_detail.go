package switchboard

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"

	"github.com/adam-stokes/orcai/internal/panelrender"
	"github.com/adam-stokes/orcai/internal/store"
	"github.com/adam-stokes/orcai/internal/styles"
)

type runMeta struct {
	PipelineFile string `json:"pipeline_file"`
	CWD          string `json:"cwd"`
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

	// Metadata: pipeline file and cwd
	meta := parseRunMetadata(run.Metadata)
	if meta.PipelineFile != "" {
		sb.WriteString(dim("pipeline: ") + fg(collapseTilde(meta.PipelineFile)) + "\n")
	}
	if meta.CWD != "" {
		sb.WriteString(dim("cwd:      ") + fg(collapseTilde(meta.CWD)) + "\n")
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
		sb.WriteString(run.Stderr)
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
	for _, line := range visible {
		rows = append(rows, boxRow(" "+line, boxW, pal.Border))
	}
	// Pad to fill the box height
	for i := len(visible); i < visibleH; i++ {
		rows = append(rows, boxRow("", boxW, pal.Border))
	}

	hints := panelrender.HintBar([]panelrender.Hint{
		{Key: "j/k", Desc: "scroll"},
		{Key: "n/p", Desc: "next/prev"},
		{Key: "m", Desc: "toggle md"},
		{Key: "q", Desc: "close"},
	}, boxW-2, pal)
	rows = append(rows, boxRow(hints, boxW, pal.Border))
	rows = append(rows, boxBot(boxW, pal.Border))

	return strings.Join(rows, "\n")
}
