package chatui

// modelRates maps model name substrings to [inputPerMTok, outputPerMTok] USD rates.
// Keys are matched via strings.Contains so prefixes like "claude-opus-4" cover versioned names.
var modelRates = map[string][2]float64{
	"claude-opus-4":   {15.00, 75.00},
	"claude-sonnet-4": {3.00, 15.00},
	"claude-haiku-4":  {0.25, 1.25},
}

// CostEstimate returns the estimated USD cost for a completed response.
// Rates are in USD per million tokens ($/MTok).
// Returns 0 when the model is unknown or token counts are zero.
func CostEstimate(model string, inputTokens, outputTokens int) float64 {
	if inputTokens == 0 && outputTokens == 0 {
		return 0
	}
	for key, rates := range modelRates {
		if containsCI(model, key) {
			inCost := float64(inputTokens) / 1_000_000 * rates[0]
			outCost := float64(outputTokens) / 1_000_000 * rates[1]
			return inCost + outCost
		}
	}
	return 0
}

// containsCI reports whether s contains substr (case-insensitive).
func containsCI(s, substr string) bool {
	sl, subl := len(s), len(substr)
	if subl == 0 {
		return true
	}
	if subl > sl {
		return false
	}
	for i := 0; i <= sl-subl; i++ {
		match := true
		for j := 0; j < subl; j++ {
			cs, csub := s[i+j], substr[j]
			if cs >= 'A' && cs <= 'Z' {
				cs += 32
			}
			if csub >= 'A' && csub <= 'Z' {
				csub += 32
			}
			if cs != csub {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}
