package glitchproto

import (
	"bytes"
	"strings"
	"testing"
)

// writeContent feeds chunks through a ContentOnlyWriter and returns
// what survived. Multi-chunk calls exercise the same line-buffer
// straddling we care about in production (provider CLIs stream by
// arbitrary buffer sizes, not lines).
func writeContent(t *testing.T, chunks ...string) string {
	t.Helper()
	var buf bytes.Buffer
	w := NewContentOnlyWriter(&buf)
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

func TestContentOnlyWriter_StripsStatsSentinel(t *testing.T) {
	in := "hello world\n" +
		`{"duration_ms":7573,"input_tokens":2106,"model":"llama3.2","output_tokens":272,"type":"gl1tch-stats"}` + "\n"
	got := writeContent(t, in)
	want := "hello world\n"
	if got != want {
		t.Fatalf("stats not stripped\nwant: %q\n got: %q", want, got)
	}
}

func TestContentOnlyWriter_StripsMultilineBrainBlock(t *testing.T) {
	// Exact shape from the screenshot that prompted this feature —
	// a brain block closing on its own line, followed by a stats
	// JSON, at the tail of an otherwise clean YAML body.
	in := strings.Join([]string{
		"- id: step-1",
		"  prompt: |",
		"    do the thing",
		`<brain type="finding" tags="security,hardcoded credentials" title="Hardcoded Credentials Found">`,
		"    The codebase contains hardcoded API keys.",
		"    File: internal/config.json line 23.",
		"</brain>",
		`{"duration_ms":7573,"input_tokens":2106,"model":"llama3.2","output_tokens":272,"type":"gl1tch-stats"}`,
		"",
	}, "\n")
	got := writeContent(t, in)
	want := "- id: step-1\n  prompt: |\n    do the thing\n"
	if got != want {
		t.Fatalf("brain+stats not stripped\nwant: %q\n got: %q", want, got)
	}
}

func TestContentOnlyWriter_StripsSameLineBrain(t *testing.T) {
	// A brain block that opens and closes on the same line, with
	// real content on both sides. The pre-tag and post-tag halves
	// should be concatenated and kept.
	in := "before text <brain type=\"note\">inside</brain> after text\n"
	got := writeContent(t, in)
	want := "before text  after text\n"
	if got != want {
		t.Fatalf("same-line brain not stripped\nwant: %q\n got: %q", want, got)
	}
}

func TestContentOnlyWriter_StripsWroteAckAndArrow(t *testing.T) {
	// Plugins sometimes emit `<> [wrote: path]` together, then the
	// stats sentinel on a following line.
	in := "main body\n" +
		"<> [wrote: internal/foo.go]\n" +
		`{"type":"gl1tch-stats","duration_ms":100}` + "\n"
	got := writeContent(t, in)
	want := "main body\n"
	if got != want {
		t.Fatalf("wrote/arrow not stripped\nwant: %q\n got: %q", want, got)
	}
}

func TestContentOnlyWriter_PreservesLegitimateBlankLines(t *testing.T) {
	// A real markdown body with paragraph breaks. The mutation
	// gate in scrubLine must not drop these.
	in := "# Heading\n\nFirst paragraph.\n\nSecond paragraph.\n"
	got := writeContent(t, in)
	if got != in {
		t.Fatalf("blank lines dropped\nwant: %q\n got: %q", in, got)
	}
}

func TestContentOnlyWriter_ComposesWithGLITCHMarkers(t *testing.T) {
	// A realistic mixed stream: the model wraps its reply in a
	// GLITCH_TEXT block, emits a brain note mid-reply, then a
	// stats sentinel after closing. All layers must unwind
	// cleanly in one pass.
	in := strings.Join([]string{
		"<<GLITCH_TEXT>>",
		"# Refined prompt",
		"",
		"You are a careful reviewer.",
		`<brain type="finding" title="noted">`,
		"some finding",
		"</brain>",
		"Keep it tight.",
		"<<GLITCH_END>>",
		`{"type":"gl1tch-stats","input_tokens":42,"output_tokens":10}`,
		"",
	}, "\n")
	got := writeContent(t, in)
	want := "# Refined prompt\n\nYou are a careful reviewer.\nKeep it tight.\n"
	if got != want {
		t.Fatalf("layered protocol not fully stripped\nwant: %q\n got: %q", want, got)
	}
}

func TestContentOnlyWriter_SurvivesBrainSplitAcrossWrites(t *testing.T) {
	// Brain open tag split across two Write() calls. The line
	// buffer should hold the fragment until the full line arrives.
	got := writeContent(t,
		"prefix line\n<brain type=\"fi",
		"nding\">\nbody\n</brain>\ntail\n",
	)
	want := "prefix line\ntail\n"
	if got != want {
		t.Fatalf("split brain leaked\nwant: %q\n got: %q", want, got)
	}
}

func TestContentOnlyWriter_SurvivesStatsSplitAcrossWrites(t *testing.T) {
	// Stats sentinel split mid-JSON. Nothing should leak out.
	got := writeContent(t,
		"real content\n",
		`{"duration_ms":123,"type":`,
		`"gl1tch-stats"}`+"\n",
	)
	want := "real content\n"
	if got != want {
		t.Fatalf("split stats leaked\nwant: %q\n got: %q", want, got)
	}
}

func TestContentOnlyWriter_FlushesTrailingPartialLine(t *testing.T) {
	// A final chunk with no trailing newline must still make it
	// out on Close. This is the shape a provider that cuts its
	// own stream short produces.
	got := writeContent(t, "first line\n", "tail no newline")
	if !strings.HasSuffix(got, "tail no newline") {
		t.Fatalf("trailing partial not flushed: %q", got)
	}
}

func TestContentOnlyWriter_DropsStandaloneArrowPrefix(t *testing.T) {
	// The `<>` arrow sometimes appears on its own line as a
	// separator marker. The arrow-prefix regex + mutation gate
	// should drop it entirely.
	got := writeContent(t, "content\n<>\nmore content\n")
	want := "content\nmore content\n"
	if got != want {
		t.Fatalf("standalone arrow leaked\nwant: %q\n got: %q", want, got)
	}
}

func TestContentOnlyWriter_KeepsYAMLDocumentSeparators(t *testing.T) {
	// YAML document separators (---) are three real characters,
	// not whitespace — they should come through untouched.
	in := "---\nname: demo\n---\nname: other\n"
	got := writeContent(t, in)
	if got != in {
		t.Fatalf("yaml separator dropped\nwant: %q\n got: %q", in, got)
	}
}
