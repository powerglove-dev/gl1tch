package glitchproto

import (
	"bytes"
	"io"
	"regexp"
	"strings"
)

// NewContentOnlyWriter returns an io.WriteCloser that strips every
// known layer of gl1tch sidecar / telemetry noise from provider
// output, leaving only the model's actual content. Use it anywhere a
// surface needs clean markdown / YAML / plain text — draft
// refinement, workflow step capture, copy-paste destinations — and
// does NOT care about block structure, brain notes, or stats.
//
// What gets stripped, in addition to everything NewTextOnlyWriter
// already handles:
//
//   - <brain type="…" tags="…" title="…">…</brain> capture blocks
//     (possibly multi-line; the body is dropped entirely because
//     content-only consumers don't render brain notes)
//   - {"type":"gl1tch-stats", …} JSON telemetry sentinels plugins
//     emit at end-of-run so the game engine can score the step
//   - [wrote: path] GLITCH_WRITE acknowledgements from file-writing
//     tool calls
//   - a leading `<>` arrow marker plugins sometimes print ahead of
//     the stats blob
//
// Architecture:
//
//	raw bytes
//	  └─► NewTextOnlyWriter (drops <<GLITCH_*>> markers)
//	        └─► contentScrubber (line-buffered sidecar strip)
//	              └─► out
//
// Stripping rules mirror glitch-desktop/frontend/src/lib/parseAgentOutput.ts
// so surfaces that render body content from Go match what the
// desktop chat renders from TypeScript. When you change the rules
// here, update the TS parser too (or vice-versa); the two must
// agree so a prompt refined in the draft editor produces the same
// body the chat view would have shown for the same raw stream.
//
// Design note: this is the "I don't care about any sidecar signals,
// just the body" primitive. Callers that DO care — chat renders
// brain notes as coloured cards, stats as a summary footer, tool
// calls as a collapsible trace — should not use this; they should
// pull the structured pieces out at the rendering layer (today via
// parseAgentOutput.ts on the TS side, someday via a matching Go
// helper that returns both body and sidecars).
func NewContentOnlyWriter(out io.Writer) io.WriteCloser {
	scrubber := &contentScrubber{out: out}
	// Chain the GLITCH marker splitter on top of the line scrubber:
	// the splitter feeds clean body bytes into the scrubber, which
	// then runs its line-based sidecar strip on the way to `out`.
	splitter := NewTextOnlyWriter(scrubber)
	return &contentOnlyWriter{splitter: splitter, scrubber: scrubber}
}

// contentOnlyWriter is the public-facing WriteCloser returned by
// NewContentOnlyWriter. It owns both the splitter (which handles
// GLITCH markers) and the scrubber (which handles everything else)
// and makes sure Close() flushes them in the right order: splitter
// first so any buffered text lands in the scrubber, then scrubber
// so any buffered partial line lands in `out`.
type contentOnlyWriter struct {
	splitter io.WriteCloser
	scrubber *contentScrubber
}

func (c *contentOnlyWriter) Write(p []byte) (int, error) {
	return c.splitter.Write(p)
}

func (c *contentOnlyWriter) Close() error {
	// Flushing the splitter first forces any trailing text the
	// GLITCH parser was buffering into the scrubber, which then
	// needs its own flush to emit any partial final line.
	if err := c.splitter.Close(); err != nil {
		return err
	}
	return c.scrubber.Close()
}

var _ io.WriteCloser = (*contentOnlyWriter)(nil)

// Scrubber regexes. Kept at package scope so they're compiled once
// at program start rather than per-writer. These mirror the rules in
// frontend/src/lib/parseAgentOutput.ts — when you change one, grep
// for the other and update it too.

// statsRe matches the JSON telemetry sentinel plugins emit at the
// end of each step. The `[^{}]*` guards ensure we don't accidentally
// swallow a well-formed JSON object from the model's own reply that
// happens to contain the word "gl1tch-stats" inside a nested
// structure; the sentinel plugins emit is always flat.
var statsRe = regexp.MustCompile(`\{[^{}]*"type"\s*:\s*"gl1tch-stats"[^{}]*\}`)

// wroteRe matches a GLITCH_WRITE acknowledgement. Plugins emit this
// inline after a file write succeeds, e.g. `[wrote: internal/foo.go]`.
var wroteRe = regexp.MustCompile(`\[wrote:\s*[^\]]+\]`)

// arrowPrefixRe matches the `<>` marker that sometimes prefixes the
// stats blob. It's line-anchored so we don't strip legitimate uses
// of `<>` deeper in the content (e.g. inside a prompt body that
// talks about the marker).
var arrowPrefixRe = regexp.MustCompile(`^\s*<>\s*`)

// brainOpenRe matches a `<brain …>` opening tag with any attribute
// payload. We match the literal tag rather than trying to validate
// attributes so a malformed tag from a misbehaving model still gets
// stripped rather than leaking through.
var brainOpenRe = regexp.MustCompile(`<brain\b[^>]*>`)

const brainCloseTag = "</brain>"

// contentScrubber is a line-buffered filter that removes brain
// capture blocks, stats JSON sentinels, [wrote:] acks, and `<>`
// prefixes from its input before forwarding the remainder to out.
//
// Why line-buffered: brain blocks and stats sentinels are emitted
// as line-aligned output in practice, and the scrubber needs a
// whole line in hand to decide whether to drop or keep it. We still
// emit as soon as each line is complete so a draft-editor surface
// sees tokens stream in roughly in sync with the model, rather than
// waiting for Close() to release a giant blob.
//
// State: inBrain flips true when we see `<brain …>` without a
// matching close on the same line, and back to false when we see
// `</brain>`. While true, all input is dropped on the floor.
type contentScrubber struct {
	out     io.Writer
	pending []byte
	inBrain bool
}

func (s *contentScrubber) Write(p []byte) (int, error) {
	s.pending = append(s.pending, p...)
	for {
		nl := bytes.IndexByte(s.pending, '\n')
		if nl < 0 {
			break
		}
		// Include the newline in the line we process so the scrub
		// output preserves line breaks without our having to
		// re-inject them after the fact.
		line := string(s.pending[:nl+1])
		s.pending = s.pending[nl+1:]
		if out := s.scrubLine(line); out != "" {
			if _, err := s.out.Write([]byte(out)); err != nil {
				return 0, err
			}
		}
	}
	return len(p), nil
}

// Close flushes any trailing partial line (no newline) as a final
// scrub pass. We DO NOT close the downstream out here — the
// contentOnlyWriter wrapper owns out's lifecycle via its own Close.
func (s *contentScrubber) Close() error {
	if len(s.pending) == 0 {
		return nil
	}
	line := string(s.pending)
	s.pending = nil
	if out := s.scrubLine(line); out != "" {
		if _, err := s.out.Write([]byte(out)); err != nil {
			return err
		}
	}
	return nil
}

// scrubLine applies every sidecar strip rule to a single line and
// returns the cleaned output. An empty return value means "this
// whole line is noise — drop it entirely", which is different from
// returning `"\n"` (meaning "this line was meaningful content that
// happened to be blank and should render as a paragraph break").
//
// The mutation-gate matters: we must never drop a legitimate blank
// line from the model's output (markdown paragraph breaks, YAML
// document separators, etc.) just because it looks the same as a
// line we scrubbed down to empty. We only drop a line when this
// function actively removed content from it.
func (s *contentScrubber) scrubLine(line string) string {
	original := line

	// Brain state machine. When we're inside a brain block, drop
	// everything until the close tag; then fall through to scrub
	// whatever tail followed the close tag on the same line.
	if s.inBrain {
		idx := strings.Index(line, brainCloseTag)
		if idx < 0 {
			// Whole line is brain-body noise.
			return ""
		}
		s.inBrain = false
		line = line[idx+len(brainCloseTag):]
	} else if m := brainOpenRe.FindStringIndex(line); m != nil {
		// A brain block started on this line. Keep whatever came
		// before the opening tag; the body (and possibly a
		// same-line close) is sidecar content that gets dropped.
		pre := line[:m[0]]
		rest := line[m[1]:]
		if idx := strings.Index(rest, brainCloseTag); idx >= 0 {
			// Same-line open-and-close: concatenate the pre and
			// post-close portions and keep processing.
			line = pre + rest[idx+len(brainCloseTag):]
		} else {
			// Multi-line brain: emit the pre-tag content (which
			// may be empty) and mark the state so subsequent
			// lines are dropped until the close arrives.
			s.inBrain = true
			line = pre
		}
	}

	// Inline strips — order is not load-bearing, but we do stats
	// before wrote so a stats blob that happens to contain a
	// [wrote:] substring inside its model field (unlikely but
	// possible) doesn't trip the wrote regex on the residue.
	line = statsRe.ReplaceAllString(line, "")
	line = wroteRe.ReplaceAllString(line, "")
	line = arrowPrefixRe.ReplaceAllString(line, "")

	// Mutation gate: if the scrub changed the line and what's
	// left is only whitespace, the line was a pure sidecar
	// sentinel (stats JSON, arrow marker, brain opener with no
	// prefix, etc.) and should disappear entirely. If the line
	// is unchanged, it's the model's own blank line and must be
	// preserved so paragraph breaks and document separators
	// survive the round trip.
	if line != original && strings.TrimSpace(line) == "" {
		return ""
	}
	return line
}

var _ io.Writer = (*contentScrubber)(nil)
