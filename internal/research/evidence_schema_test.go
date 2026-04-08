package research

import (
	"errors"
	"testing"
)

func TestParseEvidenceValid(t *testing.T) {
	raw := []byte(`{
		"schema_version": 1,
		"source": "github-prs",
		"title": "5 open PRs in 8op-org/gl1tch",
		"body": "PR #412: refactor router\nPR #418: brain stats",
		"refs": ["https://github.com/8op-org/gl1tch/pull/412"],
		"tags": ["github", "prs"]
	}`)
	ev, err := ParseEvidence(raw)
	if err != nil {
		t.Fatalf("ParseEvidence: %v", err)
	}
	if ev.Source != "github-prs" {
		t.Errorf("Source = %q", ev.Source)
	}
	if len(ev.Refs) != 1 {
		t.Errorf("Refs = %v", ev.Refs)
	}
	if len(ev.Tags) != 2 {
		t.Errorf("Tags = %v", ev.Tags)
	}
}

func TestParseEvidenceTolerantOfLeadingNoise(t *testing.T) {
	raw := []byte("INFO: pipeline starting\n{\"schema_version\":1,\"source\":\"x\",\"title\":\"t\",\"body\":\"b\"}\nbye\n")
	ev, err := ParseEvidence(raw)
	if err != nil {
		t.Fatalf("ParseEvidence: %v", err)
	}
	if ev.Source != "x" || ev.Body != "b" {
		t.Errorf("got %+v", ev)
	}
}

func TestParseEvidenceSchemaMismatch(t *testing.T) {
	raw := []byte(`{"schema_version": 2, "source": "x", "title": "t", "body": "b"}`)
	_, err := ParseEvidence(raw)
	if !errors.Is(err, ErrEvidenceSchemaMismatch) {
		t.Fatalf("expected ErrEvidenceSchemaMismatch, got %v", err)
	}
}

func TestParseEvidenceMissingFields(t *testing.T) {
	cases := []struct {
		name string
		raw  string
	}{
		{"missing source", `{"schema_version":1,"title":"t","body":"b"}`},
		{"missing title", `{"schema_version":1,"source":"x","body":"b"}`},
		{"missing body", `{"schema_version":1,"source":"x","title":"t"}`},
		{"empty body whitespace", `{"schema_version":1,"source":"x","title":"t","body":"   "}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ParseEvidence([]byte(tc.raw))
			if !errors.Is(err, ErrEvidenceMalformed) {
				t.Fatalf("expected ErrEvidenceMalformed, got %v", err)
			}
		})
	}
}

func TestParseEvidenceMalformedJSON(t *testing.T) {
	_, err := ParseEvidence([]byte("not json"))
	if !errors.Is(err, ErrEvidenceMalformed) {
		t.Fatalf("expected ErrEvidenceMalformed, got %v", err)
	}
}

func TestTrimToFirstJSONObjectHandlesNestedBraces(t *testing.T) {
	raw := []byte(`prefix {"a": {"b": "}{"}, "c": 1} trailing`)
	trimmed := trimToFirstJSONObject(raw)
	want := `{"a": {"b": "}{"}, "c": 1}`
	if string(trimmed) != want {
		t.Errorf("trimToFirstJSONObject = %q, want %q", string(trimmed), want)
	}
}
