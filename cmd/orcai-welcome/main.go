// Binary orcai-welcome is the ABBS welcome entry point.
//
// It opens the full-screen Switchboard TUI. When the user exits the switchboard
// it replaces itself with $SHELL via syscall.Exec so the tmux window gets a
// proper interactive shell (identical to the legacy orcai-welcome behaviour).
package main

import (
	"fmt"
	"os"
	"syscall"

	"github.com/adam-stokes/orcai/internal/switchboard"
)

func main() {
	switchboard.Run()
	execShell()
}

func execShell() {
	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "/bin/sh"
	}
	if err := syscall.Exec(shell, []string{shell}, os.Environ()); err != nil {
		fmt.Fprintf(os.Stderr, "orcai-welcome: exec shell: %v\n", err)
		os.Exit(1)
	}
}
