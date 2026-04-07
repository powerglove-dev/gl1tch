package glitchproto

import "io"

// NewTextOnlyWriter returns an io.WriteCloser that feeds every byte
// written to it through a StreamSplitter and forwards only the body
// text of parsed blocks to out. All `<<GLITCH_*>>` / `<<GLITCH_END>>`
// marker lines are dropped on the floor; the block kind is ignored,
// so prose, code, notes, tables, and status pings all collapse into
// a single clean stream of content bytes.
//
// This is the reusable "I just want the agent's content, not the
// output protocol scaffolding" primitive. Use it anywhere a caller
// renders or persists provider output as plain text/markdown/YAML —
// draft refinement, workflow step output capture, copy-paste
// destinations, etc. For rich rendering that cares about block kinds
// (the desktop chat view), call NewStreamSplitter directly with a
// custom emit callback instead.
//
// Composition note: every provider plugin currently prepends
// OutputProtocolInstructions to its prompt unconditionally, so
// models wrap replies in `<<GLITCH_TEXT>>` blocks even when the
// caller doesn't want structured output. That's a producer-side
// smell — the right long-term fix is to plumb an output-mode flag
// through StreamPromptOpts and the plugin boundary so plain-output
// callers can opt out of the protocol entirely. Until then, this
// writer lets any consumer get clean bytes with one line of glue.
func NewTextOnlyWriter(out io.Writer) io.WriteCloser {
	return &textOnlyWriter{
		out: out,
		splitter: NewStreamSplitter(func(ev BlockEvent) {
			if ev.Kind != BlockChunk || ev.Text == "" {
				return
			}
			// Writer errors are intentionally swallowed here: the
			// splitter's emit callback has no way to surface them,
			// and the downstream io.Writer is typically a buffer
			// or channel adapter that either always succeeds or
			// has already failed and will surface its own error
			// through the surrounding context (ctx cancel, chan
			// close, etc.).
			_, _ = out.Write([]byte(ev.Text))
		}),
	}
}

// textOnlyWriter is the concrete io.WriteCloser returned by
// NewTextOnlyWriter. It exists as a small wrapper so the returned
// Close() flushes the underlying splitter without exposing its
// internals to callers.
type textOnlyWriter struct {
	out      io.Writer
	splitter *StreamSplitter
}

func (w *textOnlyWriter) Write(p []byte) (int, error) {
	// StreamSplitter.Write always consumes the full slice — its
	// contract is "splitting failures surface as plain text" — so
	// reporting len(p) back to the caller matches the upstream
	// contract and lets callers compose us with io.MultiWriter
	// without bookkeeping the short-write case.
	return w.splitter.Write(p)
}

func (w *textOnlyWriter) Close() error {
	// Closing the splitter flushes any pending buffered bytes as a
	// final text chunk AND closes any still-open block, so no
	// content is lost at end-of-stream.
	return w.splitter.Close()
}

var _ io.WriteCloser = (*textOnlyWriter)(nil)
