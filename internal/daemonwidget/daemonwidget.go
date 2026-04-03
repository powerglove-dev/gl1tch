// Package daemonwidget launches installed plugins that declare daemon:true in
// their sidecar YAML as background processes on gl1tch session start.
//
// Each daemon binary is started with cmd.Start() (non-blocking) immediately
// after the BUSD event bus is ready. Processes are tracked and killed on
// Stop(). If a daemon exits early it is left dead — no restart logic for MVP.
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

// Manager tracks running daemon processes.
type Manager struct {
	procs []*exec.Cmd
}

// StartAll scans wrappersDir for sidecar YAMLs with daemon:true and starts
// each eligible one as a background process. Daemons whose display requirement
// cannot be satisfied in the current environment are skipped with a log line.
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

		cmd := exec.Command(sc.Command, sc.Args...)
		cmd.Stdout = nil
		cmd.Stderr = os.Stderr

		if err := cmd.Start(); err != nil {
			fmt.Fprintf(os.Stderr, "glitch: daemonwidget: start %q: %v\n", sc.Command, err)
			continue
		}

		fmt.Fprintf(os.Stderr, "glitch: daemonwidget: started %s (pid %d, display:%q)\n", sc.Name, cmd.Process.Pid, sc.Display)
		m.procs = append(m.procs, cmd)
	}

	return m
}

// Stop signals all running daemon processes to exit and reaps them.
// Errors are ignored — best-effort cleanup on session shutdown.
func (m *Manager) Stop() {
	for _, cmd := range m.procs {
		if cmd.Process != nil {
			cmd.Process.Kill() //nolint:errcheck
			cmd.Wait()         //nolint:errcheck
		}
	}
}
