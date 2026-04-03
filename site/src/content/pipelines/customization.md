---
title: "Customization"
description: "Change the voice, panel names, and messages in your workspace — make gl1tch sound like you."
order: 55
---

gl1tch ships with a default personality: dry, confident, hacker-adjacent. If that's not your vibe, every label, panel title, modal message, and onboarding script is overridable. One YAML file, no restart required.


## Quick Start

Create `~/.config/glitch/translations.yaml` and override any key you want:

```yaml
deck_header_title: "MY WORKSPACE"
signal_board_panel_title: "JOBS"
quit_modal_message: "close everything?"
```

Reopen gl1tch and your changes are live.


## How It Works

Every visible string in your workspace has a key. At startup gl1tch resolves each key through three layers in order:

```text
~/.config/glitch/translations.yaml   ← your overrides (highest priority)
        │
theme strings                         ← optional strings bundled with your theme
        │
built-in defaults                     ← shipped gl1tch personality
```

The first layer that has a value for a key wins. You only need to define the keys you want to change — everything else falls through to the default.


## Panel and Modal Labels

These keys control what you see in the UI chrome.

| Key | Default | What it labels |
|-----|---------|----------------|
| `deck_header_title` | `GL1TCH` | Top header bar |
| `pipelines_panel_title` | `PIPELINES` | Pipeline list panel |
| `agent_runner_panel_title` | `AGENT RUNNER` | Agent runner panel |
| `signal_board_panel_title` | `SIGNAL BOARD` | Live job status view |
| `activity_feed_panel_title` | `ACTIVITY FEED` | Activity feed |
| `inbox_panel_title` | `INBOX` | Run results inbox |
| `cron_panel_title` | `CRON JOBS` | Scheduled jobs panel |
| `quit_modal_title` | `BAIL OUT` | Quit confirmation modal title |
| `quit_modal_message` | `you sure? the grid will still be here.` | Quit confirmation message |
| `help_modal_title` | `GETTING STARTED` | Help modal title |
| `theme_picker_title` | `SELECT THEME` | Theme picker title |
| `theme_picker_dark_tab` | `Dark` | Dark themes tab label |
| `theme_picker_light_tab` | `Light` | Light themes tab label |
| `rerun_context_label` | `ADDITIONAL CONTEXT` | Re-run modal context field |
| `rerun_cwd_label` | `WORKING DIRECTORY` | Re-run modal directory field |


## Help Modal

These keys control the section headers and key binding descriptions in the built-in help screen (`ctrl+space ?`).

| Key | Default |
|-----|---------|
| `help_chord_note` | `Chord prefix: ^spc  (ctrl+space, then the key below)` |
| `help_section_system` | `HELP & SYSTEM` |
| `help_section_workspace` | `WORKSPACE` |
| `help_section_windows` | `WINDOWS & PANES` |
| `help_section_nav` | `NAVIGATION` |
| `help_section_panels` | `PANELS` |
| `help_bind_help` | `this help` |
| `help_bind_quit` | `quit GLITCH` |
| `help_bind_detach` | `detach  (session stays alive)` |
| `help_bind_reload` | `reload  (hot-swap binary)` |
| `help_bind_themes` | `theme picker` |
| `help_bind_jump` | `jump to any window` |
| `help_bind_new_win` | `new window` |
| `help_bind_prev_win` | `previous / next window` |
| `help_bind_split_right` | `split pane right / down` |
| `help_bind_nav_pane` | `navigate panes` |
| `help_bind_kill` | `kill pane / window` |
| `help_bind_tab_nav` | `navigate panels & list items` |
| `help_bind_enter` | `open / confirm / run selected` |
| `help_bind_esc` | `back / close overlay` |
| `help_panel_pipelines` | `run and monitor named pipelines` |
| `help_panel_agent_runner` | `launch and wrangle AI agents` |
| `help_panel_signal_board` | `inter-agent message bus` |
| `help_panel_activity_feed` | `live events, run history, logs` |
| `help_panel_cron` | `scheduled pipelines and agent jobs` |


## Welcome Onboarding

The first time you open gl1tch, you see a scripted onboarding conversation. These are fully replaceable — write your own intro, change the tone, drop in your team's context.

| Key | What it controls |
|-----|-----------------|
| `welcome_phase_intro` | First message — sets the scene and asks what you're building |
| `welcome_phase_use_case` | Response after you describe your use case |
| `welcome_phase_providers` | Explains local vs cloud providers |
| `welcome_phase_pipeline` | Walks through pipeline basics |
| `welcome_phase_nav` | Explains workspace navigation |
| `welcome_phase_brain` | Explains the brain memory system |
| `welcome_phase_done` | Closing message |

Example — replacing just the intro:

```yaml
welcome_phase_intro: |
  welcome to your workspace.

  i'm gl1tch. i run your pipelines, remember what your agents learn,
  and stay out of your way the rest of the time.

  what are you working on?
```

> **NOTE:** The onboarding only runs once on first launch. To re-run it, delete `~/.local/share/glitch/glitch.db` — or just run the welcome assistant directly with `glitch assistant`.


## ANSI Colors in Values

Any value can include ANSI escape sequences for color. Use the shorthand forms — gl1tch expands them automatically:

```yaml
deck_header_title: "\e[1;35mMY GRID\e[0m"
quit_modal_message: "\e[31myou sure?\e[0m  the grid will still be here."
```

Supported shorthand forms:

| Write | Expands to |
|-------|-----------|
| `\e[` | raw ESC byte |
| `\033[` | raw ESC byte |
| `\x1b[` | raw ESC byte |

Standard ANSI color codes: `\e[31m` red, `\e[32m` green, `\e[33m` yellow, `\e[34m` blue, `\e[35m` magenta, `\e[36m` cyan, `\e[1m` bold, `\e[0m` reset.


## Bundling Strings in a Theme

If you're building a theme and want the copy to match your aesthetic, add a `strings` block to your `theme.yaml`. These sit between your personal `translations.yaml` and the built-in defaults — so your personal overrides always win.

```yaml
name: my-theme
display_name: "My Theme"
mode: dark

palette:
  # ... colors

strings:
  deck_header_title: "SYSTEM"
  signal_board_panel_title: "OPS"
  quit_modal_title: "DISCONNECT"
  quit_modal_message: "terminate session?"
```

See [Themes](/docs/pipelines/themes) for the full theme format.


## Examples


### Corporate-friendly workspace

```yaml
deck_header_title: "WORKSPACE"
pipelines_panel_title: "AUTOMATIONS"
signal_board_panel_title: "ACTIVE JOBS"
agent_runner_panel_title: "RUN"
quit_modal_title: "EXIT"
quit_modal_message: "Close gl1tch?"
help_modal_title: "KEYBOARD SHORTCUTS"
```


### Minimal, no personality

```yaml
deck_header_title: "gl1tch"
quit_modal_title: "quit"
quit_modal_message: "exit?"
inbox_panel_title: "runs"
cron_panel_title: "scheduled"
signal_board_panel_title: "live"
```


### ANSI-styled header

```yaml
deck_header_title: "\e[1;36mGL1TCH\e[0m  \e[2m//  your workspace\e[0m"
```


## See Also

- [Themes](/docs/pipelines/themes) — change colors, borders, and layout alongside your copy
- [Plugins](/docs/pipelines/plugins) — extend what your workspace can do
