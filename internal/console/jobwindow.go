package console

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// startJobProcess runs shellCmd as a subprocess, writing stdout+stderr to
// logFile and writing the exit code to doneFile on completion.
// extraEnv is a space-separated list of KEY=VALUE pairs prepended to the env.
// startDir, if non-empty, sets the working directory for the subprocess.
// Returns (logFile, doneFile, error).
func startJobProcess(feedID, shellCmd, startDir, extraEnv string) (logFile, doneFile string, err error) {
	logFile = fmt.Sprintf("%s/glitch-%s.log", os.TempDir(), feedID)
	doneFile = fmt.Sprintf("%s/glitch-%s.done", os.TempDir(), feedID)

	// Pre-create empty log so the watcher can open it immediately.
	os.WriteFile(logFile, nil, 0o600) //nolint:errcheck

	expandedDir := expandTilde(startDir)

	// Build the full shell command: optional cd + optional env + shellCmd.
	fullCmd := shellCmd
	if expandedDir != "" {
		fullCmd = fmt.Sprintf("cd %q && %s", expandedDir, fullCmd)
	}
	if extraEnv != "" {
		fullCmd = extraEnv + " " + fullCmd
	}

	lf, err := os.OpenFile(logFile, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return logFile, doneFile, fmt.Errorf("open log file: %w", err)
	}

	cmd := exec.Command("sh", "-c", fullCmd)
	if expandedDir != "" {
		cmd.Dir = expandedDir
	}
	cmd.Stdout = lf
	cmd.Stderr = lf

	if err := cmd.Start(); err != nil {
		lf.Close()
		return logFile, doneFile, fmt.Errorf("start job: %w", err)
	}

	go func() {
		defer lf.Close()
		exitCode := 0
		if err := cmd.Wait(); err != nil {
			var exitErr *exec.ExitError
			if errors.As(err, &exitErr) {
				exitCode = exitErr.ExitCode()
			} else {
				exitCode = 1
			}
		}
		os.WriteFile(doneFile, []byte(strconv.Itoa(exitCode)), 0o600) //nolint:errcheck
	}()

	return logFile, doneFile, nil
}

// startLogWatcher launches a background goroutine that tails logFile for new
// content, sending FeedLineMsg values to ch. When doneFile appears it reads the
// exit code and sends jobDoneMsg (exit 0) or jobFailedMsg (non-zero), then exits.
func startLogWatcher(feedID, logFile, doneFile string, ch chan<- tea.Msg) {
	// No log file means no job was created (e.g. test mode). Signal
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
							if stepID, status, ok := parseStepStatus(stripANSI(line)); ok {
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

// terminalPane holds basic info about a non-glitch pane.
// Kept for API compatibility; always empty since tmux has been removed.
type terminalPane struct {
	id      string
	index   string
	command string
	size    string
}

// currentTmuxPane always returns empty string since tmux has been removed.
func currentTmuxPane() string { return "" }

// listTerminalPanes always returns nil since tmux has been removed.
func listTerminalPanes() []terminalPane { return nil }

// expandTilde replaces a leading "~" with the user's home directory.
// Paths that don't start with "~" are returned unchanged.
func expandTilde(path string) string {
	if !strings.HasPrefix(path, "~") {
		return path
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return path
	}
	return home + path[1:]
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
