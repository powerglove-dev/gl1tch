package crontui

import (
	"github.com/sahilm/fuzzy"

	"github.com/8op-org/gl1tch/internal/cron"
)

// applyFilter updates m.filtered based on the current filter input value.
// If the filter is empty, all entries are shown.
func (m *Model) applyFilter() {
	query := m.filterInput.Value()
	if query == "" {
		cp := make([]cron.Entry, len(m.entries))
		copy(cp, m.entries)
		m.filtered = cp
		return
	}

	// Build a string slice of entry names + targets for fuzzy matching.
	sources := make([]string, len(m.entries))
	for i, e := range m.entries {
		sources[i] = e.Name + " " + e.Target
	}

	matches := fuzzy.Find(query, sources)
	result := make([]cron.Entry, 0, len(matches))
	for _, match := range matches {
		result = append(result, m.entries[match.Index])
	}
	m.filtered = result
}

// appendLog appends a line to the ring buffer, dropping the oldest entry if
// the buffer exceeds logBufMax lines.
func (m *Model) appendLog(line string) {
	m.logBuf = append(m.logBuf, line)
	if len(m.logBuf) > logBufMax {
		// Drop from the front to maintain ring-buffer semantics.
		m.logBuf = m.logBuf[len(m.logBuf)-logBufMax:]
	}
}

// clampScrollForList ensures selectedIdx stays in [0, total) and adjusts
// scrollOffset so that selectedIdx is always visible within the visible window.
func (m *Model) clampScrollForList(total, visible int) {
	if total == 0 {
		m.selectedIdx = 0
		m.scrollOffset = 0
		return
	}
	if m.selectedIdx >= total {
		m.selectedIdx = total - 1
	}
	if m.selectedIdx < 0 {
		m.selectedIdx = 0
	}
	// Scroll down if selection is below visible window.
	if m.selectedIdx >= m.scrollOffset+visible {
		m.scrollOffset = m.selectedIdx - visible + 1
	}
	// Scroll up if selection is above visible window.
	if m.selectedIdx < m.scrollOffset {
		m.scrollOffset = m.selectedIdx
	}
	// Ensure scrollOffset doesn't push past the end.
	maxOffset := total - visible
	if maxOffset < 0 {
		maxOffset = 0
	}
	if m.scrollOffset > maxOffset {
		m.scrollOffset = maxOffset
	}
}
