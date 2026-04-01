---
name: sidecar-gen
description: Generate a ~/.config/glitch/wrappers/<name>.yaml sidecar for any CLI tool, making it a Tier 2 orcai plugin. Invoke with the tool name as argument.
disable-model-invocation: true
---

Generate a Tier 2 orcai plugin sidecar YAML for a CLI tool.

When invoked with a tool name as argument (e.g. `/sidecar-gen ripgrep`):

1. Read internal/discovery/discovery.go to understand SidecarSchema fields
2. Run: `<tool> --help 2>&1 | head -60` to understand the tool's interface
3. Run: `<tool> --version 2>&1` to get version info
4. Infer from help output:
   - Input format: does it read from stdin? Accept JSON flags? Positional args?
   - Output format: JSON, plain text, or mixed?
   - Key flags/args to expose as plugin parameters
5. Generate YAML with these fields (all optional except name and command):
   ```yaml
   name: <tool-name>
   description: <one-line from --help>
   command: <tool-binary>
   args:
     - <sensible default args>
   input_schema: |
     <JSON Schema string if input is structured, omit if plain text>
   output_schema: |
     <JSON Schema string if output is structured, omit if plain text>
   ```
6. Write to: ~/.config/glitch/wrappers/<tool-name>.yaml
   - Create directory if it doesn't exist: mkdir -p ~/.config/glitch/wrappers/
7. Print:
   - Path written
   - "Test with: orcai --plugin <name> <sample-input>"
   - "Edit at: ~/.config/glitch/wrappers/<name>.yaml"

Reference: The sidecar is loaded by internal/discovery/discovery.go — field names must match SidecarSchema exactly.
