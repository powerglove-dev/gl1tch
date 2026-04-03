---
title: "Plugins"
description: "Install community pipelines and executors — or build your own — to extend what your assistant can do."
order: 99
---

gl1tch ships with a solid set of built-in executors, but the real power is what the community builds on top. Plugins add new executors to your assistant — a MUD world manager, a weather fetcher, a push notifier — and once installed they work in any pipeline just like the built-ins. Think of it as an app store for your AI assistant.


## Quick Start

Install a plugin from GitHub with one command:

```bash
glitch plugin install owner/plugin-name
```

List what you have installed:

```bash
glitch plugin list
```

Use it in a pipeline immediately:

```yaml
steps:
  - id: post
    executor: plugin-name
    args:
      message: "Pipeline complete"
```

That's it. No restarts, no config edits.


## Installing Plugins

### From GitHub

```bash
# Install latest version
glitch plugin install owner/repo

# Pin to a specific version or branch
glitch plugin install owner/repo@v2.1.0
glitch plugin install owner/repo@main
```

gl1tch reads the plugin's manifest, installs the binary, and registers it as an executor. The plugin is ready to use in your pipelines immediately.

### Listing Installed Plugins

```bash
glitch plugin list
```

```text
gl1tch-mud      8op-org/gl1tch-mud      v1.0.0    ~/.local/bin/gl1tch-mud
gl1tch-weather  8op-org/gl1tch-weather  v1.2.0    ~/.local/bin/gl1tch-weather
gl1tch-notify   8op-org/gl1tch-notify   v0.9.0    ~/.local/bin/gl1tch-notify
```

### Removing a Plugin

```bash
glitch plugin remove plugin-name
```


## Using a Plugin in a Pipeline

Once installed, a plugin becomes an executor. Reference it by name in any pipeline step:

```yaml
name: morning-brief
version: "1"

steps:
  - id: weather
    executor: gl1tch-weather
    args:
      location: "Austin, TX"

  - id: notify
    executor: gl1tch-notify
    needs: [weather]
    args:
      title: "Morning brief"
      body: "{{steps.weather.output}}"
```


## Building Your Own Plugin

A plugin is a GitHub repo with a `glitch-plugin.yaml` manifest at its root. The manifest tells gl1tch how to install the binary and how to invoke it.

### Minimal manifest

```yaml
name: my-tool
description: "Does something useful"
binary: my-tool

install:
  go: github.com/you/my-tool/cmd/my-tool

sidecar:
  command: my-tool
  args: ["--format", "json"]
  category: tools
  kind: tool
```

### Using a pre-built binary instead of Go source

```yaml
name: my-tool
description: "Does something useful"
binary: my-tool
version: "v1.0.0"

install:
  release: true          # downloads from GitHub Releases

sidecar:
  command: my-tool
  category: tools
  kind: tool
```

gl1tch picks the right binary for your platform automatically (`darwin-arm64`, `linux-x86_64`, etc.).

### Manifest reference

| Field | Required | Description |
|-------|----------|-------------|
| `name` | yes | Unique plugin name — becomes the executor name |
| `description` | yes | Short description shown in `glitch plugin list` |
| `binary` | no | Binary name on PATH; defaults to `name` |
| `version` | no | Git ref, tag, or branch to install |
| `install.go` | one of | Go module path for `go install` builds |
| `install.release` | one of | `true` to download from GitHub Releases |
| `sidecar.command` | yes | The executable gl1tch calls |
| `sidecar.args` | no | Default arguments appended to every invocation |
| `sidecar.category` | no | `"tools"` or `"providers"` |
| `sidecar.kind` | no | `"tool"` or `"agent"` |
| `sidecar.input_schema` | no | JSON Schema for the executor's expected input |
| `sidecar.output_schema` | no | JSON Schema for the executor's output |


## Examples


### gl1tch-mud

Run a text MUD world as a pipeline executor. Each step can query room state, move the player, or trigger world events.

```bash
glitch plugin install 8op-org/gl1tch-mud
```

```yaml
name: explore-sector
version: "1"

steps:
  - id: look
    executor: gl1tch-mud
    args:
      command: look

  - id: describe
    executor: llm
    needs: [look]
    prompt: |
      Narrate this room in two sentences for a cyberpunk setting:
      {{steps.look.output}}
```


### gl1tch-weather

Fetch current conditions or forecasts. Useful as a data source step before any LLM summary or notification.

```bash
glitch plugin install 8op-org/gl1tch-weather
```

```yaml
name: weather-digest
version: "1"

steps:
  - id: fetch
    executor: gl1tch-weather
    args:
      location: "Austin, TX"
      units: imperial

  - id: summarize
    executor: llm
    needs: [fetch]
    prompt: "Summarize this forecast in one sentence: {{steps.fetch.output}}"
```


### gl1tch-notify

Send desktop or push notifications when a pipeline finishes, fails, or hits a threshold.

```bash
glitch plugin install 8op-org/gl1tch-notify
```

```yaml
name: deploy-and-notify
version: "1"

steps:
  - id: deploy
    executor: shell
    command: "make deploy"

  - id: alert
    executor: gl1tch-notify
    needs: [deploy]
    args:
      title: "Deploy complete"
      body: "{{steps.deploy.output}}"
```


### Publishing Your Own Plugin

1. Create a GitHub repo with your binary and a `glitch-plugin.yaml` at the root.
2. Tag a release: `git tag v1.0.0 && git push --tags`.
3. Share the install command: `glitch plugin install your-username/your-plugin`.

Anyone with gl1tch can install it in one command.


## See Also

- [Pipelines](/docs/pipelines/pipelines) — how plugins work as executors in pipeline steps
- [Cron](/docs/pipelines/cron) — schedule pipelines that use your installed plugins
- [Brain](/docs/pipelines/brain) — combine plugins with memory for smarter automations
