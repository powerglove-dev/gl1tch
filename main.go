package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"

	"github.com/powerglove-dev/gl1tch/cmd"
	"github.com/powerglove-dev/gl1tch/internal/bootstrap"
	"github.com/powerglove-dev/gl1tch/internal/console"
)

// Build-time variables injected by GoReleaser via -ldflags.
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "--version", "-v":
			fmt.Printf("orcai %s (commit %s, built %s)\n", version, commit, date)
			return
		case "_reload":
			bootstrap.WriteReloadMarker() //nolint:errcheck
			exec.Command("tmux", "detach-client").Run() //nolint:errcheck
			return
		case "help", "pipeline", "_opsx", "completion", "config", "cron", "widget":
			cmd.Execute()
			return
		default:
			if os.Args[1][0] == '-' {
				cmd.Execute()
				return
			}
		}
	}

	// If already inside a tmux session, run the switchboard TUI directly —
	// we were launched as the window command by bootstrap.
	if os.Getenv("TMUX") != "" {
		console.Run()
		return
	}

	err := bootstrap.Run()
	if errors.Is(err, bootstrap.ErrReload) {
		// Replace this process with the binary on disk — picks up a newly
		// built binary without going back through the session-already-exists
		// fast path in the same process image.
		self, _ := os.Executable()
		if resolved, err := filepath.EvalSymlinks(self); err == nil {
			self = resolved
		}
		if err := syscall.Exec(self, []string{self}, os.Environ()); err != nil {
			fmt.Fprintf(os.Stderr, "orcai: reload exec: %v\n", err)
			os.Exit(1)
		}
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "orcai: %v\n", err)
		os.Exit(1)
	}
}
