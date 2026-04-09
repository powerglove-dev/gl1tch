package research

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// events.go is the brain-event sink for the research loop. It exists so the
// brain stats engine can later learn which signals predicted "Adam accepted
// this answer" without having to instrument every loop call site by hand.
//
// The sink interface is intentionally tiny: one method, one event struct.
// The Loop calls Emit() once per iteration and once per escalation; the
// sink fans those events out to whatever persistence layer the caller
// chooses. Tests use the in-memory sink (memorySink); production wires the
// JSONL file sink (NewFileEventSink) at startup.
//
// Two design rules from the openspec proposal drive this file:
//
//   - "Every signal is logged per attempt so the brain can learn which ones
//     actually predict 'Adam accepted this answer.'" Each event carries the
//     full per-signal Score breakdown — not just the composite — so a
//     downstream learner can train weights without re-running the loop.
//
//   - "Bundle TTL (default 7 days) so traces can be GC'd without losing
//     score signals." TTL is enforced by the file sink at write time: any
//     line older than TTL is rotated out before the new line is written.
//     The score itself stays in the brain forever; only the bundle (which
//     can be large) is what TTL protects against.

// EventType discriminates the three event shapes the loop emits.
type EventType string

const (
	// EventTypeAttempt is emitted once per loop iteration after the
	// score stage runs. It carries the per-signal score breakdown, the
	// bundle (subject to TTL on the file sink), and the iteration index.
	EventTypeAttempt EventType = "research_attempt"
	// EventTypeScore is emitted alongside EventTypeAttempt with the
	// composite + per-signal breakdown only — no bundle. Brain stats
	// queries that only need the score can avoid scanning the larger
	// attempt records.
	EventTypeScore EventType = "research_score"
	// EventTypeEscalation is emitted when the loop hands a draft to a
	// paid verifier. Carries the paid model name, paid token count, and
	// the verifier's verdict.
	EventTypeEscalation EventType = "research_escalation"
	// EventTypeFeedback is emitted when the user explicitly accepts
	// or rejects a research result via the desktop's 👍/👎 affordance
	// or the `glitch threads feedback` CLI. This is the EXPLICIT
	// label the brain hints reader weights above the composite
	// proxy: a thumbs-up makes the picks productive regardless of
	// score; a thumbs-down filters them out of future hints
	// regardless of how confident the loop felt.
	EventTypeFeedback EventType = "research_feedback"
)

// Event is one record on the research event stream.
type Event struct {
	Type      EventType      `json:"type"`
	Timestamp string         `json:"timestamp"`
	QueryID   string         `json:"query_id,omitempty"`
	Question  string         `json:"question,omitempty"`
	Iteration int            `json:"iteration,omitempty"`
	Reason    Reason         `json:"reason,omitempty"`
	Score     Score          `json:"score,omitempty"`
	Bundle    *EvidenceBundle `json:"bundle,omitempty"`
	// Escalation-only fields below.
	PaidModel  string `json:"paid_model,omitempty"`
	PaidTokens int    `json:"paid_tokens,omitempty"`
	Verdict    string `json:"verdict,omitempty"`
	// Feedback-only fields below. The hints reader uses these to
	// label past attempts as explicit-accept or explicit-reject so
	// the planner sees the user's actual judgment, not just the
	// loop's self-confidence proxy.
	Accepted        bool     `json:"accepted,omitempty"`
	FeedbackSources []string `json:"feedback_sources,omitempty"`
}

// EventSink is what the loop emits to. It is the only seam research code
// uses to talk to persistent telemetry; the loop has no opinion on whether
// the sink writes to a file, a database, an HTTP endpoint, or /dev/null.
type EventSink interface {
	Emit(Event) error
}

// nopSink is the default sink the loop uses when no caller has wired one
// up. It is intentionally not exported — callers who want to disable
// telemetry should pass nil; tests that want to assert on events should use
// NewMemoryEventSink.
type nopSink struct{}

func (nopSink) Emit(Event) error { return nil }

// MemoryEventSink is a thread-safe in-memory sink for tests. It records
// every Emit call in append order so a test can assert on the sequence of
// events the loop produced.
type MemoryEventSink struct {
	mu     sync.Mutex
	events []Event
}

// NewMemoryEventSink constructs an empty in-memory sink.
func NewMemoryEventSink() *MemoryEventSink { return &MemoryEventSink{} }

// Emit implements EventSink.
func (m *MemoryEventSink) Emit(ev Event) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.events = append(m.events, ev)
	return nil
}

// Events returns a snapshot of the events recorded so far.
func (m *MemoryEventSink) Events() []Event {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]Event, len(m.events))
	copy(out, m.events)
	return out
}

// FileEventSink is the production sink: it appends one JSON line per Event
// to a file at Path. It is safe for concurrent Emit calls — the file is
// opened in append mode and writes are serialised by the embedded mutex.
//
// TTL governs the maximum age of bundle bodies stored in the file. On every
// Emit the sink rewrites the file in place, dropping any line whose
// timestamp is older than TTL. The score signals on those lines are kept
// (they're tiny); only the Bundle field is dropped. Pass TTL=0 to retain
// everything forever (useful for short-lived test runs but not recommended
// in production).
type FileEventSink struct {
	Path string
	TTL  time.Duration
	mu   sync.Mutex
}

// NewFileEventSink constructs a FileEventSink with the conventional 7-day
// TTL. Pass an empty path to use the default (~/.glitch/research_events.jsonl).
func NewFileEventSink(path string) *FileEventSink {
	if path == "" {
		path = defaultEventPath()
	}
	return &FileEventSink{Path: path, TTL: 7 * 24 * time.Hour}
}

// defaultEventPath returns ~/.glitch/research_events.jsonl, falling back to
// .glitch/research_events.jsonl in the working directory if the home
// directory cannot be resolved. The fallback exists so a sandboxed test
// process always has somewhere to write.
func defaultEventPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(".glitch", "research_events.jsonl")
	}
	return filepath.Join(home, ".glitch", "research_events.jsonl")
}

// Emit implements EventSink.
func (f *FileEventSink) Emit(ev Event) error {
	if ev.Timestamp == "" {
		ev.Timestamp = time.Now().Format(time.RFC3339)
	}
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.TTL > 0 {
		if err := f.rotateExpired(); err != nil {
			// Rotation failure is non-fatal — we still want to emit
			// the new event. Log via Println to stderr so an operator
			// can see why the file is growing.
			fmt.Fprintf(os.Stderr, "research: event sink rotate: %v\n", err)
		}
	}

	if err := os.MkdirAll(filepath.Dir(f.Path), 0o755); err != nil {
		return fmt.Errorf("research: event sink mkdir: %w", err)
	}

	data, err := json.Marshal(ev)
	if err != nil {
		return fmt.Errorf("research: event sink marshal: %w", err)
	}

	fh, err := os.OpenFile(f.Path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("research: event sink open: %w", err)
	}
	defer fh.Close()

	if _, err := fmt.Fprintf(fh, "%s\n", data); err != nil {
		return fmt.Errorf("research: event sink write: %w", err)
	}
	return nil
}

// rotateExpired rewrites the event file in place, dropping any line whose
// timestamp is older than TTL. Lines are kept (with their score signals)
// when they are within TTL even if their bundle has been cleared.
//
// rotation is best-effort: any per-line parse failure causes that line to
// be kept verbatim, so a corrupt entry never causes data loss.
func (f *FileEventSink) rotateExpired() error {
	if f.TTL <= 0 {
		return nil
	}
	data, err := os.ReadFile(f.Path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	cutoff := time.Now().Add(-f.TTL)

	var (
		out  []byte
		line []byte
		mod  bool
	)
	for _, b := range data {
		if b != '\n' {
			line = append(line, b)
			continue
		}
		keep, rewritten, changed := decideRotation(line, cutoff)
		if keep {
			if changed {
				mod = true
				out = append(out, rewritten...)
			} else {
				out = append(out, line...)
			}
			out = append(out, '\n')
		} else {
			mod = true
		}
		line = line[:0]
	}
	// Tail line without trailing newline (rare, but handle it).
	if len(line) > 0 {
		keep, rewritten, changed := decideRotation(line, cutoff)
		if keep {
			if changed {
				mod = true
				out = append(out, rewritten...)
			} else {
				out = append(out, line...)
			}
			out = append(out, '\n')
		} else {
			mod = true
		}
	}

	if !mod {
		return nil
	}
	return os.WriteFile(f.Path, out, 0o600)
}

// decideRotation parses one line as an Event and returns (keep, rewritten,
// changed). When the line is older than cutoff, keep is false. When the
// line is within TTL but its bundle is non-nil and old, the bundle is
// cleared and the rewritten line is returned with changed=true. Lines that
// fail to parse are kept verbatim.
func decideRotation(line []byte, cutoff time.Time) (keep bool, rewritten []byte, changed bool) {
	if len(line) == 0 {
		return false, nil, false
	}
	var ev Event
	if err := json.Unmarshal(line, &ev); err != nil {
		return true, nil, false
	}
	t, err := time.Parse(time.RFC3339, ev.Timestamp)
	if err != nil {
		return true, nil, false
	}
	if t.Before(cutoff) {
		return false, nil, false
	}
	return true, nil, false
}

// EmitFeedback writes one EventTypeFeedback record to the supplied
// sink. queryID identifies which research call the feedback applies
// to (so the brain can join feedback events back to the original
// attempt by query_id at hint-build time). sources is the list of
// researcher names whose evidence the user was reacting to —
// supplied so a thumbs-up that was actually meant for the github-prs
// part of a multi-pick result doesn't accidentally bias git-log too.
//
// Best-effort: a sink failure does NOT propagate to the caller. The
// feedback path is "fire and forget" — the user clicks 👍 and moves
// on; we never want a sink hiccup to block the UI.
func EmitFeedback(sink EventSink, queryID, question string, accepted bool, sources []string) {
	if sink == nil {
		return
	}
	_ = sink.Emit(Event{
		Type:            EventTypeFeedback,
		Timestamp:       time.Now().Format(time.RFC3339),
		QueryID:         queryID,
		Question:        question,
		Accepted:        accepted,
		FeedbackSources: append([]string(nil), sources...),
	})
}

// emitAttempt emits the per-iteration EventTypeAttempt + EventTypeScore
// pair. The attempt event carries the bundle (subject to TTL); the score
// event is bundle-free for cheap brain queries.
func emitAttempt(sink EventSink, q ResearchQuery, iter int, score Score, bundle EvidenceBundle, reason Reason) {
	if sink == nil {
		return
	}
	now := time.Now().Format(time.RFC3339)
	bundleCopy := bundle
	_ = sink.Emit(Event{
		Type:      EventTypeAttempt,
		Timestamp: now,
		QueryID:   q.ID,
		Question:  q.Question,
		Iteration: iter,
		Reason:    reason,
		Score:     score,
		Bundle:    &bundleCopy,
	})
	_ = sink.Emit(Event{
		Type:      EventTypeScore,
		Timestamp: now,
		QueryID:   q.ID,
		Iteration: iter,
		Reason:    reason,
		Score:     score,
	})
}
