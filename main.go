package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"

	"github.com/8op-org/gl1tch/cmd"
	"github.com/8op-org/gl1tch/internal/bootstrap"
	"github.com/8op-org/gl1tch/internal/console"
	"github.com/8op-org/gl1tch/internal/telemetry"
)

// Build-time variables injected by GoReleaser via -ldflags.
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	ctx := context.Background()
	shutdown, err := telemetry.Setup(ctx, "gl1tch")
	if err == nil {
		defer shutdown(ctx) //nolint:errcheck
	}

	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "--version", "-v":
			fmt.Printf("glitch %s (commit %s, built %s)\n", version, commit, date)
			return
		case "_reload":
			bootstrap.WriteReloadMarker() //nolint:errcheck
			exec.Command("tmux", "detach-client").Run() //nolint:errcheck
			return
		case "ask", "help", "pipeline", "workflow", "completion", "config", "cron", "widget", "backup", "restore", "game", "plugin":
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

	err = bootstrap.Run()
	if errors.Is(err, bootstrap.ErrReload) {
		// Replace this process with the binary on disk — picks up a newly
		// built binary without going back through the session-already-exists
		// fast path in the same process image.
		self, _ := os.Executable()
		if resolved, err := filepath.EvalSymlinks(self); err == nil {
			self = resolved
		}
		if err := syscall.Exec(self, []string{self}, os.Environ()); err != nil {
			fmt.Fprintf(os.Stderr, "glitch: reload exec: %v\n", err)
			os.Exit(1)
		}
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "glitch: %v\n", err)
		os.Exit(1)
	}
}
