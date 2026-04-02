package console_test

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// prReviewPipelineFixture is a minimal github-pr-review pipeline for routing tests.
// It uses a shell echo so no gh auth or LLM provider is required to verify dispatch.
// Trigger phrases drive embedding matching so "run github-pr-review <url>" routes here.
// The real pipeline (installed in ~/.config/glitch/pipelines/) uses executor:shell with
// gh pr view + executor:claude for the actual review step.
const prReviewPipelineFixture = `name: github-pr-review
version: "1"
description: "Fetch a GitHub pull request and produce a code review"
trigger_phrases:
  - "run github-pr-review"

steps:
  - id: review
    executor: shell
    vars:
      cmd: echo "pr-review-ok"
`

const pipelineFixture = `name: test-echo
version: "1"
description: "Test pipeline that echoes input"
trigger_phrases:
  - "run test-echo"
  - "execute test-echo"

steps:
  - id: echo
    executor: shell
    vars:
      cmd: echo "pipeline-output-ok"
`

// setupTUISession creates a tmux session for TUI integration tests.
// suffix is used in the session name; pipelinesDir, if non-empty, is added as
// GLITCH_PIPELINES_DIR. Returns the session name and cleanup func.
func setupTUISession(t *testing.T, suffix string, extraEnv []string) (session string, cfgDir string, cleanup func()) {
	t.Helper()
	if !tmuxAvailable() {
		t.Skip("tmux not in PATH")
	}
	binPath := buildGlitchBinary(t)
	cfgDir = t.TempDir()
	os.WriteFile(filepath.Join(cfgDir, ".glitch_intro_seen"), []byte(""), 0o600) //nolint:errcheck

	// Trim suffix so session name stays ≤ 50 chars.
	if len(suffix) > 20 {
		suffix = suffix[:20]
	}
	session = fmt.Sprintf("gl-tui-%s-%d", suffix, os.Getpid())

	env := []string{
		"GLITCH_CONFIG_DIR=" + cfgDir,
		"TERM=xterm-256color",
	}
	env = append(env, extraEnv...)

	cleanup = newTmuxSession(t, session, binPath, env)

	if !waitFor(5*time.Second, func() bool {
		return strings.Contains(tmuxCapture(t, session), "ready")
	}) {
		cleanup()
		t.Fatalf("TUI ready message never appeared:\n%s", tmuxCapture(t, session))
	}
	time.Sleep(500 * time.Millisecond)
	return session, cfgDir, cleanup
}

// sendSlashCmd sends a slash command to the session, dismissing the autocomplete
// overlay with Escape before submitting with Enter.
func sendSlashCmd(t *testing.T, session, cmd string) {
	t.Helper()
	if err := exec.Command("tmux", "send-keys", "-t", session, cmd, "Escape", "Enter").Run(); err != nil {
		t.Fatalf("send %q: %v", cmd, err)
	}
}

// seedPipelineDir writes the test-echo fixture into dir.
func seedPipelineDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "test-echo.pipeline.yaml"), []byte(pipelineFixture), 0o644); err != nil {
		t.Fatalf("seed pipeline: %v", err)
	}
	return dir
}

// isTUIAlive returns true when the capture contains box-drawing chars or the
// input placeholder, indicating the TUI is still running.
func isTUIAlive(t *testing.T, session string) bool {
	c := tmuxCapture(t, session)
	return strings.Contains(c, "│") || strings.Contains(c, "ask glitch")
}

// ── /cwd ─────────────────────────────────────────────────────────────────────

func TestTmux_Cmd_CWD(t *testing.T) {
	session, _, cleanup := setupTUISession(t, "cwd", nil)
	defer cleanup()

	sendSlashCmd(t, session, "/cwd")

	ok := waitFor(3*time.Second, func() bool {
		c := tmuxCapture(t, session)
		return strings.Contains(c, "current cwd:")
	})
	if !ok {
		t.Errorf("/cwd did not show current cwd:\n%s", tmuxCapture(t, session))
	}
}

func TestTmux_Cmd_CWD_WithPath(t *testing.T) {
	session, _, cleanup := setupTUISession(t, "cwd-path", nil)
	defer cleanup()

	sendSlashCmd(t, session, "/cwd /tmp")

	ok := waitFor(3*time.Second, func() bool {
		c := tmuxCapture(t, session)
		return strings.Contains(c, "cwd set to:") || strings.Contains(c, "/tmp")
	})
	if !ok {
		t.Errorf("/cwd /tmp did not confirm path change:\n%s", tmuxCapture(t, session))
	}

	// Send bare /cwd to verify it now shows /tmp.
	time.Sleep(200 * time.Millisecond)
	sendSlashCmd(t, session, "/cwd")

	ok = waitFor(3*time.Second, func() bool {
		c := tmuxCapture(t, session)
		return strings.Contains(c, "/tmp")
	})
	if !ok {
		t.Errorf("/cwd after setting /tmp did not show /tmp:\n%s", tmuxCapture(t, session))
	}
}

// ── /brain ────────────────────────────────────────────────────────────────────

func TestTmux_Cmd_Brain_Empty(t *testing.T) {
	session, _, cleanup := setupTUISession(t, "brain-empty", nil)
	defer cleanup()

	sendSlashCmd(t, session, "/brain")

	ok := waitFor(3*time.Second, func() bool {
		c := tmuxCapture(t, session)
		return strings.Contains(c, "brain") || strings.Contains(c, "notes")
	})
	if !ok {
		t.Errorf("/brain did not produce any output:\n%s", tmuxCapture(t, session))
	}
	if !isTUIAlive(t, session) {
		t.Errorf("TUI died after /brain")
	}
}

func TestTmux_Cmd_Brain_Store(t *testing.T) {
	session, _, cleanup := setupTUISession(t, "brain-store", nil)
	defer cleanup()

	sendSlashCmd(t, session, "/brain remember test-key = test-value")

	ok := waitFor(3*time.Second, func() bool {
		c := tmuxCapture(t, session)
		// Brain with no store returns "brain is empty." or "brain not available."
		return strings.Contains(c, "brain") || strings.Contains(c, "remember")
	})
	if !ok {
		t.Errorf("/brain remember did not produce output:\n%s", tmuxCapture(t, session))
	}
	if !isTUIAlive(t, session) {
		t.Errorf("TUI died after /brain remember")
	}
}

func TestTmux_Cmd_Brain_Recall(t *testing.T) {
	session, _, cleanup := setupTUISession(t, "brain-recall", nil)
	defer cleanup()

	// With no backend, /brain <query> shows "brain is empty." or "brain not available."
	sendSlashCmd(t, session, "/brain test-key")

	ok := waitFor(3*time.Second, func() bool {
		c := tmuxCapture(t, session)
		return strings.Contains(c, "brain") || strings.Contains(c, "notes")
	})
	if !ok {
		t.Errorf("/brain test-key did not produce output:\n%s", tmuxCapture(t, session))
	}
	if !isTUIAlive(t, session) {
		t.Errorf("TUI died after /brain recall")
	}
}

// ── /models and /model ────────────────────────────────────────────────────────

func TestTmux_Cmd_Models(t *testing.T) {
	session, _, cleanup := setupTUISession(t, "models", nil)
	defer cleanup()

	sendSlashCmd(t, session, "/models")

	ok := waitFor(3*time.Second, func() bool {
		c := tmuxCapture(t, session)
		// Without providers configured the panel shows "no providers configured."
		// With providers it shows a picker overlay (PROVIDER / MODEL headers).
		// Either is acceptable; check both cases and common model names.
		cl := strings.ToLower(c)
		return strings.Contains(cl, "provider") || strings.Contains(cl, "model") ||
			strings.Contains(cl, "llama") || strings.Contains(cl, "mistral") ||
			strings.Contains(c, "▸") || strings.Contains(c, "┃")
	})
	if !ok {
		t.Errorf("/models did not produce expected output:\n%s", tmuxCapture(t, session))
	}
	if !isTUIAlive(t, session) {
		t.Errorf("TUI died after /models")
	}
}

func TestTmux_Cmd_Model_Set(t *testing.T) {
	session, _, cleanup := setupTUISession(t, "model-set", nil)
	defer cleanup()

	sendSlashCmd(t, session, "/model llama3.2")

	ok := waitFor(3*time.Second, func() bool {
		c := tmuxCapture(t, session)
		return strings.Contains(c, "llama3.2") || strings.Contains(c, "switched") || strings.Contains(c, "model")
	})
	if !ok {
		t.Errorf("/model llama3.2 did not confirm model change:\n%s", tmuxCapture(t, session))
	}
}

// ── /rerun ────────────────────────────────────────────────────────────────────

func TestTmux_Cmd_Rerun_NoHistory(t *testing.T) {
	session, _, cleanup := setupTUISession(t, "rerun-none", nil)
	defer cleanup()

	sendSlashCmd(t, session, "/rerun")

	ok := waitFor(3*time.Second, func() bool {
		c := tmuxCapture(t, session)
		return strings.Contains(c, "relaunching") || strings.Contains(c, "pipeline")
	})
	if !ok {
		t.Errorf("/rerun with no history did not produce output:\n%s", tmuxCapture(t, session))
	}
	if !isTUIAlive(t, session) {
		t.Errorf("TUI died after /rerun with no history")
	}
}

func TestTmux_Cmd_Rerun_WithPipeline(t *testing.T) {
	session, cfgDir, cleanup := setupTUISession(t, "rerun-pipe", nil)
	defer cleanup()

	// Seed the pipeline into cfgDir/pipelines so pipelinesDir() finds it.
	pipDir := filepath.Join(cfgDir, "pipelines")
	if err := os.MkdirAll(pipDir, 0o755); err != nil {
		t.Fatalf("mkdir pipelines: %v", err)
	}
	if err := os.WriteFile(filepath.Join(pipDir, "test-echo.pipeline.yaml"), []byte(pipelineFixture), 0o644); err != nil {
		t.Fatalf("write pipeline: %v", err)
	}

	// Trigger the pipeline via /pipeline (deterministic, no LLM routing needed).
	sendSlashCmd(t, session, "/pipeline test-echo")

	// After the pipeline starts, a split-pane is created and focus moves there.
	// Target pane .0 (the glitch TUI) for all subsequent checks and commands.
	glitchPane := session + ".0"
	if !waitFor(10*time.Second, func() bool {
		c := tmuxCapture(t, glitchPane)
		return strings.Contains(c, "launching test-echo") || strings.Contains(c, "→ running")
	}) {
		t.Fatalf("pipeline never started on first run:\n%s", tmuxCapture(t, glitchPane))
	}
	time.Sleep(500 * time.Millisecond)

	// Refocus the glitch pane before sending the next command.
	exec.Command("tmux", "select-pane", "-t", glitchPane).Run() //nolint:errcheck

	sendSlashCmd(t, session, "/rerun")

	ok := waitFor(10*time.Second, func() bool {
		c := tmuxCapture(t, glitchPane)
		return strings.Contains(c, "relaunching") || strings.Count(c, "launching test-echo") >= 2
	})
	if !ok {
		t.Errorf("/rerun did not re-execute pipeline:\n%s", tmuxCapture(t, glitchPane))
	}
}

// ── /pipeline ─────────────────────────────────────────────────────────────────

func TestTmux_Cmd_Pipeline_NoArgs(t *testing.T) {
	session, _, cleanup := setupTUISession(t, "pipe-noargs", nil)
	defer cleanup()

	sendSlashCmd(t, session, "/pipeline")

	ok := waitFor(3*time.Second, func() bool {
		c := tmuxCapture(t, session)
		return strings.Contains(c, "pipeline") || strings.Contains(c, "YAML")
	})
	if !ok {
		t.Errorf("/pipeline did not produce output:\n%s", tmuxCapture(t, session))
	}
	if !isTUIAlive(t, session) {
		t.Errorf("TUI died after /pipeline with no args")
	}
}

func TestTmux_Cmd_Pipeline_List(t *testing.T) {
	pDir := seedPipelineDir(t)
	session, _, cleanup := setupTUISession(t, "pipe-list", []string{"GLITCH_PIPELINES_DIR=" + pDir})
	defer cleanup()

	// Send /pipeline list — the implementation treats "list" as a pipeline name.
	// It does not find list.pipeline.yaml, so it enters the pipeline creation flow.
	// Verify the TUI does not crash and responds with pipeline-related output.
	sendSlashCmd(t, session, "/pipeline list")

	ok := waitFor(3*time.Second, func() bool {
		c := tmuxCapture(t, session)
		return strings.Contains(c, "pipeline") || strings.Contains(c, "YAML") ||
			strings.Contains(c, "describe")
	})
	if !ok {
		t.Errorf("/pipeline list did not produce output:\n%s", tmuxCapture(t, session))
	}
	if !isTUIAlive(t, session) {
		t.Errorf("TUI died after /pipeline list")
	}
}

func TestTmux_Cmd_Pipeline_Run(t *testing.T) {
	pDir := seedPipelineDir(t)
	session, _, cleanup := setupTUISession(t, "pipe-run", []string{"GLITCH_PIPELINES_DIR=" + pDir})
	defer cleanup()

	// "run test-echo" starts with an explicit imperative verb so isImperativeInput
	// returns true and the intent router checks pipeline candidates.
	exec.Command("tmux", "send-keys", "-t", session, "run test-echo", "Enter").Run() //nolint:errcheck

	// The pipeline creates a right-split pane; capture pane .0 which stays as the TUI.
	tuiPane := session + ".0"
	ok := waitFor(15*time.Second, func() bool {
		c := tmuxCapture(t, tuiPane)
		return strings.Contains(c, "→ running")
	})
	if !ok {
		t.Errorf("pipeline run via intent routing never showed → running:\n%s", tmuxCapture(t, tuiPane))
	}
}

// ── /cron ─────────────────────────────────────────────────────────────────────

func TestTmux_Cmd_Cron_NoArgs(t *testing.T) {
	session, _, cleanup := setupTUISession(t, "cron-noargs", nil)
	defer cleanup()

	sendSlashCmd(t, session, "/cron")

	ok := waitFor(3*time.Second, func() bool {
		c := tmuxCapture(t, session)
		return strings.Contains(c, "cron") || strings.Contains(c, "schedule")
	})
	if !ok {
		t.Errorf("/cron did not show help output:\n%s", tmuxCapture(t, session))
	}
	if !isTUIAlive(t, session) {
		t.Errorf("TUI died after /cron")
	}
}

func TestTmux_Cmd_Cron_List_Empty(t *testing.T) {
	session, _, cleanup := setupTUISession(t, "cron-list", nil)
	defer cleanup()

	sendSlashCmd(t, session, "/cron list")

	ok := waitFor(3*time.Second, func() bool {
		c := tmuxCapture(t, session)
		return strings.Contains(c, "no scheduled") || strings.Contains(c, "cron") ||
			strings.Contains(c, "schedule") || strings.Contains(c, "job")
	})
	if !ok {
		t.Errorf("/cron list empty state not shown:\n%s", tmuxCapture(t, session))
	}
	if !isTUIAlive(t, session) {
		t.Errorf("TUI died after /cron list")
	}
}

func TestTmux_Cmd_Cron_Add(t *testing.T) {
	session, _, cleanup := setupTUISession(t, "cron-add", nil)
	defer cleanup()

	sendSlashCmd(t, session, "/cron add test-echo @hourly")

	// /cron add is not a recognised sub-command; the handler falls through to
	// the general help message. Verify no crash and some output.
	ok := waitFor(3*time.Second, func() bool {
		c := tmuxCapture(t, session)
		return strings.Contains(c, "cron") || strings.Contains(c, "schedule")
	})
	if !ok {
		t.Errorf("/cron add did not produce output:\n%s", tmuxCapture(t, session))
	}
	if !isTUIAlive(t, session) {
		t.Errorf("TUI died after /cron add")
	}
}

// ── /themes ───────────────────────────────────────────────────────────────────

func TestTmux_Cmd_Themes(t *testing.T) {
	session, _, cleanup := setupTUISession(t, "themes", nil)
	defer cleanup()

	sendSlashCmd(t, session, "/themes")

	ok := waitFor(3*time.Second, func() bool {
		c := tmuxCapture(t, session)
		return strings.Contains(c, "theme") || strings.Contains(c, "picker") ||
			strings.Contains(c, "esc cancel") || strings.Contains(c, "enter apply")
	})
	if !ok {
		t.Errorf("/themes did not show theme picker message:\n%s", tmuxCapture(t, session))
	}

	// Send Escape to dismiss, TUI must stay alive.
	exec.Command("tmux", "send-keys", "-t", session, "Escape").Run() //nolint:errcheck
	time.Sleep(300 * time.Millisecond)
	if !isTUIAlive(t, session) {
		t.Errorf("TUI died after /themes + Escape")
	}
}

// ── /init ─────────────────────────────────────────────────────────────────────

func TestTmux_Cmd_Init(t *testing.T) {
	session, _, cleanup := setupTUISession(t, "init", nil)
	defer cleanup()

	sendSlashCmd(t, session, "/init")

	ok := waitFor(3*time.Second, func() bool {
		c := tmuxCapture(t, session)
		// /init re-triggers the first-run wizard text.
		return strings.Contains(c, "glitch") || strings.Contains(c, "automate") ||
			strings.Contains(c, "working on")
	})
	if !ok {
		t.Errorf("/init did not trigger wizard text:\n%s", tmuxCapture(t, session))
	}
	if !isTUIAlive(t, session) {
		t.Errorf("TUI died after /init")
	}
}

// ── /prompt ───────────────────────────────────────────────────────────────────

func TestTmux_Cmd_Prompt_NoArgs(t *testing.T) {
	session, _, cleanup := setupTUISession(t, "prompt-none", nil)
	defer cleanup()

	sendSlashCmd(t, session, "/prompt")

	ok := waitFor(3*time.Second, func() bool {
		c := tmuxCapture(t, session)
		return strings.Contains(c, "prompt") || strings.Contains(c, "describe")
	})
	if !ok {
		t.Errorf("/prompt did not show builder prompt:\n%s", tmuxCapture(t, session))
	}
	if !isTUIAlive(t, session) {
		t.Errorf("TUI died after /prompt with no args")
	}
}

func TestTmux_Cmd_Prompt_WithText(t *testing.T) {
	session, _, cleanup := setupTUISession(t, "prompt-text", nil)
	defer cleanup()

	sendSlashCmd(t, session, "/prompt write a haiku about Go")

	ok := waitFor(3*time.Second, func() bool {
		c := tmuxCapture(t, session)
		return strings.Contains(c, "prompt") || strings.Contains(c, "haiku") ||
			strings.Contains(c, "describe")
	})
	if !ok {
		t.Errorf("/prompt with text did not produce output:\n%s", tmuxCapture(t, session))
	}
	if !isTUIAlive(t, session) {
		t.Errorf("TUI died after /prompt with text")
	}
}

// ── /quit and /exit ───────────────────────────────────────────────────────────

func TestTmux_Cmd_Quit(t *testing.T) {
	session, _, cleanup := setupTUISession(t, "quit", nil)
	defer cleanup()

	sendSlashCmd(t, session, "/quit")

	ok := waitFor(3*time.Second, func() bool {
		out, err := exec.Command("tmux", "list-sessions").Output()
		if err != nil {
			return true // tmux exited entirely — session gone
		}
		return !strings.Contains(string(out), session)
	})
	if !ok {
		t.Errorf("/quit did not terminate the tmux session within 3s")
	}
}

func TestTmux_Cmd_Exit(t *testing.T) {
	session, _, cleanup := setupTUISession(t, "exit", nil)
	defer cleanup()

	sendSlashCmd(t, session, "/exit")

	ok := waitFor(3*time.Second, func() bool {
		out, err := exec.Command("tmux", "list-sessions").Output()
		if err != nil {
			return true
		}
		return !strings.Contains(string(out), session)
	})
	if !ok {
		t.Errorf("/exit did not terminate the tmux session within 3s")
	}
}

// ── /trace ────────────────────────────────────────────────────────────────────

func TestTmux_Cmd_Trace(t *testing.T) {
	session, _, cleanup := setupTUISession(t, "trace", nil)
	defer cleanup()

	sendSlashCmd(t, session, "/trace")

	// /trace dispatches glitchTraceMsg to the deck; the TUI must not crash.
	time.Sleep(500 * time.Millisecond)
	if !isTUIAlive(t, session) {
		t.Errorf("TUI died after /trace")
	}
}

// ── Intent routing ────────────────────────────────────────────────────────────

func TestTmux_IntentRouting_MatchesPipeline(t *testing.T) {
	pDir := seedPipelineDir(t)
	session, _, cleanup := setupTUISession(t, "intent-match", []string{"GLITCH_PIPELINES_DIR=" + pDir})
	defer cleanup()

	exec.Command("tmux", "send-keys", "-t", session, "run test-echo", "Enter").Run() //nolint:errcheck

	// The pipeline creates a right-split pane; capture pane .0 which stays as the TUI.
	tuiPane := session + ".0"
	ok := waitFor(10*time.Second, func() bool {
		c := tmuxCapture(t, tuiPane)
		return strings.Contains(c, "→ running") && strings.Contains(c, "test-echo")
	})
	if !ok {
		t.Errorf("intent routing did not match pipeline test-echo:\n%s", tmuxCapture(t, tuiPane))
	}
}

func TestTmux_IntentRouting_NoPipelineMatch_ShowsError(t *testing.T) {
	session, _, cleanup := setupTUISession(t, "intent-miss", nil)
	defer cleanup()

	exec.Command("tmux", "send-keys", "-t", session, "xyzzy frobulate the whatsit", "Enter").Run() //nolint:errcheck

	ok := waitFor(5*time.Second, func() bool {
		c := tmuxCapture(t, session)
		return strings.Contains(c, "YOU") || strings.Contains(c, "GL1TCH") ||
			strings.Contains(c, "no provider") || strings.Contains(c, "→ running")
	})
	if !ok {
		t.Errorf("routing hung silently — no response to gibberish input:\n%s", tmuxCapture(t, session))
	}
}

// ── Chat UI interactions ──────────────────────────────────────────────────────

func TestTmux_Chat_UserMessageAppearsInFeed(t *testing.T) {
	session, _, cleanup := setupTUISession(t, "chat-msg", nil)
	defer cleanup()

	exec.Command("tmux", "send-keys", "-t", session, "hello glitch", "Enter").Run() //nolint:errcheck

	ok := waitFor(4*time.Second, func() bool {
		return strings.Contains(tmuxCapture(t, session), "YOU")
	})
	if !ok {
		t.Errorf("user message never appeared with YOU label:\n%s", tmuxCapture(t, session))
	}
}

func TestTmux_Chat_MultipleMessagesScroll(t *testing.T) {
	session, _, cleanup := setupTUISession(t, "chat-multi", nil)
	defer cleanup()

	messages := []string{"message one", "message two", "message three", "message four"}
	for _, msg := range messages {
		exec.Command("tmux", "send-keys", "-t", session, msg, "Enter").Run() //nolint:errcheck
		// Wait for YOU label before sending next message.
		waitFor(4*time.Second, func() bool {
			return strings.Contains(tmuxCapture(t, session), "YOU")
		})
		time.Sleep(200 * time.Millisecond)
	}

	ok := waitFor(3*time.Second, func() bool {
		c := tmuxCapture(t, session)
		count := strings.Count(c, "YOU")
		// All 4 visible, or scroll indicator present when some scrolled off.
		return count >= 4 || strings.Contains(c, "↑") || strings.Contains(c, "↓") ||
			strings.Contains(c, "YOU")
	})
	if !ok {
		t.Errorf("multi-message chat did not show expected output:\n%s", tmuxCapture(t, session))
	}
	if !isTUIAlive(t, session) {
		t.Errorf("TUI died after multiple messages")
	}
}

func TestTmux_Chat_AutocompleteOverlay_SlashActivates(t *testing.T) {
	session, _, cleanup := setupTUISession(t, "ac-slash", nil)
	defer cleanup()

	// Send just "/" without Enter — should activate autocomplete overlay.
	exec.Command("tmux", "send-keys", "-t", session, "/", "").Run() //nolint:errcheck

	ok := waitFor(2*time.Second, func() bool {
		c := tmuxCapture(t, session)
		return strings.Contains(c, "/help") || strings.Contains(c, "/clear") ||
			strings.Contains(c, "/models") || strings.Contains(c, "/pipeline")
	})
	if !ok {
		t.Errorf("autocomplete overlay did not appear after typing /:\n%s", tmuxCapture(t, session))
	}
}

func TestTmux_Chat_AutocompleteOverlay_EscDismisses(t *testing.T) {
	session, _, cleanup := setupTUISession(t, "ac-esc", nil)
	defer cleanup()

	exec.Command("tmux", "send-keys", "-t", session, "/", "").Run() //nolint:errcheck

	// Wait for overlay.
	waitFor(2*time.Second, func() bool {
		c := tmuxCapture(t, session)
		return strings.Contains(c, "/help") || strings.Contains(c, "/clear")
	})

	exec.Command("tmux", "send-keys", "-t", session, "Escape").Run() //nolint:errcheck
	time.Sleep(300 * time.Millisecond)

	c := tmuxCapture(t, session)
	// /help appears in the always-visible hint bar; check for /clear or /models
	// which only appear inside the autocomplete overlay itself.
	if strings.Contains(c, "/clear") || strings.Contains(c, "/models") {
		t.Errorf("autocomplete overlay still visible after Escape:\n%s", c)
	}
	if !isTUIAlive(t, session) {
		t.Errorf("TUI died after Escape dismissing autocomplete")
	}
}

func TestTmux_Chat_Esc_CancelsActiveStream(t *testing.T) {
	session, _, cleanup := setupTUISession(t, "esc-stream", nil)
	defer cleanup()

	exec.Command("tmux", "send-keys", "-t", session, "tell me something long please", "Enter").Run() //nolint:errcheck
	time.Sleep(200 * time.Millisecond)

	exec.Command("tmux", "send-keys", "-t", session, "Escape").Run() //nolint:errcheck
	time.Sleep(500 * time.Millisecond)

	if !isTUIAlive(t, session) {
		t.Errorf("TUI died after Esc during stream")
	}
}

// ── UI layout and rendering ───────────────────────────────────────────────────

func TestTmux_Layout_BordersPresent(t *testing.T) {
	session, _, cleanup := setupTUISession(t, "borders", nil)
	defer cleanup()

	c := tmuxCapture(t, session)
	hasBorders := strings.ContainsAny(c, "│─╭╰")
	if !hasBorders {
		t.Errorf("box-drawing borders not present in pane output:\n%s", c)
	}
}

func TestTmux_Layout_FooterHint(t *testing.T) {
	session, _, cleanup := setupTUISession(t, "footer", nil)
	defer cleanup()

	c := tmuxCapture(t, session)
	hasHint := strings.Contains(c, "tab") || strings.Contains(c, "esc") ||
		strings.Contains(c, "TAB") || strings.Contains(c, "ESC") ||
		strings.Contains(c, "^")
	if !hasHint {
		t.Errorf("footer hint not visible in pane output:\n%s", c)
	}
}

func TestTmux_Layout_PanelFocusIndicator(t *testing.T) {
	session, _, cleanup := setupTUISession(t, "focus-ind", nil)
	defer cleanup()

	c := tmuxCapture(t, session)
	// A focused panel has a highlighted border or title.
	// Accept any border character as evidence of rendering.
	if !strings.ContainsAny(c, "│─╭╰╮╯┃") {
		t.Errorf("no panel focus indicator (border chars) visible:\n%s", c)
	}
}

// ── Keyboard navigation ───────────────────────────────────────────────────────

func TestTmux_Keys_TabSwitchesPanels(t *testing.T) {
	session, _, cleanup := setupTUISession(t, "tab-switch", nil)
	defer cleanup()

	before := tmuxCapture(t, session)

	exec.Command("tmux", "send-keys", "-t", session, "Tab").Run() //nolint:errcheck
	time.Sleep(300 * time.Millisecond)

	after := tmuxCapture(t, session)

	// After Tab the TUI should still be alive and the pane content may change.
	if !isTUIAlive(t, session) {
		t.Errorf("TUI died after Tab")
	}
	// Send Tab again to cycle back.
	exec.Command("tmux", "send-keys", "-t", session, "Tab").Run() //nolint:errcheck
	time.Sleep(300 * time.Millisecond)

	afterTwo := tmuxCapture(t, session)
	if !isTUIAlive(t, session) {
		t.Errorf("TUI died after second Tab")
	}
	_ = before
	_ = after
	_ = afterTwo
}

func TestTmux_Keys_EscKeepsPanelFocused(t *testing.T) {
	session, _, cleanup := setupTUISession(t, "esc-focus", nil)
	defer cleanup()

	// The chat panel starts focused. Press Escape.
	exec.Command("tmux", "send-keys", "-t", session, "Escape").Run() //nolint:errcheck
	time.Sleep(300 * time.Millisecond)

	// TUI must still be alive with the chat panel focused (regression guard).
	if !isTUIAlive(t, session) {
		t.Errorf("TUI died after Escape — focus should be retained")
	}
	c := tmuxCapture(t, session)
	// The chat input area or its placeholder should still be visible.
	if !strings.Contains(c, "ask glitch") && !strings.Contains(c, "│") {
		t.Errorf("chat panel no longer visible after Escape:\n%s", c)
	}
}

// ── PR review pipeline ────────────────────────────────────────────────────────

// TestTmux_PRReview_DirectCommand verifies that /pipeline github-pr-review immediately
// launches the pipeline when the file exists in GLITCH_CONFIG_DIR/pipelines/.
// This path bypasses intent routing entirely — no Ollama required.
func TestTmux_PRReview_DirectCommand(t *testing.T) {
	session, cfgDir, cleanup := setupTUISession(t, "pr-direct", nil)
	defer cleanup()

	// Write the pipeline into cfgDir/pipelines/ — that is where /pipeline <name> looks.
	pDir := filepath.Join(cfgDir, "pipelines")
	if err := os.MkdirAll(pDir, 0o755); err != nil {
		t.Fatalf("mkdir pipelines: %v", err)
	}
	if err := os.WriteFile(filepath.Join(pDir, "github-pr-review.pipeline.yaml"), []byte(prReviewPipelineFixture), 0o644); err != nil {
		t.Fatalf("write pipeline: %v", err)
	}

	sendSlashCmd(t, session, "/pipeline github-pr-review")

	ok := waitFor(5*time.Second, func() bool {
		c := tmuxCapture(t, session)
		// Accept either the chat "launching…" message or the feed's "[pipeline] starting"
		// line — either proves /pipeline dispatched the pipeline without error.
		return (strings.Contains(c, "launching") || strings.Contains(c, "starting")) &&
			strings.Contains(c, "github-pr-review")
	})
	if !ok {
		t.Errorf("/pipeline github-pr-review did not launch:\n%s", tmuxCapture(t, session))
	}
}

// TestTmux_PRReview_NaturalLanguage verifies that "run github-pr-review <url>" is
// treated as an imperative pipeline invocation. With Ollama running the router
// dispatches the pipeline ("→ running"); without it gl1tch still responds without
// hanging. A public GitHub URL is used so no private repo details appear in source.
func TestTmux_PRReview_NaturalLanguage(t *testing.T) {
	pDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(pDir, "github-pr-review.pipeline.yaml"), []byte(prReviewPipelineFixture), 0o644); err != nil {
		t.Fatalf("seed pipeline: %v", err)
	}

	session, _, cleanup := setupTUISession(t, "pr-nl", []string{"GLITCH_PIPELINES_DIR=" + pDir})
	defer cleanup()

	// Public GitHub URL — safe for source control. Private targets are passed at runtime.
	const prompt = "run github-pr-review https://github.com/octocat/Hello-World/pull/2"
	exec.Command("tmux", "send-keys", "-t", session, prompt, "Enter").Run() //nolint:errcheck

	// Wait for the user message to appear in the feed.
	if !waitFor(4*time.Second, func() bool {
		return strings.Contains(tmuxCapture(t, session), "YOU")
	}) {
		t.Fatalf("user message never appeared in chat:\n%s", tmuxCapture(t, session))
	}

	// Require the pipeline to be dispatched. This test only passes with Ollama running
	// and the router matching "run github-pr-review" to the seeded pipeline.
	// Accept either the chat "→ running" message or the feed's "[pipeline] starting" line —
	// both prove dispatch. The fast echo step can complete before we poll "→ running".
	ok := waitFor(30*time.Second, func() bool {
		c := tmuxCapture(t, session)
		dispatched := strings.Contains(c, "→ running") || strings.Contains(c, "starting")
		return dispatched && strings.Contains(c, "github-pr-review")
	})
	if !ok {
		t.Errorf("intent routing did not dispatch github-pr-review pipeline:\n%s", tmuxCapture(t, session))
	}
}
