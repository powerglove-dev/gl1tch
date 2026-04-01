// Package clarify provides the Detector interface and registry for reactive
// agent clarification. Each executor type registers a Detector that recognises
// its output convention for requesting user input. The pipeline runner injects
// Instruction into prompts for registered executors, and the switchboard
// intercepts matching log lines to surface the clarification overlay.
package clarify

import (
	"strings"
	"sync"

	"github.com/8op-org/gl1tch/internal/systemprompts"
)

var (
	instructionOnce sync.Once
	instructionVal  string
)

// Instruction returns the clarify system prompt, loaded from
// ~/.config/glitch/prompts/clarify.md with embedded fallback.
// The result is cached after the first call.
func Instruction() string {
	instructionOnce.Do(func() {
		instructionVal = systemprompts.Load(systemprompts.Clarify)
	})
	return instructionVal
}

// Detector inspects a single log line for a clarification request.
type Detector interface {
	Detect(line string) (question string, found bool)
}

// NoOp is the default Detector — it never matches. Pipelines using executors
// without a registered Detector run to completion without interruption.
type NoOp struct{}

func (NoOp) Detect(string) (string, bool) { return "", false }

// StructuredDetector matches "GLITCH_CLARIFY: <question>" anywhere in a line,
// after stripping ANSI escape sequences. This is the standard convention for
// all AI executor types (claude, opencode, ollama, gemini, etc.).
type StructuredDetector struct{}

const marker = "GLITCH_CLARIFY:"

func (StructuredDetector) Detect(line string) (string, bool) {
	clean := stripANSI(line)
	idx := strings.Index(clean, marker)
	if idx < 0 {
		return "", false
	}
	q := strings.TrimSpace(clean[idx+len(marker):])
	if q == "" {
		return "", false
	}
	return q, true
}

// stripANSI removes ANSI CSI escape sequences from s.
func stripANSI(s string) string {
	var out strings.Builder
	i := 0
	for i < len(s) {
		if s[i] == '\x1b' && i+1 < len(s) && s[i+1] == '[' {
			i += 2
			for i < len(s) && s[i] != 'm' {
				i++
			}
			i++ // consume 'm'
			continue
		}
		out.WriteByte(s[i])
		i++
	}
	return out.String()
}

// registry maps executor IDs to their Detector.
var registry = map[string]Detector{}

// Register associates executorID with d in the global registry.
// Call from init() or package startup; not goroutine-safe after init.
func Register(executorID string, d Detector) { registry[executorID] = d }

// Get returns the Detector for executorID, or NoOp{} if unregistered.
func Get(executorID string) Detector {
	if d, ok := registry[executorID]; ok {
		return d
	}
	return NoOp{}
}

// IsReactive reports whether executorID has an active Detector registered.
func IsReactive(executorID string) bool {
	_, ok := registry[executorID]
	return ok
}

func init() {
	// All standard AI executor types use the GLITCH_CLARIFY: structured protocol.
	for _, id := range []string{"claude", "opencode", "ollama", "gemini", "github-copilot"} {
		Register(id, StructuredDetector{})
	}
}
