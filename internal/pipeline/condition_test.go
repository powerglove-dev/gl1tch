package pipeline_test

import (
	"testing"

	"github.com/8op-org/gl1tch/internal/pipeline"
)

// condVars builds a vars map with _output set for condition evaluation.
func condVars(output string) map[string]any {
	return map[string]any{"_output": output}
}

func TestEvalCondition_Contains(t *testing.T) {
	if !pipeline.EvalCondition("contains:spec", condVars("openspec output here")) {
		t.Error("expected true for contains:spec")
	}
	if pipeline.EvalCondition("contains:spec", condVars("nothing here")) {
		t.Error("expected false for contains:spec")
	}
}

func TestEvalCondition_Always(t *testing.T) {
	if !pipeline.EvalCondition("always", condVars("anything")) {
		t.Error("expected always to be true")
	}
}

func TestEvalCondition_LenGt(t *testing.T) {
	if !pipeline.EvalCondition("len > 5", condVars("hello world")) {
		t.Error("expected true for len > 5 on 11-char string")
	}
	if pipeline.EvalCondition("len > 5", condVars("hi")) {
		t.Error("expected false for len > 5 on 2-char string")
	}
}

func TestEvalCondition_Matches(t *testing.T) {
	if !pipeline.EvalCondition("matches:^go", condVars("golang is great")) {
		t.Error("expected true for matches:^go")
	}
	if pipeline.EvalCondition("matches:^go", condVars("python is great")) {
		t.Error("expected false for matches:^go")
	}
}

func TestEvalCondition_Unknown(t *testing.T) {
	// Unknown expressions default to false.
	if pipeline.EvalCondition("unknown-expr", condVars("anything")) {
		t.Error("expected false for unknown expression")
	}
}

func TestEvalCondition_NotEmpty(t *testing.T) {
	cases := []struct {
		name   string
		value  string
		want   bool
	}{
		{"non-empty value passes", "some text", true},
		{"empty string fails", "", false},
		{"whitespace-only fails", "   ", false},
		{"newline-only fails", "\n", false},
		{"content with spaces passes", "  hello  ", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := pipeline.EvalCondition("not_empty", condVars(tc.value))
			if got != tc.want {
				t.Errorf("EvalCondition(not_empty, %q) = %v, want %v", tc.value, got, tc.want)
			}
		})
	}
}

func TestEvalCondition_EmptyVars(t *testing.T) {
	// No _output key — all exprs should evaluate against empty string.
	if pipeline.EvalCondition("contains:x", map[string]any{}) {
		t.Error("expected false with no _output")
	}
	if !pipeline.EvalCondition("always", map[string]any{}) {
		t.Error("always should be true even with no _output")
	}
}
