---
title: "Plugins"
description: "Install and manage gl1tch plugins from GitHub, configure sidecar executors, and unlock plugin-driven pipelines."
order: 99
---

Plugins extend gl1tch with new executors and capabilities. The plugin system lets you discover, install, and activate plugins from GitHub repositories with a single command. Once installed, plugins become first-class executors available to any pipeline.


## Architecture overview

The plugin system consists of three layers:

**1. Discovery & Installation** (`internal/pluginmanager/`)
- `Installer` manages the full lifecycle: fetch manifest from GitHub, install the binary (via `go install` or GitHub Releases), write the sidecar YAML, and record metadata in a local registry.
- `FetchManifest()` downloads `glitch-plugin.yaml` from a GitHub repo at a specified ref (branch, tag, or commit).
- `InstallBinary()` handles both Go modules (`go install github.com/user/plugin/cmd/pluginname`) and pre-built GitHub Release assets.
- Registry (`~/.config/glitch/plugins.yaml`) tracks all installed plugins: name, source, version, binary path, sidecar path, and install timestamp.

**2. Sidecar Activation** (`~/.config/glitch/wrappers/`)
- Each plugin installation writes a sidecar YAML file (`~/.config/glitch/wrappers/<plugin-name>.yaml`) that declares the executor's command, arguments, input/output schema, and signal handlers.
- The sidecar format is identical to the `executor.SidecarSchema` used by the core executor subsystem.
- Sidecars can be authored inline in the plugin manifest or generated automatically from the manifest.

**3. Agent Capability Provider** (`internal/apmmanager/`)
- `AgentCapabilityProvider` interface lets pipelines request capabilities at runtime without knowing if the agent is installed.
- `RequireAgent(ctx, agentID)` blocks until installation completes, enabling on-demand agent bootstrapping.
- Messages (`AgentInstallStartMsg`, `AgentInstallDoneMsg`, `AgentInstallErrMsg`) integrate with the TUI to show install progress.


## Technologies

- **YAML** — Plugin manifests (`glitch-plugin.yaml`) and sidecar files use YAML 1.2 via `gopkg.in/yaml.v3`.
- **GitHub API** — Plugin discovery and binary downloads use GitHub's raw content API and releases API.
- **Go modules** — Go-based plugins are installed via `go install` with automatic `GOPRIVATE` / `GONOSUMDB` configuration for private repos.
- **BubbleTea** — The APM manager TUI component provides a two-pane interface for browsing, installing, and managing agents.


## Concepts

**Plugin Manifest** (`glitch-plugin.yaml`)
The authoritative source for a plugin. Defines the binary name, installation method, version, sidecar schema, and signal handlers. Located at the root of a plugin repository.

**Sidecar YAML**
An executor descriptor written to `~/.config/glitch/wrappers/<name>.yaml` during plugin installation. Declares the command, arguments, input/output schema, and category (e.g., "providers", "tools"). Enables the executor manager to invoke the plugin without code changes.

**Plugin Reference**
A string identifying a plugin: `owner/repo` (floating version, defaults to `main` or `master` branch) or `owner/repo@ref` (pinned to a branch, tag, or commit).

**Install Method**
Either `go` (source build via `go install`) or `release` (download pre-built binary from GitHub Releases). The manifest declares which method to use.

**Registry**
`~/.config/glitch/plugins.yaml` — a YAML file tracking all installed plugins: name, GitHub source, version, binary path, sidecar path, and install timestamp.

**Signal Handler**
A named function within a plugin binary that gl1tch invokes when a BUSD topic fires. Declared in the manifest's `sidecar.signals` list with `topic` (BUSD topic name) and `handler` (function name).


## Configuration / YAML reference

### Plugin Manifest Schema (`glitch-plugin.yaml`)

```yaml
name: <string>
  # Canonical plugin name (used as executor name and registry key).

description: <string>
  # Short human-readable description of what the plugin does.

binary: <string, optional>
  # Name of the installed binary on PATH.
  # Defaults to name.

version: <string, optional>
  # Pinned version / git ref (branch, tag, or commit).
  # If omitted, installer tries main or master.

install:
  go: <string, optional>
    # Go module path for source build, e.g. github.com/user/plugin/cmd/tool
    # Used when method is go; incompatible with release: true

  release: <boolean, optional>
    # If true, download pre-built binary from GitHub Releases.
    # Asset name is inferred from binary name + GOOS + GOARCH.

sidecar:
  command: <string>
    # Executable name or full path (e.g. mybin or /usr/local/bin/mybin).

  args: <array[string], optional>
    # Default arguments passed to the command (e.g. ["--quiet"]).

  description: <string, optional>
    # Override for the executor description.

  category: <string, optional>
    # Executor category (e.g. "providers", "tools").

  kind: <string, optional>
    # "agent" or "tool"; controls how the executor is invoked in pipelines.

  input_schema: <string, optional>
    # JSON Schema (as a string) describing expected input format.

  output_schema: <string, optional>
    # JSON Schema (as a string) describing output format.

  signals:
    - topic: <string>
        # BUSD topic name (e.g. "chat.message.created").
      handler: <string>
        # Named function in the plugin binary to invoke (e.g. "OnMessageCreated").
```

### Sidecar File Schema (`~/.config/glitch/wrappers/<name>.yaml`)

Identical to the manifest `sidecar` block, but written to disk as a standalone YAML file. Generated automatically during installation.

### Registry Entry Schema (`~/.config/glitch/plugins.yaml`)

```yaml
plugins:
  - name: <string>
      # Plugin canonical name.
    source: <string>
      # GitHub source as "owner/repo".
    version: <string>
      # Installed version or ref.
    binary_path: <string>
      # Absolute path to the installed executable.
    sidecar_path: <string>
      # Absolute path to the sidecar YAML.
    installed_at: <ISO8601 timestamp>
      # When the plugin was installed.
```


## Examples

### Example 1: Install a Go plugin

```bash
glitch plugin install owner/my-plugin
```

Plugin repository structure:
```
owner/my-plugin/
├── glitch-plugin.yaml
├── cmd/
│   └── myplugin/
│       └── main.go
└── go.mod
```

`glitch-plugin.yaml`:
```yaml
name: myplugin
description: "A simple plugin that greets the world"
binary: myplugin
version: "v1.0.0"
install:
  go: github.com/owner/my-plugin/cmd/myplugin
sidecar:
  command: myplugin
  args: ["--format", "json"]
  category: tools
  kind: tool
  description: "Greet the world"
  input_schema: |
    {
      "type": "object",
      "properties": {
        "name": {"type": "string"}
      }
    }
  output_schema: |
    {
      "type": "object",
      "properties": {
        "greeting": {"type": "string"}
      }
    }
```

After `glitch plugin install owner/my-plugin@v1.0.0`:
- Binary installed to `~/.local/bin/myplugin` (or `$GOPATH/bin/myplugin`)
- Sidecar written to `~/.config/glitch/wrappers/myplugin.yaml`
- Registry entry added to `~/.config/glitch/plugins.yaml`

### Example 2: Install a pre-built Release plugin

```bash
glitch plugin install owner/prebuilt-plugin
```

`glitch-plugin.yaml` with release-based install:
```yaml
name: codegen
description: "Code generation tool with pre-built binaries"
binary: codegen
version: "v2.1.0"
install:
  release: true
sidecar:
  command: codegen
  category: tools
  kind: tool
  args: ["--mode", "generate"]
```

The installer:
1. Fetches the manifest from the repo's `main` branch (or specified version)
2. Downloads the release asset `codegen-darwin-arm64` or `codegen-linux-x86_64` (based on GOOS/GOARCH)
3. Moves the binary to `~/.local/bin/codegen`
4. Writes the sidecar to `~/.config/glitch/wrappers/codegen.yaml`

### Example 3: Plugin with signal handlers

`glitch-plugin.yaml`:
```yaml
name: chat-listener
description: "Chat event listener and responder"
binary: chat-listener
install:
  go: github.com/owner/chat-listener/cmd/chat-listener
sidecar:
  command: chat-listener
  category: tools
  signals:
    - topic: "chat.message.created"
      handler: "OnNewMessage"
    - topic: "chat.user.joined"
      handler: "OnUserJoined"
```

When a BUSD message is published to `chat.message.created`, gl1tch invokes the plugin binary with the signal topic and payload.

### Example 4: Using a plugin in a pipeline

Once installed, the plugin is available as an executor in any pipeline:

```yaml
id: my-workflow
name: "Workflow using installed plugins"
steps:
  - id: greet
    executor: myplugin
    args:
      name: "Alice"
    vars: {}

  - id: generate_code
    executor: codegen
    args:
      prompt: "Write a Fibonacci function"
    vars:
      CODEGEN_LANG: python
```

The executor manager resolves `myplugin` by looking it up in the sidecar directory. The sidecar YAML is loaded, and the executor is invoked with the declared command and merged arguments.

### Example 5: Listing installed plugins

```bash
glitch plugin list
```

Output from the registry:
```
myplugin                | owner/my-plugin       | v1.0.0       | ~/.local/bin/myplugin
codegen                 | owner/prebuilt-plugin | v2.1.0       | ~/.local/bin/codegen
chat-listener           | owner/chat-listener   | v1.5.2       | ~/.local/bin/chat-listener
```

### Example 6: On-demand agent installation via RequireAgent

In a pipeline step that needs an APM agent:

```go
// Pipeline builtin step (e.g., builtin.agent_step) calls:
agent, err := provider.RequireAgent(ctx, "api-architect")
if err != nil {
    return fmt.Errorf("need api-architect agent: %w", err)
}
// agent.ExecutorID is now registered and available
```

The TUI shows progress:
- `AgentInstallStartMsg` — spinner appears
- `AgentInstallDoneMsg` — agent is live
- `AgentInstallErrMsg` — error displayed to user


## See Also

- [Pipelines](./pipelines.md) — how plugins are used as executors in pipeline steps
- [Executors](./executors.md) — how sidecar files integrate with the executor manager
- [APM System](./apm.md) — on-demand agent installation and capability provisioning

