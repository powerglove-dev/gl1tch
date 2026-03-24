package chatui

import (
	"math"
	"testing"
)

func TestCostEstimate_ZeroTokens(t *testing.T) {
	got := CostEstimate("claude-sonnet-4-6", 0, 0)
	if got != 0 {
		t.Errorf("expected 0 for zero tokens, got %f", got)
	}
}

func TestCostEstimate_UnknownModel(t *testing.T) {
	got := CostEstimate("gpt-4", 1000, 500)
	if got != 0 {
		t.Errorf("expected 0 for unknown model, got %f", got)
	}
}

func TestCostEstimate_Sonnet(t *testing.T) {
	// 1M input tokens at $3/MTok + 1M output tokens at $15/MTok = $18
	got := CostEstimate("claude-sonnet-4-6", 1_000_000, 1_000_000)
	want := 18.0
	if math.Abs(got-want) > 1e-9 {
		t.Errorf("sonnet: got %f, want %f", got, want)
	}
}

func TestCostEstimate_Opus(t *testing.T) {
	// 1M input at $15 + 1M output at $75 = $90
	got := CostEstimate("claude-opus-4-6", 1_000_000, 1_000_000)
	want := 90.0
	if math.Abs(got-want) > 1e-9 {
		t.Errorf("opus: got %f, want %f", got, want)
	}
}

func TestCostEstimate_Haiku(t *testing.T) {
	// 1M input at $0.25 + 1M output at $1.25 = $1.50
	got := CostEstimate("claude-haiku-4-5", 1_000_000, 1_000_000)
	want := 1.50
	if math.Abs(got-want) > 1e-9 {
		t.Errorf("haiku: got %f, want %f", got, want)
	}
}

func TestCostEstimate_PartialTokens(t *testing.T) {
	// sonnet: 500k input at $3/MTok = $1.50, 200k output at $15/MTok = $3.00 => $4.50
	got := CostEstimate("claude-sonnet-4", 500_000, 200_000)
	want := 1.50 + 3.00
	if math.Abs(got-want) > 1e-9 {
		t.Errorf("partial tokens: got %f, want %f", got, want)
	}
}
