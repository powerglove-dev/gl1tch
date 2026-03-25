# Provider Profiles — Contributor Guide

Provider profiles decouple the orcai core from any specific AI CLI tool. Instead of hard-coding knowledge about Claude, Gemini, or any other assistant binary, orcai loads a YAML profile at startup that describes where the binary lives, which environment variable carries the API key, what models are available, and how to launch a tmux session for that provider. Adding support for a new AI CLI requires nothing more than dropping in a YAML file.

## YAML Schema

```yaml
name: my-provider          # kebab-case, unique identifier
binary: my-cli             # binary name (resolved via exec.LookPath)
display_name: "My Provider"
api_key_env: MY_API_KEY    # env var orcai checks for key presence

models:
  - id: my-model-id
    display: "My Model"
    cost_input_per_1m: 0.50
    cost_output_per_1m: 1.50

session:
  window_name: my-provider  # tmux window name for this provider's session
  launch_args: []           # extra CLI arguments appended after the model flag
  env: {}                   # additional environment variables set for the process
```

### Field reference

| Field | Required | Description |
|-------|----------|-------------|
| `name` | yes | Kebab-case unique identifier. Used as the registry key; user profiles override bundled profiles of the same name. |
| `binary` | yes | Executable name. orcai resolves it with `exec.LookPath` — it must be in `$PATH`. |
| `display_name` | yes | Human-readable label shown in the picker UI. |
| `api_key_env` | yes | Environment variable orcai inspects to determine whether the provider is authenticated. If the variable is unset or empty the provider is shown as unavailable. |
| `models[].id` | yes | Model identifier passed to the binary (e.g. `claude-opus-4-5`). |
| `models[].display` | yes | Human-readable model label shown in the picker. |
| `models[].cost_input_per_1m` | no | Approximate USD cost per million input tokens (informational only). |
| `models[].cost_output_per_1m` | no | Approximate USD cost per million output tokens (informational only). |
| `session.window_name` | no | tmux window name. Defaults to `name` if omitted. |
| `session.launch_args` | no | List of extra arguments appended to the CLI invocation. |
| `session.env` | no | Map of additional environment variables for the child process. |

## Where to install

Place your profile at:

```
~/.config/orcai/providers/my-provider.yaml
```

orcai scans this directory at startup via `providers.LoadUser()`. The file name does not need to match the `name` field, but keeping them consistent avoids confusion.

## Bundled providers and override behaviour

orcai ships bundled profiles for Claude, Gemini, OpenCode, Aider, Goose, and GitHub Copilot. These are embedded in the binary and loaded via `providers.LoadBundled()`.

When user profiles are loaded, a name collision causes the **user profile to win**. This lets you customize session arguments, add models, or override the binary path for any bundled provider without patching orcai itself.

## Binary detection

orcai calls `exec.LookPath(profile.Binary)` for every loaded profile. Providers whose binary is not found are excluded from the picker and from discovery. Make sure the binary is installed and available in `$PATH` before launching orcai.

## How to test your profile

1. Install your provider binary and set the required API key environment variable.
2. Drop the YAML file into `~/.config/orcai/providers/`.
3. Run `orcai new` — your provider should appear in the picker list.
4. Select it and verify that orcai opens a tmux window running your binary.
