## Context

Orcai currently embeds its three UI widgets (sysop panel, session picker, welcome dashboard) as BubbleTea components compiled directly into the main binary. There is no way to replace them without forking. The plugin-system-v2 design established a widget manifest + binary protocol, but assumes widgets are *external* binaries — it has no concept of widgets that ship *inside* orcai itself. This design bridges that gap: core widgets become built-in subcommands while remaining replaceable via the same PATH-lookup convention Git uses for extensions.

A second concern is tmux ownership. Orcai currently claims pane layout and keybindings at startup unconditionally. Power users who have existing tmux configs or who want non-standard widget placement have no escape hatch.

## Goals / Non-Goals

**Goals:**
- Core widgets (sysop, picker, welcome) run as `orcai <name>` subcommands — no separate binaries to distribute
- PATH-based override: `orcai-sysop` in PATH is called instead of the built-in; zero config required
- Layout config (`layout.yaml`) controls pane geometry at startup; absent = do nothing
- Keybinding config (`keybindings.yaml`) controls which keys orcai binds; absent key = untouched in tmux
- No changes to pipeline, bus, or provider subsystems

**Non-Goals:**
- Runtime widget hot-swap (restart required to pick up a new override binary)
- Layout validation or conflict detection between overlapping pane specs
- Multi-session layout variations (one layout file applies globally)
- Widget protocol changes (the framed JSON bus protocol from v2 design is unchanged)

## Decisions

### D1: Subcommand-first, not sidecar-first

**Decision:** Core widgets are `cobra` subcommands of the `orcai` binary, not YAML sidecars.

**Rationale:** Sidecars require a YAML manifest and a separate binary to exist on disk. Built-in widgets have neither — they are Go code. Modeling them as subcommands gives them a stable, scriptable entry point (`orcai welcome`) without requiring any on-disk artifacts. The widget dispatch layer calls `exec.Command("orcai", "welcome")` the same way it would call any external binary, so the dispatch logic is uniform.

**Alternative considered:** Register built-ins as in-process functions and skip exec entirely. Rejected — it tightly couples the dispatch layer to the widget implementations and prevents the override path from being transparent.

### D2: Git-style override via PATH lookup

**Decision:** Before invoking `orcai <name>`, the dispatch layer runs `exec.LookPath("orcai-" + name)`. If found, that binary is exec'd instead.

**Rationale:** Git popularised this convention (`git foo` → `git-foo` in PATH). It requires zero config — drop an `orcai-sysop` binary somewhere on PATH and it takes over. It also sidesteps any manifest-management overhead for simple replacements.

**Override resolution order:**
1. `orcai-<name>` anywhere in `$PATH`
2. Built-in `orcai <name>` subcommand

**Alternative considered:** Manifest-declared override in `widget.yaml`. Rejected — adds friction for a simple "replace this widget" use case. Manifest approach is still valid for new widgets that don't shadow built-ins.

### D3: Layout config is applied once at session init, then hands off

**Decision:** `layout.yaml` is read at `orcai attach` time. Orcai runs the tmux commands to create panes and launch widgets, then stops asserting any layout. The user retains full tmux control after init.

**Rationale:** Persistent layout enforcement would fight the user every time they resize, split, or close a pane. One-shot init gives a sensible default without being antagonistic.

**Default behavior (no layout.yaml):** Orcai does nothing — no panes created, no layout applied. Existing tmux users are unaffected.

**layout.yaml shape:**
```yaml
panes:
  - name: welcome
    widget: welcome
    position: left      # left | right | top | bottom | float
    size: 40%           # columns (left/right) or rows (top/bottom)
  - name: sysop
    widget: sysop
    position: right
    size: 60%
```

### D4: Keybindings are opt-in via config

**Decision:** Orcai only binds tmux keys listed in `keybindings.yaml`. An empty or absent file means orcai binds nothing.

**Rationale:** Orcai's default keybindings conflict with common tmux configs (prefix + arrow keys for pane navigation, for example). Making bindings opt-in inverts the current conflict — users who want orcai's shortcuts add them explicitly; everyone else gets no interference.

**keybindings.yaml shape:**
```yaml
bindings:
  - key: "M-s"        # tmux key notation
    action: launch-session-picker
  - key: "M-p"
    action: open-sysop
```

**Alternative considered:** Ship a default `keybindings.yaml` with all current bindings and let users delete what they don't want. Rejected — silent interference on first install is worse UX than no interference.

## Risks / Trade-offs

- **PATH override is global** → If a user has an `orcai-welcome` from a third-party tool that has nothing to do with orcai widgets, it will be invoked. Mitigation: document the naming convention clearly; consider a `ORCAI_NO_PATH_OVERRIDE=1` escape hatch.
- **One-shot layout means drift** → After init, panes can be closed or resized. Re-running `orcai attach` on an existing session may try to create duplicate panes. Mitigation: check for existing pane names before creating; skip if already present.
- **Empty keybindings.yaml breaks existing installs** → Users upgrading from a version that auto-bound keys will lose those bindings. Mitigation: migration step that generates a `keybindings.yaml` from the current defaults during upgrade; document in changelog.
- **`orcai sysop` as a subcommand conflicts with external use** → If the built-in sysop is also what the daemon calls, circular exec is possible if the override binary is itself `orcai`. Mitigation: dispatch layer detects self-referential calls (argv[0] == override binary path) and falls back to built-in.

## Migration Plan

1. Register `sysop`, `picker`, `welcome` as cobra subcommands (no behavior change, just new entry points)
2. Introduce `internal/widgetdispatch` with PATH lookup + built-in fallback
3. Replace direct BubbleTea widget instantiation in session init with dispatch calls
4. Add `internal/layout` package; read `layout.yaml` in `orcai attach`; no-op if file absent
5. Add `internal/keybindings` package; bind only keys in `keybindings.yaml`; warn and skip missing file
6. Update `orcai attach` to run layout init before handing off to the TUI
7. Provide a `orcai config init` command that writes default `layout.yaml` and `keybindings.yaml` for users who want the classic behavior

**Rollback:** All new behavior is gated on config files. Deleting `layout.yaml` and `keybindings.yaml` restores pre-change behavior without a binary downgrade.

## Open Questions

- Should `orcai config init` be interactive (prompting for each widget position) or just dump opinionated defaults?
- Do we want `orcai-<name>` override binaries to receive the same framed JSON bus protocol as external widget binaries, or is a simpler stdio pipe sufficient?
- Should the layout config support named tmux windows (not just panes within the current window), for users who want widgets on a dedicated window?
