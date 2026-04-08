package research

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// EvidenceSchemaVersion is the integer version of the Evidence wire schema.
// Bump it when the field set or semantics change in a way that older
// producers (pipeline researchers, plugin researchers) cannot satisfy. The
// version is part of the schema document so that producers can declare which
// version they target and the loader can refuse evidence from incompatible
// producers.
const EvidenceSchemaVersion = 1

// EvidenceSchemaDoc is the human-readable description of the Evidence
// schema. PipelineResearcher includes it in the prompt of any pipeline that
// asks the LLM to write an Evidence JSON, so the model knows exactly what
// shape to produce. Keeping the prose here (not in a markdown file) means
// the schema and the prose can never drift apart at build time.
const EvidenceSchemaDoc = `Evidence JSON schema (version 1)

A researcher's final step must print exactly one JSON object with these
fields. Unknown fields are tolerated for forward compatibility but ignored.

{
  "schema_version": 1,        (required) integer, must equal 1
  "source":         string,   (required) the researcher's name
  "title":          string,   (required) one-line label for the draft to cite
  "body":           string,   (required) the actual content; may be markdown
  "refs":           [string], (optional) URLs, file paths, commit SHAs
  "tags":           [string], (optional) free-form labels for cross-source agreement
  "meta":           {string:string} (optional) small key/value extras
}

Rules:
- The object must be the only thing on stdout, no surrounding prose.
- "body" must contain the actual evidence text. Do not put placeholders.
- "refs" should be specific identifiers, not descriptions.
- Empty arrays are allowed; nil is not.`

// EvidenceWire is the on-the-wire shape that PipelineResearcher and plugin
// researchers produce. It is intentionally a superset of Evidence so that the
// loader can validate the schema_version field before constructing the
// runtime Evidence value.
type EvidenceWire struct {
	SchemaVersion int               `json:"schema_version"`
	Source        string            `json:"source"`
	Title         string            `json:"title"`
	Body          string            `json:"body"`
	Refs          []string          `json:"refs,omitempty"`
	Tags          []string          `json:"tags,omitempty"`
	Meta          map[string]string `json:"meta,omitempty"`
}

// ErrEvidenceSchemaMismatch is returned when an EvidenceWire declares a
// schema_version the host does not understand. The loop's gather stage logs
// the error and drops the evidence; it does not abort the call.
var ErrEvidenceSchemaMismatch = errors.New("research: evidence schema version mismatch")

// ErrEvidenceMalformed is returned when an EvidenceWire is structurally
// invalid (missing required fields, wrong types). Like ErrEvidenceSchemaMismatch
// it is a per-evidence drop, not a fatal error.
var ErrEvidenceMalformed = errors.New("research: evidence malformed")

// ParseEvidence reads a JSON document and returns the runtime Evidence value
// it represents, or an error explaining why the document is unusable. The
// parser is strict on required fields and tolerant on extras.
func ParseEvidence(raw []byte) (Evidence, error) {
	raw = trimToFirstJSONObject(raw)
	if len(raw) == 0 {
		return Evidence{}, fmt.Errorf("%w: empty input", ErrEvidenceMalformed)
	}
	var w EvidenceWire
	if err := json.Unmarshal(raw, &w); err != nil {
		return Evidence{}, fmt.Errorf("%w: %v", ErrEvidenceMalformed, err)
	}
	return ValidateEvidenceWire(w)
}

// ValidateEvidenceWire enforces the schema rules on a parsed wire value and
// returns the runtime Evidence on success.
func ValidateEvidenceWire(w EvidenceWire) (Evidence, error) {
	if w.SchemaVersion != EvidenceSchemaVersion {
		return Evidence{}, fmt.Errorf("%w: got %d, want %d", ErrEvidenceSchemaMismatch, w.SchemaVersion, EvidenceSchemaVersion)
	}
	if strings.TrimSpace(w.Source) == "" {
		return Evidence{}, fmt.Errorf("%w: source is required", ErrEvidenceMalformed)
	}
	if strings.TrimSpace(w.Title) == "" {
		return Evidence{}, fmt.Errorf("%w: title is required", ErrEvidenceMalformed)
	}
	if strings.TrimSpace(w.Body) == "" {
		return Evidence{}, fmt.Errorf("%w: body is required", ErrEvidenceMalformed)
	}
	return Evidence{
		Source: w.Source,
		Title:  w.Title,
		Body:   w.Body,
		Refs:   append([]string(nil), w.Refs...),
		Tags:   append([]string(nil), w.Tags...),
		Meta:   copyMeta(w.Meta),
	}, nil
}

// trimToFirstJSONObject finds the first '{' in raw and returns the slice
// from that point to the matching '}'. Pipeline final-step output often
// contains a trailing newline or a leading log line; this lets the loader
// be tolerant of that without giving up structural validation.
func trimToFirstJSONObject(raw []byte) []byte {
	start := -1
	for i, b := range raw {
		if b == '{' {
			start = i
			break
		}
	}
	if start < 0 {
		return nil
	}
	depth := 0
	inString := false
	escaped := false
	for i := start; i < len(raw); i++ {
		b := raw[i]
		if inString {
			if escaped {
				escaped = false
				continue
			}
			if b == '\\' {
				escaped = true
				continue
			}
			if b == '"' {
				inString = false
			}
			continue
		}
		switch b {
		case '"':
			inString = true
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return raw[start : i+1]
			}
		}
	}
	return nil
}

func copyMeta(in map[string]string) map[string]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}
