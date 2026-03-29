package switchboard

import (
	"os"
	"os/exec"
	"runtime"
	"strings"
)

// readClipboard returns the current system clipboard contents, or empty string
// on any error or unsupported platform.
func readClipboard() string {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("pbpaste")
	case "linux":
		cmd = exec.Command("xclip", "-o", "-selection", "clipboard")
		if _, err := exec.LookPath("xclip"); err != nil {
			cmd = exec.Command("xsel", "--clipboard", "--output")
		}
	default:
		return ""
	}
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return string(out)
}

// clipboardSnapshot captures the current clipboard state for change detection.
func clipboardSnapshot() string {
	return readClipboard()
}

// editorCmd returns the editor executable to use and true, or ("", false) if
// no usable editor is found. Prefers $EDITOR, falls back to vi.
func editorCmd() (string, bool) {
	if e := strings.TrimSpace(os.Getenv("EDITOR")); e != "" {
		return e, true
	}
	if path, err := exec.LookPath("vi"); err == nil {
		return path, true
	}
	return "", false
}
