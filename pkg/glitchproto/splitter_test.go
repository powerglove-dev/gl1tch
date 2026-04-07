package glitchproto

import (
	"strings"
	"testing"
)

// collect drains all events emitted by feeding the splitter the given chunks
// in order, then closing it. Use multiple chunks to exercise the buffering
// path where a marker straddles a Write() boundary.
func collect(t *testing.T, chunks ...string) []BlockEvent {
	t.Helper()
	var events []BlockEvent
	s := NewStreamSplitter(func(e BlockEvent) {
		// Copy attrs map so test inspection isn't aliased.
		if e.Attrs != nil {
			cp := make(map[string]string, len(e.Attrs))
			for k, v := range e.Attrs {
				cp[k] = v
			}
			e.Attrs = cp
		}
		events = append(events, e)
	})
	for _, c := range chunks {
		if _, err := s.Write([]byte(c)); err != nil {
			t.Fatalf("write: %v", err)
		}
	}
	if err := s.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	return events
}

func summarize(events []BlockEvent) string {
	var b strings.Builder
	for _, e := range events {
		b.WriteString(e.String())
		b.WriteString("\n")
	}
	return b.String()
}

func TestStreamSplitter_PlainText(t *testing.T) {
	got := collect(t, "hello world\n", "second line\n")
	want := "Start(text, map[])\nChunk(text, \"hello world\\n\")\nChunk(text, \"second line\\n\")\nEnd(text)\n"
	if summarize(got) != want {
		t.Errorf("plain text:\nwant: %q\ngot:  %q", want, summarize(got))
	}
}

func TestStreamSplitter_TextBlock(t *testing.T) {
	input := "<<GLITCH_TEXT>>\nhello\n<<GLITCH_END>>\n"
	got := collect(t, input)
	want := "Start(text, map[])\nChunk(text, \"hello\\n\")\nEnd(text)\n"
	if summarize(got) != want {
		t.Errorf("text block:\nwant: %q\ngot:  %q", want, summarize(got))
	}
}

func TestStreamSplitter_NoteWithAttrs(t *testing.T) {
	input := `<<GLITCH_NOTE type="finding" title="Unpinned actions" tags="ci,security">>
40+ actions use @main
<<GLITCH_END>>
`
	got := collect(t, input)
	if len(got) != 3 {
		t.Fatalf("expected 3 events, got %d: %v", len(got), got)
	}
	if got[0].Kind != BlockStart || got[0].Block != "note" {
		t.Errorf("expected start(note), got %v", got[0])
	}
	if got[0].Attrs["type"] != "finding" {
		t.Errorf("type attr: %q", got[0].Attrs["type"])
	}
	if got[0].Attrs["title"] != "Unpinned actions" {
		t.Errorf("title attr: %q", got[0].Attrs["title"])
	}
	if got[0].Attrs["tags"] != "ci,security" {
		t.Errorf("tags attr: %q", got[0].Attrs["tags"])
	}
	if got[1].Kind != BlockChunk || got[1].Text != "40+ actions use @main\n" {
		t.Errorf("chunk: %v", got[1])
	}
	if got[2].Kind != BlockEnd || got[2].Block != "note" {
		t.Errorf("end: %v", got[2])
	}
}

func TestStreamSplitter_MultipleBlocks(t *testing.T) {
	input := "<<GLITCH_STATUS>>\nscanning\n<<GLITCH_END>>\n<<GLITCH_TEXT>>\ndone\n<<GLITCH_END>>\n"
	got := collect(t, input)
	want := "" +
		"Start(status, map[])\n" +
		"Chunk(status, \"scanning\\n\")\n" +
		"End(status)\n" +
		"Start(text, map[])\n" +
		"Chunk(text, \"done\\n\")\n" +
		"End(text)\n"
	if summarize(got) != want {
		t.Errorf("multiple:\nwant: %q\ngot:  %q", want, summarize(got))
	}
}

func TestStreamSplitter_FreeTextBetweenBlocks(t *testing.T) {
	input := "intro line\n<<GLITCH_TEXT>>\ninside\n<<GLITCH_END>>\nouter trailing\n"
	got := collect(t, input)
	want := "" +
		"Start(text, map[])\n" +
		"Chunk(text, \"intro line\\n\")\n" +
		"End(text)\n" +
		"Start(text, map[])\n" +
		"Chunk(text, \"inside\\n\")\n" +
		"End(text)\n" +
		"Start(text, map[])\n" +
		"Chunk(text, \"outer trailing\\n\")\n" +
		"End(text)\n"
	if summarize(got) != want {
		t.Errorf("interleaved:\nwant: %q\ngot:  %q", want, summarize(got))
	}
}

func TestStreamSplitter_SplitAcrossWrites(t *testing.T) {
	// Marker is split mid-tag — splitter must buffer the partial line
	// rather than emit it as plain text.
	got := collect(t, "<<GLITCH_", "TEXT>>\nbody\n<<GLITCH_END", ">>\n")
	want := "" +
		"Start(text, map[])\n" +
		"Chunk(text, \"body\\n\")\n" +
		"End(text)\n"
	if summarize(got) != want {
		t.Errorf("split write:\nwant: %q\ngot:  %q", want, summarize(got))
	}
}

func TestStreamSplitter_PartialFinalText(t *testing.T) {
	// Final byte is text without a trailing newline. Close() must flush.
	got := collect(t, "no newline at end")
	want := "" +
		"Start(text, map[])\n" +
		"Chunk(text, \"no newline at end\")\n" +
		"End(text)\n"
	if summarize(got) != want {
		t.Errorf("partial flush:\nwant: %q\ngot:  %q", want, summarize(got))
	}
}

func TestStreamSplitter_UnknownMarkerIsText(t *testing.T) {
	// `<<GLITCH_RUN>>` belongs to the input protocol — the output splitter
	// should pass it through as plain text rather than treat it as a block.
	got := collect(t, "<<GLITCH_RUN>>\nls\n<<GLITCH_END>>\n")
	if len(got) == 0 || got[0].Kind != BlockStart || got[0].Block != "text" {
		t.Fatalf("expected first event to be Start(text), got %v", got)
	}
	// Ensure RUN was passed through, not treated as a block boundary.
	var buf strings.Builder
	for _, e := range got {
		if e.Kind == BlockChunk {
			buf.WriteString(e.Text)
		}
	}
	if !strings.Contains(buf.String(), "<<GLITCH_RUN>>") {
		t.Errorf("expected RUN marker preserved as text, got %q", buf.String())
	}
}

func TestParseAttrs_BasicPairs(t *testing.T) {
	got := parseAttrs(`type="finding" tags="a,b,c" title="x y z"`)
	if got["type"] != "finding" {
		t.Errorf("type: %q", got["type"])
	}
	if got["tags"] != "a,b,c" {
		t.Errorf("tags: %q", got["tags"])
	}
	if got["title"] != "x y z" {
		t.Errorf("title: %q", got["title"])
	}
}

func TestParseAttrs_EmptyInput(t *testing.T) {
	if got := parseAttrs(""); len(got) != 0 {
		t.Errorf("expected empty, got %v", got)
	}
}
