package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"

	"github.com/adam-stokes/orcai/cmd"
	"github.com/adam-stokes/orcai/internal/bootstrap"
	"github.com/adam-stokes/orcai/internal/jumpwindow"
	"github.com/adam-stokes/orcai/internal/promptbuilder"
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
		case "_welcome":
			// Launch the orcai-welcome widget binary. Look it up in PATH first;
			// fall back to the same directory as the orcai binary.
			bin := "orcai-welcome"
			if _, err := exec.LookPath(bin); err != nil {
				self, _ := os.Executable()
				bin = filepath.Join(filepath.Dir(self), "orcai-welcome")
			}
			wCmd := exec.Command(bin)
			wCmd.Stdin = os.Stdin
			wCmd.Stdout = os.Stdout
			wCmd.Stderr = os.Stderr
			if err := wCmd.Run(); err != nil {
				fmt.Fprintf(os.Stderr, "orcai-welcome: %v\n", err)
				os.Exit(1)
			}
			return
		case "_promptbuilder", "pipeline-builder":
			promptbuilder.Run()
			return
		case "_reload":
			bootstrap.WriteReloadMarker() //nolint:errcheck
			exec.Command("tmux", "detach-client").Run() //nolint:errcheck
			return
		case "_jump":
			jumpwindow.Run()
			return
		case "agent", "bridge", "git", "weather", "code", "new", "kill", "help", "pipeline", "ollama", "_opsx", "completion",
			"sysop", "picker", "welcome", "config", "cron", "prompts":
			cmd.Execute()
			return
		default:
			if os.Args[1][0] == '-' {
				cmd.Execute()
				return
			}
		}
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
