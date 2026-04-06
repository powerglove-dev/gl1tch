package handlers

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"strings"
	"time"
)

// ProcessService supervises an external binary as a supervisor.Service.
// It restarts the process on crash with backoff, and respects ctx cancellation.
type ProcessService struct {
	// ServiceName is the supervisor service name.
	ServiceName string
	// Command is the path to the binary.
	Command string
	// Args are command-line arguments.
	Args []string
	// Display controls launch conditions: "systray" only launches when
	// a GUI is available; "" always launches.
	Display string
	// BuildFn is an optional function called before first launch to build
	// the binary from source. Nil means no build step.
	BuildFn func() error
}

func (p *ProcessService) Name() string { return p.ServiceName }

func (p *ProcessService) Start(ctx context.Context) error {
	if p.Display == "systray" && !hasGUIDisplay() {
		slog.Info("process: skipping (no display)", "name", p.ServiceName)
		return nil
	}

	if p.BuildFn != nil {
		if err := p.BuildFn(); err != nil {
			slog.Warn("process: build failed", "name", p.ServiceName, "err", err)
			return nil // non-fatal
		}
	}

	// Kill any orphaned instances from a previous run.
	killOrphaned(p.Command)

	// Restart loop with backoff.
	for {
		cmd := exec.CommandContext(ctx, p.Command, p.Args...)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr

		if err := cmd.Start(); err != nil {
			slog.Warn("process: start failed", "name", p.ServiceName, "err", err)
			select {
			case <-ctx.Done():
				return nil
			case <-time.After(3 * time.Second):
				continue
			}
		}

		slog.Info("process: started", "name", p.ServiceName, "pid", cmd.Process.Pid)
		_ = cmd.Wait()

		select {
		case <-ctx.Done():
			return nil
		default:
			slog.Info("process: restarting", "name", p.ServiceName)
			time.Sleep(3 * time.Second)
		}
	}
}

// hasGUIDisplay returns true if a graphical display is available.
func hasGUIDisplay() bool {
	// macOS always has a display when running as a desktop app.
	if _, err := exec.LookPath("osascript"); err == nil {
		return true
	}
	// Linux: check for X11 or Wayland.
	if os.Getenv("DISPLAY") != "" || os.Getenv("WAYLAND_DISPLAY") != "" {
		return true
	}
	return false
}

// killOrphaned kills any running process matching the binary name.
func killOrphaned(command string) {
	base := command
	if idx := strings.LastIndex(command, "/"); idx >= 0 {
		base = command[idx+1:]
	}

	out, err := exec.Command("pgrep", "-f", base).Output()
	if err != nil {
		return
	}

	myPid := os.Getpid()
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line == "" {
			continue
		}
		var pid int
		if _, err := fmt.Sscanf(line, "%d", &pid); err == nil && pid != myPid {
			proc, err := os.FindProcess(pid)
			if err == nil {
				_ = proc.Signal(os.Interrupt)
			}
		}
	}

	// Give processes a moment to exit.
	time.Sleep(200 * time.Millisecond)
}
