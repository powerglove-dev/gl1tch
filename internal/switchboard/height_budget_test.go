package switchboard

import (
	"strings"
	"testing"
)

func TestViewLeftColumnFitsHeight(t *testing.T) {
	m := NewWithPipelines([]string{"pipeline-a", "pipeline-b"})
	for _, h := range []int{20, 24, 40, 50} {
		result := m.viewLeftColumn(h, 40)
		lines := strings.Split(result, "\n")
		if len(lines) != h {
			t.Errorf("height=%d: got %d lines", h, len(lines))
		}
	}
}

func TestViewLeftColumnNoPanic_SmallHeight(t *testing.T) {
	m := NewWithPipelines([]string{"pipeline-a"})
	// Should not panic even with tiny heights.
	for _, h := range []int{5, 8, 10} {
		_ = m.viewLeftColumn(h, 40)
	}
}
