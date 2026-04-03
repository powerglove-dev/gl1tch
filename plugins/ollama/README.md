# orcai-ollama

orcai plugin for running local AI models via the [Ollama](https://ollama.com) daemon.

## Installation

```sh
make install
```

This builds `orcai-ollama` and copies it to `~/.local/bin/` alongside `ollama.yaml` to `~/.config/orcai/wrappers/`.

## Usage in Pipelines

```yaml
- id: generate
  plugin: ollama
  prompt: "Explain BubbleTea in one paragraph."
  vars:
    model: llama3.2
```

### Passing Inference Options

Use `option_<key>` vars to set Ollama inference parameters at call time:

```yaml
- id: generate
  plugin: ollama
  prompt: "Summarise this document."
  vars:
    model: llama3.2
    option_num_ctx: "16384"
    option_temperature: "0"
```

The `option_<key>` pairs are translated to `--option key=value` arguments. Integer-parseable values are forwarded as JSON numbers; all others as strings.

## Direct Invocation

```sh
echo "What is BubbleTea?" | orcai-ollama --model llama3.2

# With a larger context window
echo "Summarise this long doc." | orcai-ollama --model llama3.2 --option num_ctx=16384
```

## Creating Named Model Aliases

Use `--create-model` to bake inference parameters into a reusable Ollama model alias. This avoids repeating `--option` flags on every call and makes the alias available to any plugin by name.

```sh
orcai-ollama --model llama3.2 --option num_ctx=16384 --create-model llama3.2-16k
# Created model 'llama3.2-16k'

orcai-ollama --model qwen2.5 --option num_ctx=16384 --create-model qwen2.5-16k
# Created model 'qwen2.5-16k'

orcai-ollama --model qwen3:8b --option num_ctx=16384 --create-model qwen3:8b-16k
# Created model 'qwen3:8b-16k'
```

After creation, use the alias anywhere:

```sh
echo "Hello" | orcai-ollama --model llama3.2-16k
```

Or in a pipeline step:

```yaml
vars:
  model: llama3.2-16k
```

The alias is also usable from the opencode plugin as `ollama/llama3.2-16k` — see the [orcai-opencode README](../opencode/README.md).

> **opencode users:** opencode validates model names against its own config. After creating an alias, register it in `~/.config/opencode/opencode.json` under `provider.ollama.models`. See the [orcai-opencode README](../opencode/README.md#registering-aliases-in-opencode) for the exact snippet.

### How it works

`--create-model` generates an Ollama Modelfile like:

```
FROM llama3.2
PARAMETER num_ctx 16384
```

and runs `ollama create <name> -f <modelfile>`. The resulting alias is stored in Ollama's local model registry and persists across restarts.

Multiple `--option` flags each become a `PARAMETER` line:

```sh
orcai-ollama --model llama3.2 \
  --option num_ctx=16384 \
  --option temperature=0 \
  --create-model llama3.2-precise-16k
```

## Environment Variables

| Variable | Description | Default |
|---|---|---|
| `GLITCH_MODEL` | Fallback model name when `--model` is not set | — |
| `GLITCH_OLLAMA_URL` | Base URL of the Ollama daemon | `http://localhost:11434` |

## Flags

| Flag | Description |
|---|---|
| `--model <name>` | Ollama model to use (e.g. `llama3.2`, `qwen2.5-16k`) |
| `--option key=value` | Inference option, repeatable (e.g. `--option num_ctx=16384`) |
| `--create-model <name>` | Create a named model alias from `--model` + options, then exit |
| `--list-models` | Output installed models as JSON and exit |
