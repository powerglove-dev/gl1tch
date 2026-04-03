---
title: "Plugins"
description: "Install community pipelines and executors — or build your own — to extend what your assistant can do."
order: 99
---

gl1tch ships with a solid set of built-in executors, but the real power is what the community builds on top. Plugins add new executors to your assistant — a Jira client, a Slack poster, a custom code generator — and once installed, they work in any pipeline just like the built-ins. Think of it as an app store for your AI assistant.


## Quick Start

Install a plugin from GitHub with one command:

```bash
glitch apm install owner/plugin-name
```

List what you have installed:

```bash
glitch apm list
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
glitch apm install owner/repo

# Pin to a specific version or branch
glitch apm install owner/repo@v2.1.0
glitch apm install owner/repo@main
```

gl1tch reads the plugin's manifest, installs the binary, and registers it as an executor. The plugin is ready to use in your pipelines immediately.

### Listing Installed Plugins

```bash
glitch apm list
```

```text
my-plugin       owner/my-plugin       v1.0.0    ~/.local/bin/my-plugin
codegen         owner/codegen         v2.1.0    ~/.local/bin/codegen
```

### Removing a Plugin

```bash
glitch apm remove plugin-name
```


## Using a Plugin in a Pipeline

Once installed, a plugin becomes an executor. Reference it by name in any pipeline step:

```yaml
name: notify-and-generate
version: "1"

steps:
  - id: generate
    executor: codegen
    args:
      prompt: "Write a Fibonacci function in Python"

  - id: notify
    executor: slack-poster
    needs: [generate]
    args:
      channel: "#dev"
      message: "Code generation complete"
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
| `description` | yes | Short description shown in `glitch apm list` |
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


### Slack Notification Plugin

Install and use a Slack posting plugin:

```bash
glitch apm install acme/glitch-slack
```

```yaml
name: deploy-notify
version: "1"

steps:
  - id: deploy
    executor: shell
    command: "make deploy"

  - id: notify
    executor: glitch-slack
    needs: [deploy]
    args:
      channel: "#deploys"
      message: "Deploy finished at {{now}}"
```


### Code Generator Plugin

Pin to a specific release for reproducibility:

```bash
glitch apm install acme/glitch-codegen@v2.1.0
```

```yaml
name: generate-tests
version: "1"

steps:
  - id: generate
    executor: glitch-codegen
    args:
      language: python
      prompt: "Write unit tests for the attached function"
      input: "{{steps.read.output}}"
```


### Publishing Your Own Plugin

1. Create a GitHub repo with your binary and a `glitch-plugin.yaml` at the root.
2. Tag a release: `git tag v1.0.0 && git push --tags`.
3. Share the install command: `glitch apm install your-username/your-plugin`.

Anyone with gl1tch can install it in one command.


## See Also

- [Pipelines](/docs/pipelines/pipelines) — how plugins work as executors in pipeline steps
- [Cron](/docs/pipelines/cron) — schedule pipelines that use your installed plugins
- [Brain](/docs/pipelines/brain) — combine plugins with memory for smarter automations
