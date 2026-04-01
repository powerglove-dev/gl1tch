package console

import (
	"testing"

	"github.com/8op-org/gl1tch/internal/themes"
)

// TestDynamicHeader_FullWidth verifies that every line returned by DynamicHeader
// has a visible width exactly equal to the requested panel width.
func TestDynamicHeader_FullWidth(t *testing.T) {
	b := &themes.Bundle{
		HeaderStyle: themes.HeaderStyle{
			Panels: map[string]themes.PanelHeaderStyle{
				"pipelines": {Accent: "#bd93f9", Text: "#282a36"},
			},
		},
	}
	for _, w := range []int{80, 120, 200, 220} {
		lines := DynamicHeader(b, "pipelines", w, "", "")
		if lines == nil {
			t.Fatalf("width %d: DynamicHeader returned nil", w)
		}
		for i, line := range lines {
			got := visibleWidth(line)
			if got != w {
				t.Errorf("width %d line %d: visible width = %d, want %d", w, i, got, w)
			}
		}
	}
}
