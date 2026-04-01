// Package welcome is a thin shim that delegates to the switchboard package.
//
// The standalone welcome dashboard has been replaced by the Switchboard TUI.
// This package is retained for backwards compatibility with callers that import
// internal/welcome directly; all paths now call console.Run().
package welcome

import "github.com/8op-org/gl1tch/internal/console"

// Run starts the GLITCH Switchboard. The busSocket parameter is accepted but
// ignored — bus connectivity is handled inside the console.
func Run(_ string) error {
	console.Run()
	return nil
}
