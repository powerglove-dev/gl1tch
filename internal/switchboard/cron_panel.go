package switchboard

import (
	"fmt"
	"sort"
	"strings"

	orcaicron "github.com/adam-stokes/orcai/internal/cron"
)

// CronPanel holds display state for the CRON JOBS panel.
type CronPanel struct {
	selectedIdx  int
	scrollOffset int
	focused      bool
	filterQuery  string
	filterActive bool
}

// cronEntry pairs a cron Entry with its formatted next-run string.
type cronEntry struct {
	e    orcaicron.Entry
	next string
}

// filteredCronEntries returns sorted cron entries filtered by the cron panel's search query.
func (m Model) filteredCronEntries() []cronEntry {
	entries, err := orcaicron.LoadConfig()
	if err != nil || len(entries) == 0 {
		return nil
	}
	sorted := make([]cronEntry, 0, len(entries))
	for _, e := range entries {
		t, err := orcaicron.NextRun(e)
		rel := ""
		if err == nil {
			rel = orcaicron.FormatRelative(t)
		}
		sorted = append(sorted, cronEntry{e: e, next: rel})
	}
	sort.Slice(sorted, func(i, j int) bool {
		ti, ei := orcaicron.NextRun(sorted[i].e)
		tj, ej := orcaicron.NextRun(sorted[j].e)
		if ei != nil || ej != nil {
			return false
		}
		return ti.Before(tj)
	})
	query := strings.ToLower(m.cronPanel.filterQuery)
	if query == "" {
		return sorted
	}
	var out []cronEntry
	for _, item := range sorted {
		if strings.Contains(strings.ToLower(item.e.Name), query) {
			out = append(out, item)
		}
	}
	return out
}

// buildCronSection renders the CRON JOBS box as a slice of ANSI lines.
func (m Model) buildCronSection(w, height int) []string {
	pal := m.ansiPalette()
	borderColor := pal.Border
	if m.cronPanel.focused {
		borderColor = pal.Accent
	}

	var rows []string
	if sprite := PanelHeader(m.activeBundle(), "cron", w, borderColor); sprite != nil {
		rows = append(rows, sprite...)
	} else {
		rows = append(rows, boxTop(w, RenderHeader("cron"), borderColor, pal.Accent))
	}

	// maxRows: remaining content rows after header, leaving 1 for boxBot.
	maxRows := height - len(rows) - 1
	if maxRows < 0 {
		maxRows = 0
	}

	// Search prompt row.
	if m.cronPanel.filterActive {
		cursor := "█"
		prompt := fmt.Sprintf("  %s/%s %s%s%s%s", pal.Accent, aRst, pal.FG, m.cronPanel.filterQuery, cursor, aRst)
		rows = append(rows, boxRow(prompt, w, borderColor))
		maxRows--
		if maxRows < 0 {
			maxRows = 0
		}
	}

	sorted := m.filteredCronEntries()
	if len(sorted) == 0 {
		rows = append(rows, boxRow(pal.Dim+"  no scheduled jobs"+aRst, w, borderColor))
		// Pad and close.
		for len(rows) < height-1 {
			rows = append(rows, boxRow("", w, borderColor))
		}
		rows = append(rows, boxBot(w, borderColor))
		return rows[:min(len(rows), height)]
	}

	offset := m.cronPanel.scrollOffset
	shown := 0
	for i := offset; i < len(sorted) && shown < maxRows; i++ {
		item := sorted[i]
		name := item.e.Name
		kind := item.e.Kind
		sched := item.e.Schedule
		rel := item.next

		content := fmt.Sprintf("  %s  %s  %s  %s%s%s",
			pal.FG+name+aRst,
			pal.Dim+kind+aRst,
			pal.Dim+sched+aRst,
			pal.Accent, rel, aRst,
		)

		if m.cronPanel.focused && i == m.cronPanel.selectedIdx {
			highlightContent := fmt.Sprintf("  %s%s%s  %s  %s  %s%s%s",
				pal.Accent, name, aRst,
				pal.Dim+kind+aRst,
				pal.Dim+sched+aRst,
				pal.Accent, rel, aRst,
			)
			rows = append(rows, boxRow(highlightContent, w, borderColor))
		} else {
			rows = append(rows, boxRow(content, w, borderColor))
		}
		shown++
	}

	// Pad to fill allocated height so panels below stay at a fixed position.
	for len(rows) < height-1 {
		rows = append(rows, boxRow("", w, borderColor))
	}
	rows = append(rows, boxBot(w, borderColor))
	if len(rows) > height {
		rows = rows[:height]
	}
	return rows
}
