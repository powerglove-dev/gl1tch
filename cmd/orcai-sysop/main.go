// Binary orcai-sysop is the ABS sysop panel widget.
//
// Run without arguments to open the interactive sysop monitor.
// Run with "toggle" to show/hide the panel as a tmux split-pane.
package main

import (
	"os"

	"github.com/adam-stokes/orcai/internal/sidebar"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "toggle" {
		sidebar.RunToggle()
		return
	}
	sidebar.Run()
}
