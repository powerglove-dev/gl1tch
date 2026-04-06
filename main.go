package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"

	"github.com/8op-org/gl1tch/cmd"
	"github.com/8op-org/gl1tch/internal/bootstrap"
	"github.com/8op-org/gl1tch/internal/telemetry"
)

// Build-time variables injected by GoReleaser via -ldflags.
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	// Load .env early so all code paths (TUI + CLI subcommands) see the vars.
	if home, err := os.UserHomeDir(); err == nil {
		bootstrap.LoadDotenv(filepath.Join(home, ".config", "glitch", ".env"))
	}
	bootstrap.LoadDotenv(".env")

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
			return
		case "ask", "busd", "help", "model", "observe", "pipeline", "workflow", "completion", "config", "cron", "widget", "backup", "restore", "game", "plugin", "serve", "tui", "desktop":
			cmd.Execute()
			return
		default:
			if os.Args[1][0] == '-' {
				cmd.Execute()
				return
			}
		}
	}

	// Default: run the TUI with all services.
	// For the desktop GUI, run `glitch-desktop` (built via `task desktop`).
	err = bootstrap.Run()
	if errors.Is(err, bootstrap.ErrReload) {
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

