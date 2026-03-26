// Package welcome is a thin shim that delegates to the switchboard package.
//
// The standalone welcome dashboard has been replaced by the Switchboard TUI.
// This package is retained for backwards compatibility with callers that import
// internal/welcome directly; all paths now call switchboard.Run().
package welcome

import "github.com/adam-stokes/orcai/internal/switchboard"

// Run starts the ABBS Switchboard. The busSocket parameter is accepted but
// ignored — bus connectivity is handled inside the switchboard.
func Run(_ string) error {
	switchboard.Run()
	return nil
}
