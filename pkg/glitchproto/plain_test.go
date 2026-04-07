package glitchproto

import (
	"bytes"
	"io"
	"strings"
	"testing"
)

// writePlain feeds chunks through a TextOnlyWriter and returns the
// bytes that survived marker stripping. Writing in multiple chunks
// exercises the same marker-straddling-boundary path the splitter
// tests exercise, which is the failure mode we most care about
// (provider CLIs stream by arbitrary buffer sizes, not by line).
func writePlain(t *testing.T, chunks ...string) string {
	t.Helper()
	var buf bytes.Buffer
	w := NewTextOnlyWriter(&buf)
	for _, c := range chunks {
		if _, err := w.Write([]byte(c)); err != nil {
			t.Fatalf("write: %v", err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	return buf.String()
}

func TestTextOnlyWriter_StripsTextBlockMarkers(t *testing.T) {
	got := writePlain(t,
		"<<GLITCH_TEXT>>\n",
		"# Hello\n\nSome prose.\n",
		"<<GLITCH_END>>\n",
	)
	want := "# Hello\n\nSome prose.\n"
	if got != want {
		t.Fatalf("text-only output mismatch\nwant: %q\n got: %q", want, got)
	}
}

func TestTextOnlyWriter_CollapsesMixedBlocks(t *testing.T) {
	// A refine call that the model happened to wrap in a code block
	// (YAML body) followed by a trailing text block (rationale).
	// Both should come through as clean content with no markers.
	got := writePlain(t,
		"<<GLITCH_CODE lang=\"yaml\">>\n",
		"name: demo\nsteps: []\n",
		"<<GLITCH_END>>\n",
		"<<GLITCH_TEXT>>\n",
		"Done.\n",
		"<<GLITCH_END>>\n",
	)
	want := "name: demo\nsteps: []\nDone.\n"
	if got != want {
		t.Fatalf("mixed-block output mismatch\nwant: %q\n got: %q", want, got)
	}
}

func TestTextOnlyWriter_SurvivesMarkerSplitAcrossWrites(t *testing.T) {
	// Split a marker line across two writes to exercise the
	// splitter's pending-buffer path. If this regresses we'd see
	// marker fragments leak into the output.
	got := writePlain(t,
		"<<GLITCH_",
		"TEXT>>\nclean body\n<<GLITCH_END>>\n",
	)
	want := "clean body\n"
	if got != want {
		t.Fatalf("boundary-split output mismatch\nwant: %q\n got: %q", want, got)
	}
}

func TestTextOnlyWriter_PassesUnwrappedOutput(t *testing.T) {
	// Models sometimes ignore the protocol and emit raw markdown.
	// We must still hand that straight through — the splitter
	// treats it as implicit text, which our emit callback forwards
	// verbatim.
	got := writePlain(t, "just markdown\nno protocol here\n")
	want := "just markdown\nno protocol here\n"
	if got != want {
		t.Fatalf("unwrapped output mismatch\nwant: %q\n got: %q", want, got)
	}
}

func TestTextOnlyWriter_ComposesWithMultiWriter(t *testing.T) {
	// The main use case in draft.go is tee-ing cleaned bytes into
	// both a save buffer and a live-stream sink. Verify the writer
	// slots into io.MultiWriter without short-write surprises.
	var save bytes.Buffer
	var live bytes.Buffer
	w := NewTextOnlyWriter(io.MultiWriter(&save, &live))
	raw := "<<GLITCH_TEXT>>\nhello\n<<GLITCH_END>>\n"
	n, err := w.Write([]byte(raw))
	if err != nil {
		t.Fatalf("write: %v", err)
	}
	if n != len(raw) {
		t.Fatalf("short write: want %d, got %d", len(raw), n)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	if save.String() != "hello\n" || live.String() != "hello\n" {
		t.Fatalf("multiwriter mismatch: save=%q live=%q", save.String(), live.String())
	}
}

func TestTextOnlyWriter_FlushesTrailingBufferOnClose(t *testing.T) {
	// End the stream mid-line with no trailing newline. Close()
	// must force-flush the pending buffer as text so the last
	// characters of the refinement aren't silently dropped.
	got := writePlain(t,
		"<<GLITCH_TEXT>>\n",
		"tail without newline",
	)
	if !strings.HasSuffix(got, "tail without newline") {
		t.Fatalf("close did not flush trailing buffer: %q", got)
	}
}
