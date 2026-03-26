// Binary orcai-sysop is the ABBS Switchboard widget.
//
// Run without arguments to open the full-screen Switchboard.
// Run with "toggle" to show/hide the Switchboard as a tmux popup.
package main

import (
	"os"

	"github.com/adam-stokes/orcai/internal/switchboard"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "toggle" {
		switchboard.RunToggle()
		return
	}
	switchboard.Run()
}
