package console

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// currentTmuxSession returns the name of the current tmux session by reading the
// TMUX environment variable or falling back to `tmux display-message -p '#S'`.
func currentTmuxSession() string {
	tmuxEnv := strings.TrimSpace(os.Getenv("TMUX"))
	if tmuxEnv != "" {
		// TMUX=/tmp/tmux-1000/default,12345,0 — session name is the first field,
		// but it's actually the socket path. We need `tmux display-message -p '#S'`.
		// Fall through to exec approach for accuracy.
	}
	out, err := exec.Command("tmux", "display-message", "-p", "#S").Output()
	if err == nil {
		return strings.TrimSpace(string(out))
	}
	return ""
}

// createJobWindow creates a detached tmux window named "glitch-<feedID>".
//
// label is the human-readable pipeline/agent name stored as the tmux user
// option @glitch-label so the jump-window popup can display it instead of the
// raw window name. Pass an empty string to skip setting the option.
//
// startDir, when non-empty, sets the working directory for the new tmux window
// via the -c flag. Pass an empty string to inherit the session's default.
//
// If shellCmd is non-empty the window runs the command, tees output to logFile,
// and writes the exit code to doneFile. remain-on-exit keeps the window alive
// after the command finishes so the user can inspect the scrollback. Use
// startLogWatcher to receive FeedLineMsg / jobDoneMsg / jobFailedMsg events.
//
// If shellCmd is empty the window runs tail -f on logFile (legacy path for
// in-process agent jobs that write to logFile themselves).
//
// Returns (target, logFile, doneFile). All empty strings if tmux is unavailable.
func createJobWindow(feedID, shellCmd, label, startDir string) (target, logFile, doneFile string) {
	// Never create real tmux windows during go test runs.
	if strings.HasSuffix(os.Args[0], ".test") {
		return "", "", ""
	}
	if _, err := exec.LookPath("tmux"); err != nil {
		return "", "", ""
	}
	session := currentTmuxSession()
	if session == "" {
		return "", "", ""
	}
	windowName := "glitch-" + feedID
	logFile = fmt.Sprintf("%s/glitch-%s.log", os.TempDir(), feedID)
	doneFile = fmt.Sprintf("%s/glitch-%s.done", os.TempDir(), feedID)
	target = session + ":" + windowName

	// Pre-create empty log so the watcher can open it immediately.
	os.WriteFile(logFile, nil, 0o600) //nolint:errcheck

	var windowCmd string
	if shellCmd != "" {
		// Tee output to log file; write exit code to done file on completion;
		// then exec $SHELL so the pane transitions to a live interactive shell
		// rather than showing "[pane is dead]". remain-on-exit is set as a
		// safety net in case $SHELL itself exits.
		windowCmd = fmt.Sprintf("{ %s ; } 2>&1 | tee %s ; echo $? > %s ; exec $SHELL", shellCmd, logFile, doneFile)
	} else {
		// Legacy: in-process agent job — tail the log file live.
		windowCmd = "tail -f " + logFile + " 2>/dev/null"
	}

	// Use -t "session:" (trailing colon) to always append at the end of the
	// window list, avoiding index conflicts when multiple windows are created
	// in rapid succession.
	args := []string{"new-window", "-d", "-t", session + ":", "-n", windowName, "-P", "-F", "#{window_id}"}
	if startDir != "" {
		args = append(args, "-c", startDir)
	}
	args = append(args, windowCmd)
	out, err := exec.Command("tmux", args...).Output()
	if err == nil {
		if id := strings.TrimSpace(string(out)); id != "" {
			target = session + ":" + id // stable @N ID, survives auto-rename
		}
	}

	// Keep window alive after command exits and disable auto-rename.
	exec.Command("tmux", "set-window-option", "-t", target, "remain-on-exit", "on").Run()    //nolint:errcheck
	exec.Command("tmux", "set-window-option", "-t", target, "automatic-rename", "off").Run() //nolint:errcheck
	if label != "" {
		exec.Command("tmux", "set-window-option", "-t", target, "@glitch-label", label).Run() //nolint:errcheck
	}

	return target, logFile, doneFile
}

// startLogWatcher launches a background goroutine that tails logFile for new
// content, sending FeedLineMsg values to ch. When doneFile appears it reads the
// exit code and sends jobDoneMsg (exit 0) or jobFailedMsg (non-zero), then exits.
func startLogWatcher(feedID, logFile, doneFile string, ch chan<- tea.Msg) {
	// No log file means no tmux window was created (e.g. test mode). Signal
	// done immediately so drainChan callers are not left blocking.
	if logFile == "" {
		close(ch)
		return
	}
	go func() {
		var offset int64
		for {
			time.Sleep(150 * time.Millisecond)

			// Read any new bytes from the log file.
			if f, err := os.Open(logFile); err == nil {
				f.Seek(offset, io.SeekStart) //nolint:errcheck
				data, _ := io.ReadAll(f)
				f.Close()
				if len(data) > 0 {
					offset += int64(len(data))
					for _, line := range strings.Split(strings.TrimRight(string(data), "\n"), "\n") {
						if line != "" {
							if stepID, status, ok := parseStepStatus(line); ok {
								ch <- StepStatusMsg{FeedID: feedID, StepID: stepID, Status: status}
							} else {
								ch <- FeedLineMsg{ID: feedID, Line: line}
							}
						}
					}
				}
			}

			// Check for the done file.
			raw, err := os.ReadFile(doneFile)
			if err != nil {
				continue // not done yet
			}
			code := strings.TrimSpace(string(raw))
			if code == "" {
				continue // file exists but not written yet
			}
			os.Remove(doneFile) //nolint:errcheck
			if code == "0" {
				ch <- jobDoneMsg{id: feedID}
			} else {
				ch <- jobFailedMsg{id: feedID, err: fmt.Errorf("pipeline exited with code %s", code)}
			}
			return
		}
	}()
}

// currentTmuxPane returns the pane ID of the running process as set by tmux
// in the TMUX_PANE environment variable (e.g. "%42"). Empty when not in tmux.
func currentTmuxPane() string {
	return strings.TrimSpace(os.Getenv("TMUX_PANE"))
}

// createJobPane creates an inline tmux pane for a pipeline job instead of a
// separate window. The first pipeline is split horizontally from the glitch
// pane at 50 % (side-by-side). Subsequent pipelines are split vertically
// below lastPaneID so they stack evenly on the right side of the screen.
//
// Returns (paneID, logFile, doneFile). All empty strings if tmux is
// unavailable or no TMUX_PANE is set for the first split.
func createJobPane(feedID, shellCmd, label, startDir, lastPaneID string) (paneID, logFile, doneFile string) {
	if strings.HasSuffix(os.Args[0], ".test") {
		return "", "", ""
	}
	if _, err := exec.LookPath("tmux"); err != nil {
		return "", "", ""
	}
	glitchPane := currentTmuxPane()
	if glitchPane == "" && lastPaneID == "" {
		return "", "", ""
	}

	logFile = fmt.Sprintf("%s/glitch-%s.log", os.TempDir(), feedID)
	doneFile = fmt.Sprintf("%s/glitch-%s.done", os.TempDir(), feedID)
	os.WriteFile(logFile, nil, 0o600) //nolint:errcheck

	windowCmd := fmt.Sprintf("{ %s ; } 2>&1 | tee %s ; echo $? > %s ; exec $SHELL", shellCmd, logFile, doneFile)

	var args []string
	if lastPaneID == "" {
		// First pipeline: split the glitch pane in half horizontally (left=glitch, right=pipeline).
		args = []string{"split-window", "-h", "-l", "50%", "-t", glitchPane, "-P", "-F", "#{pane_id}"}
	} else {
		// Subsequent pipelines: split vertically below the last pipeline pane.
		args = []string{"split-window", "-v", "-t", lastPaneID, "-P", "-F", "#{pane_id}"}
	}
	if startDir != "" {
		args = append(args, "-c", startDir)
	}
	args = append(args, windowCmd)

	out, err := exec.Command("tmux", args...).Output()
	if err != nil {
		return "", logFile, doneFile
	}
	paneID = strings.TrimSpace(string(out))
	if paneID == "" {
		return "", logFile, doneFile
	}

	// Keep pane alive after command exits so the user can inspect output.
	exec.Command("tmux", "set-option", "-pt", paneID, "remain-on-exit", "on").Run() //nolint:errcheck
	if label != "" {
		exec.Command("tmux", "select-pane", "-t", paneID, "-T", label).Run() //nolint:errcheck
	}
	return paneID, logFile, doneFile
}

var ansiEscRe = regexp.MustCompile(`\x1b\[[0-9;]*[a-zA-Z]`)

// stripANSI removes ANSI escape sequences from s.
func stripANSI(s string) string {
	return ansiEscRe.ReplaceAllString(s, "")
}

// appendToFile appends data to the named file, creating it if needed.
func appendToFile(path, data string) {
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return
	}
	defer f.Close()
	f.WriteString(data) //nolint:errcheck
}
