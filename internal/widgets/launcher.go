package widgets

import (
	"fmt"
	"os/exec"
)

// Launch opens a new tmux window named after the widget in the given tmux
// session and runs the widget binary inside it. The widget binary is
// responsible for connecting to the busd socket and registering itself.
//
// Returns an error if the tmux command itself fails to execute. Note that tmux
// may return success even if the widget binary is not found — the shell inside
// the tmux window will handle that error.
func Launch(m Manifest, tmuxSession string) error {
	cmd := exec.Command("tmux", "new-window",
		"-t", tmuxSession,
		"-n", m.Name,
		m.Binary,
	)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("widgets: launch %s in tmux session %s: %w", m.Name, tmuxSession, err)
	}
	return nil
}
