// Package daemonwidget launches installed plugins that declare daemon:true in
// their sidecar YAML as background processes on gl1tch session start.
//
// Each daemon binary is started with cmd.Start() (non-blocking) immediately
// after the BUSD event bus is ready. Processes are tracked per entry and
// automatically restarted on unexpected exit with a 3-second backoff.
//
// Display requirements
//
// A sidecar may declare a display field to describe its graphical needs:
//
//	display: ""          — headless, always launched (default)
//	display: headless    — same as empty
//	display: systray     — requires a GUI windowing environment; skipped when
//	                       no display is detected (SSH, headless Linux, etc.)
//
// On macOS (darwin) a windowing environment is always assumed present.
// On other platforms the launcher checks $DISPLAY (X11) and $WAYLAND_DISPLAY.
package daemonwidget

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"gopkg.in/yaml.v3"
)

// sidecar is the minimal subset of executor.SidecarSchema we need here.
// Duplicating it avoids an import cycle with internal/executor.
type sidecar struct {
	Name    string   `yaml:"name"`
	Command string   `yaml:"command"`
	Args    []string `yaml:"args,omitempty"`
	Daemon  bool     `yaml:"daemon,omitempty"`
	// Display describes graphical requirements: "", "headless", or "systray".
	Display string `yaml:"display,omitempty"`
}

// hasDisplay returns true when a windowing environment is available.
// On macOS it is always true. On Linux/other it requires $DISPLAY or
// $WAYLAND_DISPLAY to be set.
func hasDisplay() bool {
	if runtime.GOOS == "darwin" {
		return true
	}
	return os.Getenv("DISPLAY") != "" || os.Getenv("WAYLAND_DISPLAY") != ""
}

// canLaunch reports whether a daemon with the given display requirement can be
// started in the current environment.
func canLaunch(display string) bool {
	switch display {
	case "systray":
		return hasDisplay()
	default: // "", "headless", or any unrecognised value — always launch
		return true
	}
}

// killAndWait terminates any running process whose command line matches
// cmdPath and blocks until the process is gone (or 2 s elapses). It uses
// pgrep to find the PID, SIGTERM to request a clean exit, then polls
// kill -0 (existence check) so callers never see a double icon.
func killAndWait(cmdPath string) {
	out, err := exec.Command("pgrep", "-f", cmdPath).Output()
	if err != nil {
		return // no match — nothing to kill
	}
	for _, line := range strings.Fields(strings.TrimSpace(string(out))) {
		pid, err := strconv.Atoi(line)
		if err != nil {
			continue
		}
		proc, err := os.FindProcess(pid)
		if err != nil {
			continue
		}
		proc.Signal(syscall.SIGTERM) //nolint:errcheck

		deadline := time.Now().Add(2 * time.Second)
		for time.Now().Before(deadline) {
			if proc.Signal(syscall.Signal(0)) != nil {
				break // process is gone
			}
			time.Sleep(50 * time.Millisecond)
		}
	}
}

// daemonEntry tracks a single running daemon with its restart state.
type daemonEntry struct {
	sc      sidecar
	cmd     *exec.Cmd
	mu      sync.Mutex
	stopped bool // set to true when Stop() is called so restart loop exits
}

// Manager supervises daemon processes, restarting them on unexpected exit.
type Manager struct {
	entries []*daemonEntry
	wg      sync.WaitGroup
}

// startEntry starts the daemon process for the given entry and launches a
// supervision goroutine that restarts it on unexpected exit.
func (m *Manager) startEntry(entry *daemonEntry) {
	m.wg.Add(1)
	go func() {
		defer m.wg.Done()
		for {
			entry.mu.Lock()
			if entry.stopped {
				entry.mu.Unlock()
				return
			}
			cmd := exec.Command(entry.sc.Command, entry.sc.Args...)
			cmd.Stdout = nil
			cmd.Stderr = os.Stderr
			entry.cmd = cmd
			entry.mu.Unlock()

			if err := cmd.Start(); err != nil {
				fmt.Fprintf(os.Stderr, "glitch: daemonwidget: start %q: %v\n", entry.sc.Command, err)
				// Back off before retrying so we don't spin hard on a bad binary.
				time.Sleep(3 * time.Second)
				entry.mu.Lock()
				if entry.stopped {
					entry.mu.Unlock()
					return
				}
				entry.mu.Unlock()
				continue
			}
			fmt.Fprintf(os.Stderr, "glitch: daemonwidget: started %s (pid %d, display:%q)\n",
				entry.sc.Name, cmd.Process.Pid, entry.sc.Display)

			// Wait for the process to exit.
			_ = cmd.Wait()

			entry.mu.Lock()
			if entry.stopped {
				entry.mu.Unlock()
				return
			}
			entry.mu.Unlock()

			fmt.Fprintf(os.Stderr, "glitch: daemonwidget: %s exited unexpectedly, restarting in 3s\n", entry.sc.Name)
			time.Sleep(3 * time.Second)
		}
	}()
}

// StartAll scans wrappersDir for sidecar YAMLs with daemon:true and starts
// each eligible one as a supervised background process. Daemons whose display
// requirement cannot be satisfied in the current environment are skipped.
// Errors for individual daemons are printed to stderr and skipped — a single
// bad entry will not prevent the others from launching.
func StartAll(wrappersDir string) *Manager {
	m := &Manager{}

	entries, err := os.ReadDir(wrappersDir)
	if err != nil {
		if !os.IsNotExist(err) {
			fmt.Fprintf(os.Stderr, "glitch: daemonwidget: read wrappers dir: %v\n", err)
		}
		return m
	}

	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".yaml" {
			continue
		}

		path := filepath.Join(wrappersDir, e.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			fmt.Fprintf(os.Stderr, "glitch: daemonwidget: read %s: %v\n", e.Name(), err)
			continue
		}

		var sc sidecar
		if err := yaml.Unmarshal(data, &sc); err != nil {
			fmt.Fprintf(os.Stderr, "glitch: daemonwidget: parse %s: %v\n", e.Name(), err)
			continue
		}

		if !sc.Daemon {
			continue
		}

		if !canLaunch(sc.Display) {
			fmt.Fprintf(os.Stderr, "glitch: daemonwidget: skipping %s (display:%q not available)\n", sc.Name, sc.Display)
			continue
		}

		if sc.Command == "" {
			fmt.Fprintf(os.Stderr, "glitch: daemonwidget: %s has daemon:true but no command\n", e.Name())
			continue
		}

		// Kill any orphaned instance before starting a fresh one and wait for
		// it to fully exit. This prevents duplicate systray icons when the
		// previous session exited without cleanup (e.g. SIGKILL).
		killAndWait(sc.Command)

		de := &daemonEntry{sc: sc}
		m.entries = append(m.entries, de)
		m.startEntry(de)
	}

	return m
}

// Stop signals all running daemon processes to exit and waits for their
// supervision goroutines to finish. Errors are ignored — best-effort cleanup.
func (m *Manager) Stop() {
	for _, de := range m.entries {
		de.mu.Lock()
		de.stopped = true
		if de.cmd != nil && de.cmd.Process != nil {
			de.cmd.Process.Kill() //nolint:errcheck
		}
		de.mu.Unlock()
	}
	m.wg.Wait()
}
