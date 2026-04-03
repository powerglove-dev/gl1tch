# orcai-opencode

orcai plugin for running agentic AI coding tasks via [opencode](https://opencode.ai).

## Installation

```sh
make install
```

This builds `orcai-opencode` and copies it to `~/.local/bin/` alongside `opencode.yaml` to `~/.config/orcai/wrappers/`.

## Usage in Pipelines

```yaml
- id: agent
  plugin: opencode
  prompt: "Refactor this function to use error wrapping."
  vars:
    model: ollama/llama3.2
```

### Supported Model Formats

| Format | Example | Notes |
|---|---|---|
| `ollama/<name>` | `ollama/llama3.2` | Runs via local Ollama daemon |
| `ollama/<name>:<tag>` | `ollama/qwen2.5:latest` | Pinned tag |
| Other providers | `anthropic/claude-3-5-sonnet` | Passed through to opencode directly |

## Using Custom High-Context Models

If you've created named Ollama aliases with higher context windows (e.g. via `orcai-ollama --create-model`), reference them with the `ollama/` prefix:

```yaml
- id: agent
  plugin: opencode
  prompt: "Analyse this large codebase and suggest refactors."
  vars:
    model: ollama/llama3.2-16k
```

```yaml
- id: agent
  plugin: opencode
  prompt: "Review this diff."
  vars:
    model: ollama/qwen2.5-16k
```

The pre-configured 16k-context aliases included in `opencode.yaml` are:
- `ollama/llama3.2-16k:latest` — Llama 3.2 with 16384-token context
- `ollama/qwen2.5-16k:latest` — Qwen 2.5 with 16384-token context
- `ollama/qwen3:8b-16k` — Qwen 3 8B with 16384-token context

These are created locally with `orcai-ollama --create-model`. See the [orcai-ollama README](../ollama/README.md#creating-named-model-aliases) for the full workflow.

### Registering aliases in opencode

opencode validates model names against its own config before calling Ollama. Any custom alias must be added to `~/.config/opencode/opencode.json` under the `ollama.models` map, otherwise opencode will throw `ProviderModelNotFoundError` even if the model exists locally.

```json
{
  "provider": {
    "ollama": {
      "models": {
        "qwen2.5-16k:latest": { "name": "qwen2.5-16k:latest" },
        "llama3.2-16k:latest": { "name": "llama3.2-16k:latest" },
        "qwen3:8b-16k":        { "name": "qwen3:8b-16k" }
      }
    }
  }
}
```

Add this alongside your existing model entries — do not replace the `$schema`, `npm`, `name`, or `options` keys.

## Direct Invocation

```sh
echo "Explain this function." | orcai-opencode --model ollama/llama3.2
```

## Environment Variables

| Variable | Description |
|---|---|
| `GLITCH_MODEL` | Fallback model when `--model` is not set |

## Flags

| Flag | Description |
|---|---|
| `--model <provider/name>` | Model in `provider/model` format (e.g. `ollama/llama3.2-16k`) |
