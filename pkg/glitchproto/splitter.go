// Package glitchproto holds the gl1tch agent output protocol: a tiny
// streaming parser that splits raw agent stdout into structured BlockEvents
// based on `<<GLITCH_*>>` / `<<GLITCH_END>>` markers.
//
// The complementary OutputProtocolInstructions string lives in
// plugins/glitchctx (the plugin-side helper module) so plugins can inject
// it into prompts without taking a dependency on the main gl1tch module.
// Both files MUST stay in sync — when you change the marker grammar here,
// update the prompt instructions there.
package glitchproto

import (
	"bytes"
	"fmt"
	"io"
	"strings"
)

// BlockEvent is a single event emitted by StreamSplitter as it parses
// agent output. The lifecycle for one block is:
//
//	BlockStart{Kind: "note", Attrs: {...}}
//	BlockChunk{Text: "first piece"}
//	BlockChunk{Text: "more text"}
//	BlockEnd{}
//
// Free text outside any block is reported as Block="text" Start/Chunk/End
// events too, so consumers can treat it uniformly.
type BlockEvent struct {
	Kind  BlockEventKind
	Block string            // block name: "text", "note", "table", "code", "status"
	Attrs map[string]string // parsed key="value" pairs from the start tag
	Text  string            // chunk text (only for BlockChunk)
}

// BlockEventKind enumerates the lifecycle stages of a block.
type BlockEventKind int

const (
	// BlockStart marks the opening of a new block. Block + Attrs are populated.
	BlockStart BlockEventKind = iota
	// BlockChunk delivers a piece of body text inside the current block.
	BlockChunk
	// BlockEnd marks the end of the current block.
	BlockEnd
)

// StreamSplitter parses agent output incrementally as bytes arrive and
// emits BlockEvents to a callback. It is safe to call Write() with byte
// slices that split a marker mid-tag — the splitter buffers the trailing
// fragment and re-tries on the next Write.
//
// StreamSplitter implements io.WriteCloser, so it can be dropped into any
// existing pipeline that already passes an io.Writer to a child process.
type StreamSplitter struct {
	emit func(BlockEvent)

	// pending holds bytes that may be the start of a marker line. We only
	// commit them downstream once we've confirmed they aren't a marker.
	pending []byte

	// Currently open block. When empty we're in the implicit "text" mode.
	currentBlock string
	currentOpen  bool
}

// NewStreamSplitter constructs a splitter that delivers events to emit.
func NewStreamSplitter(emit func(BlockEvent)) *StreamSplitter {
	return &StreamSplitter{emit: emit}
}

// Write feeds raw bytes from the agent into the splitter. It always
// returns len(p), nil — splitting failures are surfaced as plain text.
func (s *StreamSplitter) Write(p []byte) (int, error) {
	s.pending = append(s.pending, p...)
	s.process(false)
	return len(p), nil
}

// Close flushes any buffered bytes as final text and ends any open block.
// Call this once the underlying stream is done.
func (s *StreamSplitter) Close() error {
	s.process(true)
	if len(s.pending) > 0 {
		s.emitText(string(s.pending))
		s.pending = nil
	}
	if s.currentOpen {
		s.emit(BlockEvent{Kind: BlockEnd, Block: s.currentBlock})
		s.currentOpen = false
		s.currentBlock = ""
	}
	return nil
}

// process drains as many complete lines/markers from the buffer as it can.
// When flush is true, the trailing partial-line buffer is force-emitted as
// text — used by Close to make sure no bytes are lost.
func (s *StreamSplitter) process(flush bool) {
	for {
		// Look for the next newline. Without one we can't safely classify
		// the buffer because a marker may still be incoming.
		nl := bytes.IndexByte(s.pending, '\n')
		if nl < 0 {
			// Partial line. If it can't possibly become a marker, flush it
			// as text now so the user sees output without buffering forever.
			if !flush && couldBeMarker(s.pending) {
				return
			}
			if flush || !couldBeMarker(s.pending) {
				if len(s.pending) > 0 && !couldBeMarker(s.pending) {
					s.emitText(string(s.pending))
					s.pending = nil
				}
			}
			return
		}

		line := string(s.pending[:nl])
		s.pending = s.pending[nl+1:]

		if marker := parseMarker(line); marker != nil {
			s.handleMarker(*marker)
			continue
		}

		// Plain text line — re-attach the newline we consumed.
		s.emitText(line + "\n")
	}
}

// handleMarker reacts to a parsed start/end marker by opening or closing
// the appropriate block, closing any implicit text block first.
func (s *StreamSplitter) handleMarker(m marker) {
	if m.end {
		if s.currentOpen {
			s.emit(BlockEvent{Kind: BlockEnd, Block: s.currentBlock})
			s.currentOpen = false
			s.currentBlock = ""
		}
		return
	}

	// Opening a new block — close any implicit text run first.
	if s.currentOpen {
		s.emit(BlockEvent{Kind: BlockEnd, Block: s.currentBlock})
		s.currentOpen = false
	}
	s.currentBlock = m.block
	s.currentOpen = true
	s.emit(BlockEvent{
		Kind:  BlockStart,
		Block: m.block,
		Attrs: m.attrs,
	})
}

// emitText feeds plain text into either the currently-open block or an
// implicit text block. It batches into BlockStart/BlockChunk events so the
// frontend handles all output uniformly.
func (s *StreamSplitter) emitText(text string) {
	if text == "" {
		return
	}
	if !s.currentOpen {
		s.currentBlock = "text"
		s.currentOpen = true
		s.emit(BlockEvent{Kind: BlockStart, Block: "text"})
	}
	s.emit(BlockEvent{Kind: BlockChunk, Block: s.currentBlock, Text: text})
}

// marker is the parsed form of a single delimiter line.
type marker struct {
	end   bool
	block string            // text|note|table|code|status (lowercase)
	attrs map[string]string // attribute pairs from `key="value"`
}

// parseMarker recognizes lines of the form `<<GLITCH_*>>` (with optional
// attributes). Returns nil if the line is not a marker.
func parseMarker(line string) *marker {
	t := strings.TrimSpace(line)
	if !strings.HasPrefix(t, "<<GLITCH_") || !strings.HasSuffix(t, ">>") {
		return nil
	}
	inner := strings.TrimSuffix(strings.TrimPrefix(t, "<<GLITCH_"), ">>")
	if inner == "" {
		return nil
	}
	if inner == "END" {
		return &marker{end: true}
	}

	// Split off the keyword from any attribute payload. Keyword is the
	// leading run of A-Z / digits, attributes follow whitespace.
	keyword := inner
	rest := ""
	if i := strings.IndexByte(inner, ' '); i >= 0 {
		keyword = inner[:i]
		rest = strings.TrimSpace(inner[i+1:])
	}

	switch keyword {
	case "TEXT", "NOTE", "TABLE", "CODE", "STATUS":
	default:
		// GLITCH_WRITE / GLITCH_RUN belong to the input protocol — they may
		// flow through the splitter on their way back out from a tool
		// harness. Treat them as plain text so they aren't swallowed.
		return nil
	}
	return &marker{
		block: strings.ToLower(keyword),
		attrs: parseAttrs(rest),
	}
}

// parseAttrs extracts key="value" pairs out of the marker payload. Values
// can contain anything except a literal `"`.
func parseAttrs(s string) map[string]string {
	out := map[string]string{}
	i := 0
	for i < len(s) {
		for i < len(s) && (s[i] == ' ' || s[i] == '\t') {
			i++
		}
		if i >= len(s) {
			break
		}
		keyStart := i
		for i < len(s) && s[i] != '=' && s[i] != ' ' {
			i++
		}
		if i >= len(s) || s[i] != '=' {
			break
		}
		key := s[keyStart:i]
		i++
		if i >= len(s) || s[i] != '"' {
			break
		}
		i++
		valStart := i
		for i < len(s) && s[i] != '"' {
			i++
		}
		if i > len(s) {
			break
		}
		val := s[valStart:i]
		if i < len(s) {
			i++
		}
		out[key] = val
	}
	return out
}

// couldBeMarker returns true if the buffered bytes could still grow into a
// marker line. We use this to decide whether to flush a partial line as
// text immediately or wait for more input.
//
// The heuristic is intentionally permissive: if the buffer has no newline
// AND its trimmed prefix is a prefix of `<<GLITCH_`, we keep buffering.
// Anything else gets flushed.
func couldBeMarker(b []byte) bool {
	if len(b) == 0 {
		return false
	}
	t := bytes.TrimLeft(b, " \t")
	const prefix = "<<GLITCH_"
	if len(t) < len(prefix) {
		return bytes.HasPrefix([]byte(prefix), t)
	}
	return bytes.HasPrefix(t, []byte(prefix))
}

// Compile-time assertion that StreamSplitter satisfies io.WriteCloser.
var _ io.WriteCloser = (*StreamSplitter)(nil)

// String makes BlockEvent printable in test failures and logs.
func (e BlockEvent) String() string {
	switch e.Kind {
	case BlockStart:
		return fmt.Sprintf("Start(%s, %v)", e.Block, e.Attrs)
	case BlockChunk:
		return fmt.Sprintf("Chunk(%s, %q)", e.Block, e.Text)
	case BlockEnd:
		return fmt.Sprintf("End(%s)", e.Block)
	default:
		return "Unknown"
	}
}
