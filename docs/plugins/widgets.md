# Widget Plugins — Contributor Guide

A widget is a standalone binary that orcai launches in a new tmux window. Widgets receive live events from orcai (theme changes, session lifecycle, telemetry) over a Unix socket bus, letting them react to the workspace state without any shared memory or internal coupling to orcai's own code.

Widgets can be written in any language that can open a Unix socket — Go, Python, Rust, Node.js, or even a plain shell script.

## Widget manifest (`widget.yaml`)

Every widget has a manifest file that tells orcai its name, what binary to run, and which events it wants to receive.

```yaml
name: my-widget
binary: my-widget-binary
description: "What this widget does"
subscribe:
  - theme.changed
  - session.started
```

### Field reference

| Field | Required | Description |
|-------|----------|-------------|
| `name` | yes | Kebab-case unique identifier. |
| `binary` | yes | Executable name resolved via `exec.LookPath`. Must be in `$PATH`. |
| `description` | no | Short human-readable description. |
| `subscribe` | no | List of event topics this widget wants to receive. Supports wildcards (see below). |

## Where to install

Place the manifest and binary together under:

```
~/.config/orcai/widgets/my-widget/widget.yaml
```

orcai scans `~/.config/orcai/widgets/` at startup, loading each subdirectory that contains a valid `widget.yaml`.

## Bus protocol

orcai runs a Unix socket event bus daemon (`busd`) that all widgets connect to.

### Socket path

```
$XDG_RUNTIME_DIR/orcai/bus.sock
```

Falls back to:

```
~/.cache/orcai/bus.sock
```

### Message format

All messages are newline-delimited JSON (`\n` terminated). Both frames sent by the widget and frames delivered by orcai use this format.

### Registration frame (widget → orcai)

The first line the widget writes after connecting declares its identity and subscriptions:

```json
{"name":"my-widget","subscribe":["theme.changed"]}\n
```

### Event frame (orcai → widget)

After registration, orcai delivers matching events as JSON objects:

```json
{"event":"theme.changed","payload":{"name":"abs"}}\n
```

The `payload` shape varies by event topic — see the event catalogue below.

### Topic wildcards

A subscription of `session.*` matches `session.started`, `session.ended`, and any future `session.X` topic. Wildcards use a single `*` as the final segment and match exactly one level.

### Event catalogue

| Topic | Payload fields | Description |
|-------|---------------|-------------|
| `theme.changed` | `name` (string) | Active theme was switched. |
| `session.started` | `provider`, `model` | A new AI session window was opened. |
| `session.ended` | `provider` | An AI session window was closed. |
| `orcai.telemetry` | varies | Internal metrics (experimental). |

## Widget lifecycle

1. orcai discovers the widget manifest on startup.
2. When the user opens the widget (or orcai launches it automatically), orcai calls `tmux new-window -n <name> <binary>`.
3. The binary starts, connects to the bus socket, and sends the registration frame.
4. orcai routes matching events to the widget's connection.
5. When the user closes the tmux window, the binary exits; orcai prunes the dead connection.

## Example widget (bash)

```bash
#!/bin/bash
# hello-widget: prints every theme.changed event to stdout.
SOCK="${XDG_RUNTIME_DIR:-$HOME/.cache}/orcai/bus.sock"
echo '{"name":"hello","subscribe":["theme.changed"]}' | nc -U "$SOCK"
```

Save as `~/.config/orcai/widgets/hello/hello-widget` (chmod +x), then create:

```yaml
# ~/.config/orcai/widgets/hello/widget.yaml
name: hello
binary: hello-widget
description: "Prints theme change events"
subscribe:
  - theme.changed
```

## Language support

Any runtime that can open a Unix domain socket works:

- **Go** — `net.Dial("unix", sockPath)`
- **Python** — `socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)`
- **Rust** — `std::os::unix::net::UnixStream`
- **Node.js** — `net.createConnection(sockPath)`
- **Shell** — `nc -U $SOCK` or `socat UNIX-CONNECT:$SOCK -`
