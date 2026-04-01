---
name: plugin-dev
description: Generate a new orcai plugin stub that satisfies GlitchPlugin gRPC interface and registers capabilities
disable-model-invocation: true
---

Generate a new orcai plugin implementation. Read these files first:
- proto/gl1tch/v1/plugin.proto — defines GlitchPlugin gRPC service
- internal/plugin/plugin.go — Plugin interface and StubPlugin pattern
- internal/discovery/discovery.go — how plugins are discovered

A plugin must:
1. Implement the Plugin interface (Name, Capabilities, Execute)
2. Support streaming ExecuteResponse chunks for real-time output
3. Register capabilities (what commands/actions it handles)
4. Be discoverable by the plugin manager

When invoked with a plugin name/purpose as arguments, generate:
- A complete Go plugin stub in a new directory
- Capability registration matching the plugin's purpose
- A Makefile target to build and register the plugin
- Brief usage notes
