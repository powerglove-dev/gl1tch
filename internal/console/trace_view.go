package console

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/8op-org/gl1tch/internal/styles"
)

type traceSpan struct {
	Name       string
	DurationMS int64
	OK         bool
	Depth      int
}

func loadTraceForRun(tracesPath, runID string) ([]traceSpan, error) {
	f, err := os.Open(tracesPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer f.Close()

	var spans []traceSpan
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
		if line == "" {
			continue
		}
		var raw map[string]any
		if err := json.Unmarshal([]byte(line), &raw); err != nil {
			continue
		}
		if !spanMatchesRun(raw, runID) {
			continue
		}
		spans = append(spans, spanFromRaw(raw))
	}
	return spans, sc.Err()
}

func spanMatchesRun(raw map[string]any, runID string) bool {
	attrs, _ := raw["Attributes"].([]any)
	for _, a := range attrs {
		m, _ := a.(map[string]any)
		if m["Key"] == "run.id" {
			val, _ := m["Value"].(map[string]any)
			if v, _ := val["Value"].(string); v == runID {
				return true
			}
		}
	}
	return false
}

func spanFromRaw(raw map[string]any) traceSpan {
	name, _ := raw["Name"].(string)
	status, _ := raw["Status"].(map[string]any)
	code, _ := status["Code"].(string)
	return traceSpan{
		Name: name,
		OK:   code != "Error",
	}
}

func renderTraceTree(spans []traceSpan, _ int) string {
	if len(spans) == 0 {
		return styles.Dimmed.Render("no trace data for this run")
	}
	var sb strings.Builder
	okStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#50fa7b"))
	errStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#ff5555"))
	dimStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#6272a4"))
	for _, s := range spans {
		indent := strings.Repeat("  ", s.Depth)
		status := okStyle.Render("OK")
		if !s.OK {
			status = errStyle.Render("ERR")
		}
		dur := ""
		if s.DurationMS > 0 {
			dur = dimStyle.Render(fmt.Sprintf(" · %dms", s.DurationMS))
		}
		sb.WriteString(fmt.Sprintf("%s%s%s %s\n", indent, s.Name, dur, status))
	}
	return sb.String()
}

// tracesFilePath returns the path for the OTel traces JSONL file.
// Mirrors the logic in internal/telemetry/telemetry.go.
func tracesFilePath() string {
	base := os.Getenv("XDG_DATA_HOME")
	if base == "" {
		home, _ := os.UserHomeDir()
		base = home + "/.local/share"
	}
	return base + "/glitch/traces.jsonl"
}
