package switchboard

import (
	"os"
	"os/exec"
	"strings"
)

// buildDebugPopup renders an 80%-wide centered overlay box showing the captured
// content of the tmux window for the given job. tmuxWindow is the fully-qualified
// target "session:orcai-<feedID>" stored in jobHandle.
func buildDebugPopup(termH, termW int, tmuxWindow string) string {
	popW := termW * 80 / 100
	if popW < 40 {
		popW = 40
	}
	popH := termH * 80 / 100
	if popH < 10 {
		popH = 10
	}

	// Capture the tmux pane output for the job's window.
	var content string
	if tmuxWindow == "" {
		content = "  no tmux window associated with this job"
	} else {
		out, err := exec.Command("tmux", "capture-pane", "-t", tmuxWindow, "-p", "-e").Output()
		if err != nil {
			content = "  window closed or not available"
		} else {
			content = string(out)
		}
	}

	title := tmuxWindow
	if title == "" {
		title = "DEBUG"
	}
	return renderPopupBox(popH, popW, title, content)
}

// renderPopupBox draws a bordered box of height h and width w, with the given
// title in the top border and content inside.
func renderPopupBox(h, w int, title, content string) string {
	if h < 3 {
		h = 3
	}
	if w < 10 {
		w = 10
	}

	bodyH := h - 2 // top + bottom border

	// Split content into lines and clip/pad.
	contentLines := strings.Split(content, "\n")
	var bodyLines []string
	for _, line := range contentLines {
		// Strip trailing whitespace.
		line = strings.TrimRight(line, " \t\r")
		// Wrap/clip to fit inside box (w-2 for borders, -2 for padding).
		innerW := w - 4
		if innerW < 1 {
			innerW = 1
		}
		if len(line) > innerW {
			line = line[:innerW]
		}
		bodyLines = append(bodyLines, "  "+line)
		if len(bodyLines) >= bodyH {
			break
		}
	}
	// Pad if fewer lines than bodyH.
	for len(bodyLines) < bodyH {
		bodyLines = append(bodyLines, "")
	}

	// Build the box.
	label := " " + title + " "
	dashes := max(w-2-len(label), 0)
	left := dashes / 2
	right := dashes - left
	topBorder := aPur + "┌" + strings.Repeat("─", left) + aBrC + label + aPur + strings.Repeat("─", right) + "┐" + aRst

	var rows []string
	rows = append(rows, topBorder)
	for i := 0; i < bodyH; i++ {
		line := ""
		if i < len(bodyLines) {
			line = bodyLines[i]
		}
		// Pad line to fill inner width.
		innerW := w - 2
		padded := line
		vl := len(padded) // approximate vis length (no ANSI in captured pane)
		if vl < innerW {
			padded += strings.Repeat(" ", innerW-vl)
		}
		if len(padded) > innerW {
			padded = padded[:innerW]
		}
		rows = append(rows, aPur+"│"+aRst+padded+aPur+"│"+aRst)
	}

	botBorder := aPur + "└" + strings.Repeat("─", max(w-2, 0)) + "┘" + aRst
	rows = append(rows, botBorder)

	return strings.Join(rows, "\n")
}

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

// createJobWindow creates a detached tmux window named "orcai-<feedID>" in the
// current session. Returns the fully-qualified target "session:window" so
// subsequent tmux commands can address it unambiguously.
// Returns an empty string if tmux is not available.
func createJobWindow(feedID string) string {
	if _, err := exec.LookPath("tmux"); err != nil {
		return ""
	}
	session := currentTmuxSession()
	windowName := "orcai-" + feedID
	// Create the window detached in the current session.
	target := session + ":" + windowName
	exec.Command("tmux", "new-window", "-d", "-t", session, "-n", windowName).Run() //nolint:errcheck
	return target
}
