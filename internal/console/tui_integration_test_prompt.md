# TUI Integration Test Suite — Implementation Brief

## Goal

Write a comprehensive tmux-based integration test file at
`internal/console/tui_integration_test.go` that exercises the full glitch TUI
end-to-end. This is an addition to the existing tests in:

- `tmux_integration_test.go` — startup, /help, /clear, chat response
- `terminal_integration_test.go` — /terminal command
- `terminal_nl_integration_test.go` — /terminal natural language

Do **not** duplicate those tests. Cover everything they leave unaddressed.

---

## Test Infrastructure — Reuse These Helpers (already in tmux_integration_test.go)

```go
tmuxAvailable() bool
buildGlitchBinary(t) string               // compiles fresh binary
newTmuxSession(t, name, cmd, env) func()  // 220×50 session, returns cleanup
tmuxCapture(t, session) string            // capture-pane -p output
tmuxSend(session, keys...) error          // send-keys to session
waitFor(maxWait, fn) bool                 // poll fn every 200ms
```

All tests **must**:
1. `if !tmuxAvailable() { t.Skip("tmux not in PATH") }`
2. Call `buildGlitchBinary(t)` at the top
3. Use unique session names: `fmt.Sprintf("glitch-test-%s-%d", shortName, os.Getpid())`
4. Pre-create the sentinel: `os.WriteFile(filepath.Join(cfgDir, ".glitch_intro_seen"), []byte(""), 0o600)`
5. Set env: `GLITCH_CONFIG_DIR=cfgDir`, `TERM=xterm-256color`
6. `defer cleanup()` immediately after `newTmuxSession`
7. Wait for `"ready"` before sending any keys (5s max), then `time.Sleep(500ms)` to stabilize
8. Use `exec.Command("tmux", "send-keys", "-t", session, inputText, "Escape", "Enter").Run()` for slash
   commands — the `"Escape"` dismisses the autocomplete overlay `/` activates

For pipeline tests, also set `GLITCH_PIPELINES_DIR=pipelinesDir` in env, where
`pipelinesDir` is a temp directory with a minimal pipeline YAML seeded into it.

---

## Minimal Pipeline Fixture

Tests that exercise pipeline-related commands need at least one pipeline on disk.
Use this minimal fixture (seed it into a temp `pipelinesDir`):

```yaml
name: test-echo
version: "1"
description: "Test pipeline that echoes input"

steps:
  - name: echo
    kind: shell
    run: echo "pipeline-output-ok"
```

Write it as: `filepath.Join(pipelinesDir, "test-echo.pipeline.yaml")`

The pipeline trigger text (what the user types to match it via the router) is the
pipeline name: `"test-echo"` or phrased naturally. For integration tests that only
need to verify `/pipeline` or `/rerun` command dispatch (not actual execution),
seeding the file is sufficient.

---

## Tests to Write

### 1. Slash Command: /cwd

**`TestTmux_Cmd_CWD`**
- Send `/cwd` (no args)
- Verify the chat shows the current working directory (a path string appears in output)
- The pane content should contain an absolute path

**`TestTmux_Cmd_CWD_WithPath`**
- Send `/cwd /tmp`
- Verify chat shows confirmation that cwd changed (contains "/tmp" or "set cwd")
- Send `/cwd` again, verify it now shows `/tmp`

---

### 2. Slash Command: /brain

**`TestTmux_Cmd_Brain_Empty`**
- Send `/brain` with no existing brain notes (fresh cfgDir)
- Verify a sensible empty-state response appears (no crash, meaningful message)

**`TestTmux_Cmd_Brain_Store`**
- Send `/brain remember test-key = test-value`
- Verify chat confirms the note was stored (any acknowledgement)

**`TestTmux_Cmd_Brain_Recall`**
- Store a note, then send `/brain test-key`
- Verify the stored value appears in the response

---

### 3. Slash Command: /models and /model

**`TestTmux_Cmd_Models`**
- Send `/models`
- Verify a model picker overlay or model list appears (contains known Ollama model
  names or a picker UI element — look for list borders or model name patterns)
- Verify ESC dismisses without crashing (TUI still alive, "ready" or prompt visible)

**`TestTmux_Cmd_Model_Set`**
- Send `/model llama3.2`
- Verify chat shows a confirmation (model name appears in response, or prompt
  footer/header reflects the model change)

---

### 4. Slash Command: /rerun

**`TestTmux_Cmd_Rerun_NoHistory`**
- Send `/rerun` with no prior pipeline runs (fresh cfgDir)
- Verify a sensible message (no crash; "no pipeline" or similar)

**`TestTmux_Cmd_Rerun_WithPipeline`**
- Seed `pipelinesDir` with the minimal `test-echo` fixture
- Trigger a pipeline run by typing `"test-echo"` and Enter (intent router picks it up)
- Wait for `"→ running"` to appear in the pane
- Then send `/rerun`
- Verify the pipeline re-executes (second `"→ running"` appears, or output repeats)

---

### 5. Slash Command: /pipeline

**`TestTmux_Cmd_Pipeline_NoArgs`**
- Send `/pipeline` (no subcommand)
- Verify help text or subcommand list appears (e.g. "run", "list", or usage hint)

**`TestTmux_Cmd_Pipeline_List`**
- Seed `pipelinesDir` with the `test-echo` fixture
- Send `/pipeline list`
- Verify `test-echo` appears in the chat output

**`TestTmux_Cmd_Pipeline_Run`**
- Seed `pipelinesDir` with the `test-echo` fixture  
- Send `/pipeline run test-echo`
- Verify `"→ running"` appears, then eventually `"pipeline-output-ok"` or a
  completion signal shows in the feed (may take up to 15s)

---

### 6. Slash Command: /cron

**`TestTmux_Cmd_Cron_NoArgs`**
- Send `/cron`
- Verify a cron help or status output appears (no crash)

**`TestTmux_Cmd_Cron_List_Empty`**
- Send `/cron list` in a fresh cfgDir
- Verify an empty state message or empty list (no crash)

**`TestTmux_Cmd_Cron_Add`**
- Send `/cron add test-echo @hourly`
- Verify acknowledgement (schedule stored, confirms name and schedule)

---

### 7. Slash Command: /themes

**`TestTmux_Cmd_Themes`**
- Send `/themes`
- Verify a theme picker or theme list appears (UI overlay or theme names)
- Verify ESC dismisses it without crashing

---

### 8. Slash Command: /init

**`TestTmux_Cmd_Init`**
- Send `/init`
- Verify the first-run wizard is triggered (intro text or wizard prompt appears)
  NOTE: do NOT pre-create the sentinel if testing /init itself, or verify the
  command re-triggers the wizard even when sentinel exists

---

### 9. Slash Command: /prompt

**`TestTmux_Cmd_Prompt_NoArgs`**
- Send `/prompt` (no args)
- Verify prompt builder UI or help appears (no crash)

**`TestTmux_Cmd_Prompt_WithText`**
- Send `/prompt write a haiku about Go`
- Verify the prompt text appears in the chat, or builder is pre-filled

---

### 10. Slash Command: /quit and /exit

**`TestTmux_Cmd_Quit`**
- Send `/quit`
- Verify the tmux session exits (pane disappears or session becomes dead within 3s)
- Use `exec.Command("tmux", "list-sessions").Output()` and confirm session name
  is absent after the command

**`TestTmux_Cmd_Exit`**
- Same as /quit but using `/exit`

---

### 11. Slash Command: /trace (developer diagnostic)

**`TestTmux_Cmd_Trace`**
- Send `/trace`
- Verify some diagnostic output appears (no crash; trace info or "trace" in output)

---

### 12. Intent Routing → Pipeline Execution

**`TestTmux_IntentRouting_MatchesPipeline`**
- Seed `pipelinesDir` with the `test-echo` fixture
- Type `"run test-echo"` (natural language that should match the pipeline)
- Verify `"→ running"` appears in the feed within 10s
- Verify the pipeline name `"test-echo"` appears somewhere in the output

**`TestTmux_IntentRouting_NoPipelineMatch_ShowsError`**
- Fresh cfgDir, no pipelines
- Type `"xyzzy frobulate the whatsit"` (gibberish, won't match anything)
- Verify GL1TCH responds — either LLM reply or `"no provider"` error message
- This is a regression guard: routing must never hang silently

---

### 13. Chat UI Interactions

**`TestTmux_Chat_UserMessageAppearsInFeed`**
- Type any message, Enter
- Verify `"YOU"` label appears in the feed with the typed text nearby

**`TestTmux_Chat_MultipleMessagesScroll`**
- Send 4 messages in sequence (wait for each "YOU" label before sending next)
- Verify all 4 `"YOU"` appearances in captured output OR scroll indicators appear
- Verifies feed doesn't corrupt or crash under multi-turn conversation

**`TestTmux_Chat_AutocompleteOverlay_SlashActivates`**
- Wait for ready, then send just `"/"` (no Enter)
- Verify autocomplete overlay is visible (contains `/help` or `/clear` or `"/models"`)
- This is an integration-level companion to the unit test of the same name

**`TestTmux_Chat_AutocompleteOverlay_EscDismisses`**
- Send `"/"`, wait for overlay, send Escape
- Verify overlay is gone (no `/help` visible) but TUI is alive (input placeholder visible)

**`TestTmux_Chat_Esc_CancelsActiveStream`**
- (Only meaningful with a provider configured; without one, verify no crash)
- Start a message, press Escape before response completes
- Verify the input is cleared OR stream stops (no hang)

---

### 14. UI Layout and Rendering

**`TestTmux_Layout_BordersPresent`**
- Start TUI, wait for ready
- Capture pane and verify box-drawing characters are present (`│`, `─`, `╭`, `╰`)
- Verifies the TUI renders its panel borders in the terminal

**`TestTmux_Layout_FooterHint`**
- Start TUI, capture after ready
- Verify the hint footer is visible (contains key hint text like `"tab"` or `"esc"`)

**`TestTmux_Layout_PanelFocusIndicator`**
- Start TUI
- Verify the focused panel has a visible focus indicator in its border or header

---

### 15. Keyboard Navigation

**`TestTmux_Keys_TabSwitchesPanels`**
- Send Tab key
- Verify a different panel becomes focused (hint footer changes, or border style shifts)
- Send Tab again — verify focus cycles back

**`TestTmux_Keys_EscKeepsPanelFocused`**
- (Regression: esc used to drop focus entirely)
- Focus on chat panel, press Escape
- Verify chat panel is still focused (hint footer still shows chat keys)
  This is an integration companion to the unit regression test.

---

## Implementation Notes

1. **Timeouts**: Slash commands that don't invoke LLM (e.g. /cwd, /clear, /themes)
   should resolve within 3s. Intent routing with no provider resolves within 5s.
   Pipeline execution can take up to 30s.

2. **Detecting "alive"**: After any command, verifying the TUI is still alive means
   checking that `tmuxCapture` returns non-empty output containing `"│"` or `"ask glitch"`.

3. **Detecting model picker**: If /models shows a picker overlay, look for
   `"┃"` or `"▸"` or known model name patterns like `"llama"` or `"mistral"`.

4. **Detecting cron output**: cron command output likely contains `"@"` (cron schedule
   syntax) or the word `"schedule"` or `"cron"`.

5. **Detecting /brain output**: Look for the note key/value, or `"brain"` in the output.
   The exact format depends on the brain store implementation.

6. **Pipeline feed line**: When a pipeline is triggered, the feed (left panel) shows
   `"→ running test-echo"` or similar. After completion, a done marker appears.

7. **No assertions on exact strings for LLM output**: When testing paths that go through
   the LLM, assert on structural markers (`"GL1TCH"`, `"YOU"`, `"→ running"`) not
   specific text content.

8. **Test naming convention**: All new tests use `TestTmux_` prefix for consistency
   with existing integration tests.

9. **Session name uniqueness**: Include the test function name in the session name
   (trimmed to avoid tmux's 50-char limit) so parallel test runs don't collide.

10. **Package**: `package console_test` (external test package, same as existing tests)

---

## File to Create

`/Users/stokes/Projects/gl1tch/internal/console/tui_integration_test.go`

Imports needed:
```go
import (
    "fmt"
    "os"
    "os/exec"
    "path/filepath"
    "strings"
    "testing"
    "time"
)
```

Do not import the `console` package unless testing an exported function — all
behavior is verified via the TUI binary running in tmux.
