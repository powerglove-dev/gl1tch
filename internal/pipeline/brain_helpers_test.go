package pipeline_test

import (
	"testing"

	"github.com/adam-stokes/orcai/internal/pipeline"
)

func boolPtr(b bool) *bool { return &b }

func TestStepUseBrain(t *testing.T) {
	tests := []struct {
		name  string
		pBool bool
		sBool *bool
		want  bool
	}{
		{"pipe true + step nil = true", true, nil, true},
		{"pipe true + step false = false", true, boolPtr(false), false},
		{"pipe false + step true = true", false, boolPtr(true), true},
		{"pipe false + step nil = false", false, nil, false},
		{"pipe false + step false = false", false, boolPtr(false), false},
		{"pipe true + step true = true", true, boolPtr(true), true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &pipeline.Pipeline{UseBrain: tt.pBool}
			s := &pipeline.Step{UseBrain: tt.sBool}
			if got := pipeline.ExportedStepUseBrain(p, s); got != tt.want {
				t.Errorf("stepUseBrain(pipe=%v, step=%v) = %v, want %v", tt.pBool, tt.sBool, got, tt.want)
			}
		})
	}
}

func TestStepWriteBrain(t *testing.T) {
	tests := []struct {
		name  string
		pBool bool
		sBool *bool
		want  bool
	}{
		{"pipe true + step nil = true", true, nil, true},
		{"pipe true + step false = false", true, boolPtr(false), false},
		{"pipe false + step true = true", false, boolPtr(true), true},
		{"pipe false + step nil = false", false, nil, false},
		{"pipe false + step false = false", false, boolPtr(false), false},
		{"pipe true + step true = true", true, boolPtr(true), true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &pipeline.Pipeline{WriteBrain: tt.pBool}
			s := &pipeline.Step{WriteBrain: tt.sBool}
			if got := pipeline.ExportedStepWriteBrain(p, s); got != tt.want {
				t.Errorf("stepWriteBrain(pipe=%v, step=%v) = %v, want %v", tt.pBool, tt.sBool, got, tt.want)
			}
		})
	}
}
