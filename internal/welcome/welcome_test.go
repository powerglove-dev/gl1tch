// Package welcome tests verify that the welcome shim compiles and that Run
// is callable. The welcome dashboard was replaced by the Deck; this
// package is now a thin delegation layer.
package welcome

import (
	"testing"
)

// TestRunSignature verifies that Run accepts a string and returns an error,
// matching the expected function signature for callers.
func TestRunSignature(t *testing.T) {
	// Compile-time check: if Run's signature changes the build will fail.
	var fn func(string) error = Run
	_ = fn
}
